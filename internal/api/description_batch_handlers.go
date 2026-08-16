package api

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/pborges/aiinventory/internal/auth"
	"github.com/pborges/aiinventory/internal/gemini"
	"github.com/pborges/aiinventory/internal/inventory"
	"github.com/pborges/aiinventory/internal/store"
)

const (
	maxDescriptionBatchItems = 60
	descriptionBatchWorkers  = 3
)

type descriptionBatchInput struct {
	ItemID int64  `json:"item_id"`
	Hint   string `json:"hint"`
}

type startDescriptionBatchRequest struct {
	Items []descriptionBatchInput `json:"items"`
}

type descriptionBatchItem struct {
	ItemID         int64  `json:"item_id"`
	AssetTag       string `json:"asset_tag"`
	PrimaryImageID int64  `json:"primary_image_id,omitempty"`
	Status         string `json:"status"`
	Description    string `json:"description,omitempty"`
	Error          string `json:"error,omitempty"`
	hint           string
}

type descriptionBatchStatus struct {
	Exists    bool                   `json:"exists"`
	Running   bool                   `json:"running"`
	StartedAt string                 `json:"started_at,omitempty"`
	Items     []descriptionBatchItem `json:"items"`
}

// descriptionBatchManager isolates job state by the authenticated user that
// started it. Inventory data is shared, but one user cannot observe, attach
// to, or block another user's background job.
type descriptionBatchManager struct {
	mu      sync.Mutex
	runners map[int64]*descriptionBatchRunner
}

func newDescriptionBatchManager() *descriptionBatchManager {
	return &descriptionBatchManager{runners: make(map[int64]*descriptionBatchRunner)}
}

func (m *descriptionBatchManager) runner(userID int64, create bool) *descriptionBatchRunner {
	m.mu.Lock()
	defer m.mu.Unlock()
	runner := m.runners[userID]
	if runner == nil && create {
		runner = &descriptionBatchRunner{}
		m.runners[userID] = runner
	}
	return runner
}

func (m *descriptionBatchManager) snapshot(userID int64) descriptionBatchStatus {
	runner := m.runner(userID, false)
	if runner == nil {
		return descriptionBatchStatus{Items: []descriptionBatchItem{}}
	}
	return runner.snapshot()
}

type descriptionBatchRunner struct {
	mu        sync.Mutex
	reserved  bool
	exists    bool
	running   bool
	startedAt time.Time
	items     []descriptionBatchItem
}

// reserve closes the check/start race without replacing the previous status
// before validation, settings lookup, and rate-limit charging have succeeded.
func (r *descriptionBatchRunner) reserve() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running || r.reserved {
		return false
	}
	r.reserved = true
	return true
}

func (r *descriptionBatchRunner) releaseReservation() {
	r.mu.Lock()
	r.reserved = false
	r.mu.Unlock()
}

func (r *descriptionBatchRunner) start(items []descriptionBatchItem) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reserved = false
	r.exists = true
	r.running = true
	r.startedAt = time.Now()
	r.items = items
}

func (r *descriptionBatchRunner) snapshot() descriptionBatchStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]descriptionBatchItem, len(r.items))
	copy(items, r.items)
	for i := range items {
		items[i].hint = ""
	}
	status := descriptionBatchStatus{Exists: r.exists, Running: r.running, Items: items}
	if r.exists {
		status.StartedAt = r.startedAt.UTC().Format(time.RFC3339)
	}
	return status
}

func (r *descriptionBatchRunner) begin(index int) descriptionBatchItem {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[index].Status = "generating"
	return r.items[index]
}

func (r *descriptionBatchRunner) complete(index int, description string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err != nil {
		r.items[index].Status = "error"
		r.items[index].Error = err.Error()
		return
	}
	r.items[index].Status = "done"
	r.items[index].Description = description
}

func (r *descriptionBatchRunner) finish() {
	r.mu.Lock()
	r.running = false
	r.mu.Unlock()
}

func (s *Server) handleDescriptionBatchStatus(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, s.descriptionBatches.snapshot(user.ID))
}

func (s *Server) handleStartDescriptionBatch(w http.ResponseWriter, r *http.Request) {
	client := s.geminiClient()
	if client == nil {
		writeError(w, http.StatusServiceUnavailable, "AI features are disabled (configure a Gemini API key in Settings)")
		return
	}
	user, ok := auth.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req startDescriptionBatchRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.Items) < 1 || len(req.Items) > maxDescriptionBatchItems {
		writeError(w, http.StatusBadRequest, "items must contain between 1 and 60 entries")
		return
	}

	seen := make(map[int64]struct{}, len(req.Items))
	items := make([]descriptionBatchItem, 0, len(req.Items))
	for _, input := range req.Items {
		if input.ItemID <= 0 {
			writeError(w, http.StatusBadRequest, "item_id must be positive")
			return
		}
		if _, exists := seen[input.ItemID]; exists {
			writeError(w, http.StatusBadRequest, "item_id values must be unique")
			return
		}
		seen[input.ItemID] = struct{}{}

		item, err := s.store.GetItemByID(r.Context(), input.ItemID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "item not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		images, err := s.store.ListImageMetaByItem(r.Context(), input.ItemID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		var primaryImageID int64
		if len(images) > 0 {
			primaryImageID = images[0].ID
		}
		items = append(items, descriptionBatchItem{
			ItemID: input.ItemID, AssetTag: item.AssetTag, PrimaryImageID: primaryImageID,
			Status: "pending", hint: input.Hint,
		})
	}

	model, prompt, err := s.resolveGeminiConfig(r.Context(), gemini.DescriptionRegeneration)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	runner := s.descriptionBatches.runner(user.ID, true)
	if !runner.reserve() {
		writeError(w, http.StatusConflict, "a description batch is already in progress")
		return
	}
	started := false
	defer func() {
		if !started {
			runner.releaseReservation()
		}
	}()
	// Charge only after this user's runner is successfully reserved. Rejected
	// concurrent starts therefore do not consume the AI budget.
	if !s.aiLimiter.allowN(s.clientKey(r), time.Now(), len(items)) {
		writeRateLimitError(w)
		return
	}

	runner.start(items)
	started = true
	go s.runDescriptionBatch(runner, client, user.ID, model, prompt, len(items))
	writeJSON(w, http.StatusAccepted, runner.snapshot())
}

func (s *Server) runDescriptionBatch(runner *descriptionBatchRunner, client gemini.Client, userID int64, model, prompt string, count int) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	defer runner.finish()

	jobs := make(chan int)
	var workers sync.WaitGroup
	workerCount := min(descriptionBatchWorkers, count)
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for index := range jobs {
				item := runner.begin(index)
				description, err := inventory.RegenerateDescription(ctx, s.store, client, userID, model, prompt, item.ItemID, item.hint)
				runner.complete(index, description, err)
			}
		}()
	}
	for index := 0; index < count; index++ {
		jobs <- index
	}
	close(jobs)
	workers.Wait()
}

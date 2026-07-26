package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/pborges/aiinventory/internal/auth"
	"github.com/pborges/aiinventory/internal/gemini"
	"github.com/pborges/aiinventory/internal/inventory"
	"github.com/pborges/aiinventory/internal/store"
)

type itemSummaryResponse struct {
	ID             int64  `json:"id"`
	AssetTag       string `json:"asset_tag"`
	Description    string `json:"description"`
	LocationCode   string `json:"location_code,omitempty"`
	PrimaryImageID *int64 `json:"primary_image_id,omitempty"`
}

// handleSearch implements the Search view (README flow #3): a free-text
// query plus the no-description/no-location/specific-location filters.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	params := store.SearchParams{
		Query:         q.Get("q"),
		NoDescription: q.Get("no_description") == "1",
		NoLocation:    q.Get("no_location") == "1",
	}
	if v := q.Get("location_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid location_id")
			return
		}
		params.LocationID = &id
	}

	results, err := s.store.SearchItems(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := make([]itemSummaryResponse, 0, len(results))
	for _, it := range results {
		out = append(out, itemSummaryResponse{
			ID:             it.ID,
			AssetTag:       it.AssetTag,
			Description:    it.Description,
			LocationCode:   it.LocationCode,
			PrimaryImageID: it.PrimaryImageID,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

type bulkItemIDsRequest struct {
	ItemIDs []int64 `json:"item_ids"`
}

// handleBulkDelete implements the Search view's bulk "Delete" action.
func (s *Server) handleBulkDelete(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req bulkItemIDsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx := r.Context()
	deleted := 0
	for _, id := range req.ItemIDs {
		if err := inventory.DeleteItem(ctx, s.store, user.ID, id); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		deleted++
	}
	writeJSON(w, http.StatusOK, map[string]int{"deleted": deleted})
}

type bulkRegenerateDescriptionRequest struct {
	Items []struct {
		ItemID int64  `json:"item_id"`
		Hint   string `json:"hint"`
	} `json:"items"`
}

// handleBulkRegenerateDescription kicks off a detached, server-side batch
// for the Search view's bulk "Regenerate description" action, returning
// immediately (202) rather than blocking for however long every item takes.
// The previous version ran the whole loop inline within this request, which
// meant closing the tab or refreshing the page cancelled r.Context() and
// silently stopped the remaining items partway through — this is why the
// batch runs on its own context in a goroutine and progress is polled via
// handleBulkRegenerateDescriptionStatus instead of returned here directly.
func (s *Server) handleBulkRegenerateDescription(w http.ResponseWriter, r *http.Request) {
	if s.gemini == nil {
		writeError(w, http.StatusServiceUnavailable, "AI features are disabled (GEMINI_API_KEY not configured)")
		return
	}
	user, ok := auth.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req bulkRegenerateDescriptionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Items) == 0 {
		writeError(w, http.StatusBadRequest, "items is required")
		return
	}

	ctx := r.Context()

	// resolve settings before claiming the batch, so a failure here never
	// leaves it stuck "running" with no goroutine to release it
	model, prompt, err := s.resolveGeminiConfig(ctx, gemini.DescriptionRegeneration)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	batchRequests := make([]inventory.DescriptionBatchRequest, 0, len(req.Items))
	assetTags := make(map[int64]string, len(req.Items))
	for _, it := range req.Items {
		item, err := s.store.GetItemByID(ctx, it.ItemID)
		if err != nil {
			continue // item no longer exists; skip it rather than fail the whole batch
		}
		batchRequests = append(batchRequests, inventory.DescriptionBatchRequest{ItemID: it.ItemID, Hint: it.Hint})
		assetTags[it.ItemID] = item.AssetTag
	}
	if len(batchRequests) == 0 {
		writeError(w, http.StatusBadRequest, "no valid items")
		return
	}

	if !s.descriptionBatch.TryStart(batchRequests, assetTags) {
		writeError(w, http.StatusConflict, "a description batch is already running")
		return
	}

	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		inventory.RunDescriptionBatch(bgCtx, s.store, s.gemini, s.descriptionBatch, user.ID, model, prompt)
	}()

	w.WriteHeader(http.StatusAccepted)
}

type descriptionBatchItemResponse struct {
	ItemID      int64  `json:"item_id"`
	AssetTag    string `json:"asset_tag"`
	Hint        string `json:"hint,omitempty"`
	Status      string `json:"status"`
	Description string `json:"description,omitempty"`
	Error       string `json:"error,omitempty"`
}

// handleBulkRegenerateDescriptionStatus is polled by the frontend's
// generate-descriptions modal to show live per-item progress.
func (s *Server) handleBulkRegenerateDescriptionStatus(w http.ResponseWriter, r *http.Request) {
	running, items := s.descriptionBatch.Snapshot()
	out := make([]descriptionBatchItemResponse, 0, len(items))
	for _, it := range items {
		out = append(out, descriptionBatchItemResponse{
			ItemID:      it.ItemID,
			AssetTag:    it.AssetTag,
			Hint:        it.Hint,
			Status:      string(it.Status),
			Description: it.Description,
			Error:       it.Error,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"running": running, "items": out})
}

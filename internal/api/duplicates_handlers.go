package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/pborges/aiinventory/internal/auth"
	"github.com/pborges/aiinventory/internal/gemini"
	"github.com/pborges/aiinventory/internal/inventory"
	"github.com/pborges/aiinventory/internal/store"
)

type duplicateStatusResponse struct {
	Running   bool   `json:"running"`
	StartedAt string `json:"started_at,omitempty"`
}

// handleDuplicatesStatus polls the in-memory Runner directly — no DB read
// needed for "is it running" (see README's Data model notes).
func (s *Server) handleDuplicatesStatus(w http.ResponseWriter, r *http.Request) {
	running, startedAt, _ := s.duplicateRunner.Status()
	resp := duplicateStatusResponse{Running: running}
	if running {
		resp.StartedAt = startedAt.UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleStartDuplicateRun kicks off a background run (README flow #5).
// Only one run may be active at a time, enforced by Runner.TryStart, not a
// DB constraint. The actual work runs in a goroutine using a context
// independent of this request, since the request's context is cancelled
// the moment this handler returns.
func (s *Server) handleStartDuplicateRun(w http.ResponseWriter, r *http.Request) {
	if s.geminiClient() == nil {
		writeError(w, http.StatusServiceUnavailable, "AI features are disabled (configure a Gemini API key in Settings)")
		return
	}
	user, ok := auth.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// resolve settings before claiming the runner, so a failure here never
	// leaves it stuck "running" with no goroutine to release it
	model, prompt, err := s.resolveGeminiConfig(r.Context(), gemini.DuplicateDetection)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if !s.duplicateRunner.TryStart(user.ID) {
		writeError(w, http.StatusConflict, "a duplicate-finder run is already in progress")
		return
	}

	client := s.geminiClient()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := inventory.RunDetection(ctx, s.store, client, s.duplicateRunner, user.ID, model, prompt); err != nil {
			log.Printf("duplicate finder run failed: %v", err)
		}
	}()

	w.WriteHeader(http.StatusAccepted)
}

type duplicateGroupMemberResponse struct {
	ItemID   int64  `json:"item_id"`
	AssetTag string `json:"asset_tag"`
}

type duplicateGroupResponse struct {
	ID        int64                          `json:"id"`
	Items     []duplicateGroupMemberResponse `json:"items"`
	Reasoning string                         `json:"reasoning"`
	CreatedAt string                         `json:"created_at"`
}

// handleListDuplicateGroups powers the duplicate finder's report view.
func (s *Server) handleListDuplicateGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := s.store.ListPendingDuplicateGroups(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]duplicateGroupResponse, 0, len(groups))
	for _, g := range groups {
		items := make([]duplicateGroupMemberResponse, 0, len(g.Items))
		for _, m := range g.Items {
			items = append(items, duplicateGroupMemberResponse{ItemID: m.ItemID, AssetTag: m.AssetTag})
		}
		out = append(out, duplicateGroupResponse{
			ID:        g.ID,
			Items:     items,
			Reasoning: g.Reasoning,
			CreatedAt: g.CreatedAt.Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": out})
}

// handleDismissDuplicateGroup marks a group "not a duplicate".
func (s *Server) handleDismissDuplicateGroup(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid group id")
		return
	}
	if err := s.store.DismissDuplicateGroup(r.Context(), id, user.ID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found or already resolved")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type mergeDuplicateGroupRequest struct {
	SurvivorItemID int64  `json:"survivor_item_id"`
	LocationID     *int64 `json:"location_id"`
}

// handleMergeDuplicateGroup resolves a group by consolidating every other
// member into survivor_item_id.
func (s *Server) handleMergeDuplicateGroup(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid group id")
		return
	}
	var req mergeDuplicateGroupRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := s.store.MergeDuplicateGroup(r.Context(), user.ID, id, req.SurvivorItemID, req.LocationID); err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			writeError(w, http.StatusNotFound, "not found")
		case errors.Is(err, store.ErrGroupNotPending):
			writeError(w, http.StatusConflict, "group is no longer pending")
		case errors.Is(err, store.ErrNotGroupMember):
			writeError(w, http.StatusBadRequest, "survivor is not a member of this group")
		default:
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

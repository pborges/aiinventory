package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/pborges/aiinventory/internal/auth"
	"github.com/pborges/aiinventory/internal/domain"
	"github.com/pborges/aiinventory/internal/store"
)

type labelResponse struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

func toLabelResponse(l domain.Label) labelResponse {
	return labelResponse{ID: l.ID, Name: l.Name, Color: l.Color}
}

func toLabelResponses(labels []domain.Label) []labelResponse {
	out := make([]labelResponse, 0, len(labels))
	for _, l := range labels {
		out = append(out, toLabelResponse(l))
	}
	return out
}

// handleListLabels powers the Settings label-management section and the
// label-cloud pickers on the Search and item detail views.
func (s *Server) handleListLabels(w http.ResponseWriter, r *http.Request) {
	labels, err := s.store.ListLabels(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"labels": toLabelResponses(labels)})
}

type labelRequest struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

func (req labelRequest) validate() string {
	if strings.TrimSpace(req.Name) == "" {
		return "name is required"
	}
	if strings.TrimSpace(req.Color) == "" {
		return "color is required"
	}
	return ""
}

// handleCreateLabel creates a new label from the Settings label-management form.
func (s *Server) handleCreateLabel(w http.ResponseWriter, r *http.Request) {
	var req labelRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if msg := req.validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	label, err := s.store.CreateLabel(r.Context(), req.Name, req.Color)
	if err != nil {
		if errors.Is(err, store.ErrLabelNameTaken) {
			writeError(w, http.StatusConflict, "label name taken")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]labelResponse{"label": toLabelResponse(label)})
}

// handleUpdateLabel renames and/or recolors an existing label.
func (s *Server) handleUpdateLabel(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid label id")
		return
	}
	var req labelRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if msg := req.validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	if err := s.store.UpdateLabel(r.Context(), id, req.Name, req.Color); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		if errors.Is(err, store.ErrLabelNameTaken) {
			writeError(w, http.StatusConflict, "label name taken")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	label, err := s.store.GetLabelByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]labelResponse{"label": toLabelResponse(label)})
}

// handleDeleteLabel removes a label entirely; ON DELETE CASCADE detaches it
// from every item it was applied to.
func (s *Server) handleDeleteLabel(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid label id")
		return
	}
	if err := s.store.DeleteLabel(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type setItemLabelsRequest struct {
	LabelIDs []int64 `json:"label_ids"`
}

// handleSetItemLabels replaces an item's full set of labels — the item
// detail view's label toggle-cloud sends the whole desired set on every
// click, same as handleReorderImages does for the image carousel.
func (s *Server) handleSetItemLabels(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid item id")
		return
	}
	var req setItemLabelsRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	ctx := r.Context()
	if err := s.store.SetItemLabelsWithActivity(ctx, user.ID, id, req.LabelIDs); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	s.handleGetItem(w, r)
}

package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/pborges/aiinventory/internal/store"
)

// handleListLocationLabels powers the Settings location-label management
// section and the locations view's sidebar filter label cloud. Reuses
// labelRequest/labelResponse from labels_handlers.go — the shape (name +
// color) is identical to item labels, only the backing table differs.
func (s *Server) handleListLocationLabels(w http.ResponseWriter, r *http.Request) {
	labels, err := s.store.ListLocationLabels(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"labels": toLabelResponses(labels)})
}

// handleCreateLocationLabel creates a new location label from the Settings
// location-label management form.
func (s *Server) handleCreateLocationLabel(w http.ResponseWriter, r *http.Request) {
	var req labelRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if msg := req.validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	label, err := s.store.CreateLocationLabel(r.Context(), req.Name, req.Color)
	if err != nil {
		if errors.Is(err, store.ErrLocationLabelNameTaken) {
			writeError(w, http.StatusConflict, "label name taken")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]labelResponse{"label": toLabelResponse(label)})
}

// handleUpdateLocationLabel renames and/or recolors an existing location label.
func (s *Server) handleUpdateLocationLabel(w http.ResponseWriter, r *http.Request) {
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

	if err := s.store.UpdateLocationLabel(r.Context(), id, req.Name, req.Color); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		if errors.Is(err, store.ErrLocationLabelNameTaken) {
			writeError(w, http.StatusConflict, "label name taken")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	label, err := s.store.GetLocationLabelByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]labelResponse{"label": toLabelResponse(label)})
}

// handleDeleteLocationLabel removes a location label entirely; ON DELETE
// CASCADE detaches it from every location it was applied to.
func (s *Server) handleDeleteLocationLabel(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid label id")
		return
	}
	if err := s.store.DeleteLocationLabel(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

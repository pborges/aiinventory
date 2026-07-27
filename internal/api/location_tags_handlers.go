package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/pborges/aiinventory/internal/store"
)

// handleListLocationTags powers the Settings location-tag management
// section and the locations view's sidebar filter tag cloud. Reuses
// tagRequest/tagResponse from tags_handlers.go — the shape (name + color) is
// identical to item tags, only the backing table differs.
func (s *Server) handleListLocationTags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.store.ListLocationTags(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tags": toTagResponses(tags)})
}

// handleCreateLocationTag creates a new location tag from the Settings
// location-tag management form.
func (s *Server) handleCreateLocationTag(w http.ResponseWriter, r *http.Request) {
	var req tagRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg := req.validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	tag, err := s.store.CreateLocationTag(r.Context(), req.Name, req.Color)
	if err != nil {
		if errors.Is(err, store.ErrLocationTagNameTaken) {
			writeError(w, http.StatusConflict, "tag name taken")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]tagResponse{"tag": toTagResponse(tag)})
}

// handleUpdateLocationTag renames and/or recolors an existing location tag.
func (s *Server) handleUpdateLocationTag(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid tag id")
		return
	}
	var req tagRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg := req.validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	if err := s.store.UpdateLocationTag(r.Context(), id, req.Name, req.Color); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		if errors.Is(err, store.ErrLocationTagNameTaken) {
			writeError(w, http.StatusConflict, "tag name taken")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	tag, err := s.store.GetLocationTagByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]tagResponse{"tag": toTagResponse(tag)})
}

// handleDeleteLocationTag removes a location tag entirely; ON DELETE CASCADE
// detaches it from every location it was applied to.
func (s *Server) handleDeleteLocationTag(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid tag id")
		return
	}
	if err := s.store.DeleteLocationTag(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

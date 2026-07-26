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

type tagResponse struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

func toTagResponse(t domain.Tag) tagResponse {
	return tagResponse{ID: t.ID, Name: t.Name, Color: t.Color}
}

func toTagResponses(tags []domain.Tag) []tagResponse {
	out := make([]tagResponse, 0, len(tags))
	for _, t := range tags {
		out = append(out, toTagResponse(t))
	}
	return out
}

// handleListTags powers the Settings tag-management section and the
// tag-cloud pickers on the Search and item detail views.
func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.store.ListTags(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tags": toTagResponses(tags)})
}

type tagRequest struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

func (req tagRequest) validate() string {
	if strings.TrimSpace(req.Name) == "" {
		return "name is required"
	}
	if strings.TrimSpace(req.Color) == "" {
		return "color is required"
	}
	return ""
}

// handleCreateTag creates a new tag from the Settings tag-management form.
func (s *Server) handleCreateTag(w http.ResponseWriter, r *http.Request) {
	var req tagRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg := req.validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	tag, err := s.store.CreateTag(r.Context(), req.Name, req.Color)
	if err != nil {
		if errors.Is(err, store.ErrTagNameTaken) {
			writeError(w, http.StatusConflict, "tag name taken")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]tagResponse{"tag": toTagResponse(tag)})
}

// handleUpdateTag renames and/or recolors an existing tag.
func (s *Server) handleUpdateTag(w http.ResponseWriter, r *http.Request) {
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

	if err := s.store.UpdateTag(r.Context(), id, req.Name, req.Color); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		if errors.Is(err, store.ErrTagNameTaken) {
			writeError(w, http.StatusConflict, "tag name taken")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	tag, err := s.store.GetTagByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]tagResponse{"tag": toTagResponse(tag)})
}

// handleDeleteTag removes a tag entirely; ON DELETE CASCADE detaches it from
// every item it was applied to.
func (s *Server) handleDeleteTag(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid tag id")
		return
	}
	if err := s.store.DeleteTag(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type setItemTagsRequest struct {
	TagIDs []int64 `json:"tag_ids"`
}

// handleSetItemTags replaces an item's full set of tags — the item detail
// view's tag toggle-cloud sends the whole desired set on every click, same
// as handleReorderImages does for the image carousel.
func (s *Server) handleSetItemTags(w http.ResponseWriter, r *http.Request) {
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
	var req setItemTagsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx := r.Context()
	if err := s.store.SetItemTags(ctx, id, req.TagIDs); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := s.store.LogActivity(ctx, user.ID, domain.ActivityItemTagsUpdated, &id, nil, ""); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	s.handleGetItem(w, r)
}

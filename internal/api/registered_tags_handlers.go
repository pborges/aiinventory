package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pborges/aiinventory/internal/domain"
	"github.com/pborges/aiinventory/internal/store"
)

// registeredTagResponse is shared by the asset-tag and location-tag
// registry handlers — same shape, different underlying table.
type registeredTagResponse struct {
	ID        int64  `json:"id"`
	Tag       string `json:"tag"`
	CreatedAt string `json:"created_at"`
	// Assigned is true when this tag is currently in use by an item/
	// location — the registry list view groups on this and hides delete
	// for assigned entries.
	Assigned bool `json:"assigned"`
}

func toRegisteredAssetTagResponse(t domain.RegisteredAssetTag) registeredTagResponse {
	return registeredTagResponse{ID: t.ID, Tag: t.Tag, CreatedAt: t.CreatedAt.Format(time.RFC3339), Assigned: t.Assigned}
}

func toRegisteredLocationTagResponse(t domain.RegisteredLocationTag) registeredTagResponse {
	return registeredTagResponse{ID: t.ID, Tag: t.LocationTag, CreatedAt: t.CreatedAt.Format(time.RFC3339), Assigned: t.Assigned}
}

type registeredTagUploadResponse struct {
	Added   int `json:"added"`
	Skipped int `json:"skipped"`
}

// handleListRegisteredAssetTags powers the Settings asset-tag registry
// section's list view.
func (s *Server) handleListRegisteredAssetTags(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.ListRegisteredAssetTagRows(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]registeredTagResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, toRegisteredAssetTagResponse(row))
	}
	writeJSON(w, http.StatusOK, map[string]any{"tags": out})
}

type registeredTagRequest struct {
	Tag string `json:"tag"`
}

// handleCreateRegisteredAssetTag adds a single tag to the asset-tag
// registry from the Settings form — idempotent, same as the bulk upload.
func (s *Server) handleCreateRegisteredAssetTag(w http.ResponseWriter, r *http.Request) {
	var req registeredTagRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	tag := strings.ToUpper(strings.TrimSpace(req.Tag))
	if !assetTagPattern.MatchString(tag) {
		writeError(w, http.StatusBadRequest, "tag must be exactly 4 uppercase letters")
		return
	}

	ctx := r.Context()
	if err := s.store.RegisterAssetTag(ctx, tag); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	row, err := s.store.GetRegisteredAssetTagByTag(ctx, tag)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]registeredTagResponse{"tag": toRegisteredAssetTagResponse(row)})
}

// handleDeleteRegisteredAssetTag removes a single entry from the asset-tag
// registry. Registry entries support create/bulk-create/list/delete only —
// no edit.
func (s *Server) handleDeleteRegisteredAssetTag(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.store.DeleteRegisteredAssetTag(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleUploadRegisteredAssetTags bulk-imports a .txt file, one tag per
// line — the write side of the label-printer script's bulk import. If any
// line fails the shape check, the whole upload is rejected (naming the bad
// lines) rather than partially importing a garbled file.
func (s *Server) handleUploadRegisteredAssetTags(w http.ResponseWriter, r *http.Request) {
	data, err := readUploadedTextFile(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid file upload")
		return
	}
	valid, invalid := parseTagLines(data, assetTagPattern)
	if len(invalid) > 0 {
		writeError(w, http.StatusBadRequest, "invalid lines (must be exactly 4 uppercase letters): "+strings.Join(invalid, ", "))
		return
	}
	added, skipped, err := s.store.BulkRegisterAssetTags(r.Context(), valid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, registeredTagUploadResponse{Added: added, Skipped: skipped})
}

// handleListRegisteredLocationTags powers the Settings location-tag
// registry section's list view.
func (s *Server) handleListRegisteredLocationTags(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.ListRegisteredLocationTagRows(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]registeredTagResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, toRegisteredLocationTagResponse(row))
	}
	writeJSON(w, http.StatusOK, map[string]any{"tags": out})
}

// handleCreateRegisteredLocationTag adds a single tag to the location-tag
// registry from the Settings form — idempotent, same as the bulk upload.
func (s *Server) handleCreateRegisteredLocationTag(w http.ResponseWriter, r *http.Request) {
	var req registeredTagRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	tag := strings.ToUpper(strings.TrimSpace(req.Tag))
	if !locationTagPattern.MatchString(tag) {
		writeError(w, http.StatusBadRequest, `tag must be "@" followed by exactly 3 uppercase letters`)
		return
	}

	ctx := r.Context()
	if err := s.store.RegisterLocationTag(ctx, tag); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	row, err := s.store.GetRegisteredLocationTagByTag(ctx, tag)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]registeredTagResponse{"tag": toRegisteredLocationTagResponse(row)})
}

// handleDeleteRegisteredLocationTag removes a single entry from the
// location-tag registry. Registry entries support create/bulk-create/list/
// delete only — no edit.
func (s *Server) handleDeleteRegisteredLocationTag(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.store.DeleteRegisteredLocationTag(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleUploadRegisteredLocationTags bulk-imports a .txt file, one tag per
// line. If any line fails the shape check, the whole upload is rejected
// (naming the bad lines) rather than partially importing a garbled file.
func (s *Server) handleUploadRegisteredLocationTags(w http.ResponseWriter, r *http.Request) {
	data, err := readUploadedTextFile(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid file upload")
		return
	}
	valid, invalid := parseTagLines(data, locationTagPattern)
	if len(invalid) > 0 {
		writeError(w, http.StatusBadRequest, `invalid lines (must be "@" followed by exactly 3 uppercase letters): `+strings.Join(invalid, ", "))
		return
	}
	added, skipped, err := s.store.BulkRegisterLocationTags(r.Context(), valid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, registeredTagUploadResponse{Added: added, Skipped: skipped})
}

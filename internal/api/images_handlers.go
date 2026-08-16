package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/pborges/aiinventory/internal/store"
)

// handleGetImage serves a single image's raw bytes so the frontend can use
// plain <img src="/api/images/{id}"> tags rather than shipping base64 blobs
// inside JSON search/item responses.
func (s *Server) handleGetImage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid image id")
		return
	}

	img, err := s.store.GetImageByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.Header().Set("Content-Type", img.ContentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Write(img.Data)
}

package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/pborges/aiinventory/internal/auth"
	"github.com/pborges/aiinventory/internal/domain"
	"github.com/pborges/aiinventory/internal/gemini"
	"github.com/pborges/aiinventory/internal/inventory"
	"github.com/pborges/aiinventory/internal/store"
)

type itemImageResponse struct {
	ID          int64  `json:"id"`
	Description string `json:"description"`
	SortOrder   int    `json:"sort_order"`
}

type activityResponse struct {
	Username  string `json:"username"`
	Action    string `json:"action"`
	Detail    string `json:"detail,omitempty"`
	CreatedAt string `json:"created_at"`
}

type itemDetailResponse struct {
	ID           int64               `json:"id"`
	AssetTag     string              `json:"asset_tag"`
	Description  string              `json:"description"`
	LocationID   *int64              `json:"location_id,omitempty"`
	LocationCode string              `json:"location_code,omitempty"`
	Images       []itemImageResponse `json:"images"`
	Activity     []activityResponse  `json:"activity"`
}

func toActivityResponse(a domain.Activity) activityResponse {
	return activityResponse{
		Username:  a.Username,
		Action:    string(a.Action),
		Detail:    a.Detail,
		CreatedAt: a.CreatedAt.Format(time.RFC3339),
	}
}

// handleGetItem powers the item detail/edit view (README flow #6): the
// carousel's images, the consolidated description, the location (if any),
// and the per-item activity log.
func (s *Server) handleGetItem(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid item id")
		return
	}

	ctx := r.Context()
	item, err := s.store.GetItemByID(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	images, err := s.store.ListImageMetaByItem(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	imageResponses := make([]itemImageResponse, 0, len(images))
	for _, img := range images {
		imageResponses = append(imageResponses, itemImageResponse{ID: img.ID, Description: img.Description, SortOrder: img.SortOrder})
	}

	var locationCode string
	if item.LocationID != nil {
		if loc, err := s.store.GetLocationByID(ctx, *item.LocationID); err == nil {
			locationCode = loc.Code
		}
	}

	activity, err := s.store.ListActivityForItem(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	activityResponses := make([]activityResponse, 0, len(activity))
	for _, a := range activity {
		activityResponses = append(activityResponses, toActivityResponse(a))
	}

	writeJSON(w, http.StatusOK, itemDetailResponse{
		ID:           item.ID,
		AssetTag:     item.AssetTag,
		Description:  item.Description,
		LocationID:   item.LocationID,
		LocationCode: locationCode,
		Images:       imageResponses,
		Activity:     activityResponses,
	})
}

type updateItemRequest struct {
	Description string `json:"description"`
}

// handleUpdateItem handles a manual (human-edited, not AI-regenerated)
// description edit from the item detail view.
func (s *Server) handleUpdateItem(w http.ResponseWriter, r *http.Request) {
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
	var req updateItemRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx := r.Context()
	if err := s.store.UpdateItemDescription(ctx, id, req.Description); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := s.store.LogActivity(ctx, user.ID, domain.ActivityDescriptionEdited, &id, nil, ""); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	s.handleGetItem(w, r)
}

type reorderImagesRequest struct {
	ImageIDs []int64 `json:"image_ids"`
}

// handleReorderImages backs the item detail view's drag-to-reorder
// carousel — the first ID in the list becomes the primary image.
func (s *Server) handleReorderImages(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.CurrentUser(r.Context()); !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid item id")
		return
	}
	var req reorderImagesRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := s.store.ReorderImages(r.Context(), id, req.ImageIDs); err != nil {
		writeError(w, http.StatusBadRequest, "invalid image order: "+err.Error())
		return
	}

	s.handleGetItem(w, r)
}

// handleDeleteImage removes a single photo from an item — for cleaning up
// a bad or duplicate capture without deleting the whole item.
func (s *Server) handleDeleteImage(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	itemID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid item id")
		return
	}
	imageID, err := strconv.ParseInt(r.PathValue("imageId"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid image id")
		return
	}

	ctx := r.Context()
	if err := s.store.DeleteImage(ctx, itemID, imageID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := s.store.LogActivity(ctx, user.ID, domain.ActivityImageDeleted, &itemID, nil, ""); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	s.handleGetItem(w, r)
}

// handleRegenerateItemDescription is the item detail view's single-item
// "Generate description" action: Gemini reviews every per-image note
// attached to the item and consolidates them into one description (the
// same logic as the Search view's bulk action, invoked here for just one
// item and returning the full refreshed item detail).
func (s *Server) handleRegenerateItemDescription(w http.ResponseWriter, r *http.Request) {
	if s.gemini == nil {
		writeError(w, http.StatusServiceUnavailable, "AI features are disabled (GEMINI_API_KEY not configured)")
		return
	}
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

	ctx := r.Context()
	model, prompt, err := s.resolveGeminiConfig(ctx, gemini.DescriptionRegeneration)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if _, err := inventory.RegenerateDescription(ctx, s.store, s.gemini, user.ID, model, prompt, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeError(w, http.StatusBadGateway, "gemini request failed: "+err.Error())
		return
	}

	s.handleGetItem(w, r)
}

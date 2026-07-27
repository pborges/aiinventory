package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/pborges/aiinventory/internal/auth"
	"github.com/pborges/aiinventory/internal/domain"
	"github.com/pborges/aiinventory/internal/inventory"
	"github.com/pborges/aiinventory/internal/store"
)

type locationResponse struct {
	ID          int64  `json:"id"`
	Code        string `json:"code"`
	Description string `json:"description,omitempty"`
}

func toLocationResponse(loc domain.Location) locationResponse {
	return locationResponse{ID: loc.ID, Code: loc.Code, Description: loc.Description}
}

// handleListLocations powers the location view's sidebar (README flow #4).
func (s *Server) handleListLocations(w http.ResponseWriter, r *http.Request) {
	locations, err := s.store.ListLocations(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]locationResponse, 0, len(locations))
	for _, loc := range locations {
		out = append(out, toLocationResponse(loc))
	}
	writeJSON(w, http.StatusOK, map[string]any{"locations": out})
}

type updateLocationRequest struct {
	Description string `json:"description"`
}

// handleUpdateLocation sets (or clears) a location's optional description —
// the locations view's under-the-code editor for the selected location.
func (s *Server) handleUpdateLocation(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid location id")
		return
	}
	var req updateLocationRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx := r.Context()
	if err := s.store.UpdateLocationDescription(ctx, id, req.Description); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	loc, err := s.store.GetLocationByID(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]locationResponse{"location": toLocationResponse(loc)})
}

type locationItemResponse struct {
	ID          int64               `json:"id"`
	AssetTag    string              `json:"asset_tag"`
	Description string              `json:"description"`
	Images      []itemImageResponse `json:"images"`
}

// handleGetLocationItems returns the items currently linked to a location
// with their full image set (not just the primary image), for the location
// view's "live carousel" item cards.
func (s *Server) handleGetLocationItems(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid location id")
		return
	}

	ctx := r.Context()
	items, err := s.store.ListItemsByLocation(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := make([]locationItemResponse, 0, len(items))
	for _, it := range items {
		images, err := s.store.ListImageMetaByItem(ctx, it.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		imgResp := make([]itemImageResponse, 0, len(images))
		for _, img := range images {
			imgResp = append(imgResp, itemImageResponse{ID: img.ID, Description: img.Description, SortOrder: img.SortOrder})
		}
		out = append(out, locationItemResponse{ID: it.ID, AssetTag: it.AssetTag, Description: it.Description, Images: imgResp})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// handleGetLocationActivity powers the location view's footer activity log.
func (s *Server) handleGetLocationActivity(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid location id")
		return
	}
	activity, err := s.store.ListActivityForLocation(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]activityResponse, 0, len(activity))
	for _, a := range activity {
		out = append(out, toActivityResponse(a))
	}
	writeJSON(w, http.StatusOK, map[string]any{"activity": out})
}

type moveItemRequest struct {
	ItemID int64 `json:"item_id"`
}

// handleMoveItem backs the location view's drag-and-drop item card ->
// location sidebar entry relocation.
func (s *Server) handleMoveItem(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	locationID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid location id")
		return
	}
	var req moveItemRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	item, err := inventory.MoveItemToLocation(r.Context(), s.store, user.ID, req.ItemID, locationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item_id": item.ID, "location_id": locationID})
}

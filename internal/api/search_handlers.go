package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/pborges/aiinventory/internal/auth"
	"github.com/pborges/aiinventory/internal/store"
)

type itemSummaryResponse struct {
	ID                  int64           `json:"id"`
	AssetTag            string          `json:"asset_tag"`
	Description         string          `json:"description"`
	LocationTag         string          `json:"location_tag,omitempty"`
	LocationDescription string          `json:"location_description,omitempty"`
	PrimaryImageID      *int64          `json:"primary_image_id,omitempty"`
	Labels              []labelResponse `json:"labels"`
}

// handleSearch implements the Search view (README flow #3): a free-text
// query plus the no-description/no-location/no-photo/specific-location filters.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	params := store.SearchParams{
		Query:         q.Get("q"),
		NoDescription: q.Get("no_description") == "1",
		NoLocation:    q.Get("no_location") == "1",
		NoPhoto:       q.Get("no_photo") == "1",
	}
	if v := q.Get("location_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid location_id")
			return
		}
		params.LocationID = &id
	}
	for _, v := range q["label_id"] {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid label_id")
			return
		}
		params.LabelIDs = append(params.LabelIDs, id)
	}
	for _, v := range q["location_label_id"] {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid location_label_id")
			return
		}
		params.LocationLabelIDs = append(params.LocationLabelIDs, id)
	}

	results, err := s.store.SearchItems(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := make([]itemSummaryResponse, 0, len(results))
	for _, it := range results {
		out = append(out, itemSummaryResponse{
			ID:                  it.ID,
			AssetTag:            it.AssetTag,
			Description:         it.Description,
			LocationTag:         it.LocationTag,
			LocationDescription: it.LocationDescription,
			PrimaryImageID:      it.PrimaryImageID,
			Labels:              toLabelResponses(it.Labels),
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
	if !decodeJSON(w, r, &req) {
		return
	}

	if err := s.store.DeleteItemsWithActivity(r.Context(), user.ID, req.ItemIDs); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "one or more items were not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"deleted": len(req.ItemIDs)})
}

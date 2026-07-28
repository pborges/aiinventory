package api

import (
	"io"
	"net/http"
	"regexp"

	"github.com/pborges/aiinventory/internal/auth"
	"github.com/pborges/aiinventory/internal/domain"
	"github.com/pborges/aiinventory/internal/gemini"
	"github.com/pborges/aiinventory/internal/inventory"
)

// locationCodePattern mirrors assetTagPattern (capture_handlers.go): Gemini's
// JSON schema only constrains these to strings, not their shape, so a
// misread (extra/missing letter, a stray digit, lowercase, etc.) can slip
// through as valid-looking JSON. Both flows need this deterministic check
// before their result is trusted.
var locationCodePattern = regexp.MustCompile(`^@[A-Z]{3}$`)

type movedItemResponse struct {
	AssetTag     string `json:"asset_tag"`
	FromLocation string `json:"from_location,omitempty"`
}

type reconcileDiffResponse struct {
	HasLocationCode bool   `json:"has_location_code"`
	LocationCode    string `json:"location_code,omitempty"`
	// AssetTags is the raw list Gemini read from the frame — the frontend
	// echoes it back verbatim to /api/reconcile/apply once the user approves.
	AssetTags []string            `json:"asset_tags"`
	New       []string            `json:"new"`
	Added     []string            `json:"added"`
	Moved     []movedItemResponse `json:"moved"`
	Removed   []string            `json:"removed"`
}

func toReconcileDiffResponse(diff domain.ReconcileDiff, assetTags []string) reconcileDiffResponse {
	moved := make([]movedItemResponse, 0, len(diff.Moved))
	for _, m := range diff.Moved {
		moved = append(moved, movedItemResponse{AssetTag: m.AssetTag, FromLocation: m.FromLocation})
	}
	newTags := diff.New
	if newTags == nil {
		newTags = []string{}
	}
	added := diff.Added
	if added == nil {
		added = []string{}
	}
	removed := diff.Removed
	if removed == nil {
		removed = []string{}
	}
	if assetTags == nil {
		assetTags = []string{}
	}
	return reconcileDiffResponse{
		HasLocationCode: true,
		LocationCode:    diff.LocationCode,
		AssetTags:       assetTags,
		New:             newTags,
		Added:           added,
		Moved:           moved,
		Removed:         removed,
	}
}

// handleReconcilePreview implements the read-only half of README flow #2:
// a captured photo is analyzed for a location code + visible asset tags,
// and the resulting diff is returned for the user to approve — nothing is
// written yet.
func (s *Server) handleReconcilePreview(w http.ResponseWriter, r *http.Request) {
	if s.geminiClient() == nil {
		writeError(w, http.StatusServiceUnavailable, "AI features are disabled (configure a Gemini API key in Settings)")
		return
	}
	if _, ok := auth.CurrentUser(r.Context()); !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	file, header, err := r.FormFile("image")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing image file")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxUploadBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read image")
		return
	}
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg"
	}

	ctx := r.Context()
	model, prompt, err := s.resolveGeminiConfig(ctx, gemini.LocationReconciliation)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	analysis, err := s.geminiClient().AnalyzeReconciliation(ctx, model, prompt, data, contentType)
	if err != nil {
		writeError(w, http.StatusBadGateway, "gemini request failed: "+err.Error())
		return
	}

	if !analysis.HasLocationCode || !locationCodePattern.MatchString(analysis.LocationCode) {
		writeJSON(w, http.StatusOK, reconcileDiffResponse{HasLocationCode: false})
		return
	}
	for _, tag := range analysis.AssetTags {
		if !assetTagPattern.MatchString(tag) {
			// Gemini read a well-formed location code but at least one
			// asset tag failed the deterministic shape check — likely an
			// OCR misread. Reject the whole preview rather than reconciling
			// against a partially-garbled tag set, which could show real
			// items as falsely "removed" from this location.
			writeJSON(w, http.StatusOK, reconcileDiffResponse{HasLocationCode: false})
			return
		}
	}

	diff, err := inventory.ComputeReconciliation(ctx, s.store, analysis.LocationCode, analysis.AssetTags)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toReconcileDiffResponse(diff, analysis.AssetTags))
}

type applyReconcileRequest struct {
	LocationCode string   `json:"location_code"`
	AssetTags    []string `json:"asset_tags"`
}

// handleReconcileApply is the write half: the frontend resubmits the same
// (location_code, asset_tags) the user approved in the preview. The diff is
// recomputed fresh here (never trusting a client-supplied diff) so a stale
// approval can't silently apply against changed state, then written
// atomically by store.ApplyReconciliation.
func (s *Server) handleReconcileApply(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req applyReconcileRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !locationCodePattern.MatchString(req.LocationCode) {
		writeError(w, http.StatusBadRequest, `location_code must be "@" followed by exactly 3 uppercase letters`)
		return
	}
	for _, tag := range req.AssetTags {
		if !assetTagPattern.MatchString(tag) {
			writeError(w, http.StatusBadRequest, "asset_tags must each be exactly 4 uppercase letters")
			return
		}
	}

	ctx := r.Context()
	diff, err := inventory.ComputeReconciliation(ctx, s.store, req.LocationCode, req.AssetTags)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := s.store.ApplyReconciliation(ctx, user.ID, diff); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, toReconcileDiffResponse(diff, req.AssetTags))
}

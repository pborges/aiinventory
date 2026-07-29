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

// locationTagPattern mirrors assetTagPattern (capture_handlers.go): Gemini's
// JSON schema only constrains these to strings, not their shape, so a
// misread (extra/missing letter, a stray digit, lowercase, etc.) can slip
// through as valid-looking JSON. Both flows need this deterministic check
// before their result is trusted.
var locationTagPattern = regexp.MustCompile(`^@[A-Z]{3}$`)

type movedItemResponse struct {
	AssetTag     string `json:"asset_tag"`
	FromLocation string `json:"from_location,omitempty"`
}

// tagResolutionResponse mirrors one inventory.TagMatch — see
// capture_handlers.go's capturePreviewResponse for the same contract used by
// the item-ingest flow. Corrected is never trusted to auto-apply here
// either: a single OCR read (or even two reads that consistently produce
// the same misread) has no way to tell "typo of an existing tag" apart from
// "coincidentally similar but genuinely different new tag," so it's always
// surfaced as a single-candidate suggestion, never silently substituted.
type tagResolutionResponse struct {
	Raw        string   `json:"raw"`
	Status     string   `json:"status"` // exact | corrected | ambiguous | no_match
	Candidates []string `json:"candidates,omitempty"`
}

type reconcileDiffResponse struct {
	HasLocationTag bool   `json:"has_location_tag"`
	LocationTag    string `json:"location_tag,omitempty"`
	// RawLocationTag is what Gemini actually read, always set alongside
	// HasLocationTag on a fresh preview — mirrors capturePreviewResponse's
	// RawAssetTag. The deterministic registry check runs on top of this, it
	// never replaces it.
	RawLocationTag string `json:"raw_location_tag,omitempty"`
	// LocationTagCorrected is true when there's a single, confident
	// distance-1 registry candidate for RawLocationTag. Never auto-applied —
	// same never-silently-substituted rule as asset tags — it's only a
	// pre-selected suggestion (LocationTagCandidates[0]) the operator must
	// still confirm.
	LocationTagCorrected bool `json:"location_tag_corrected,omitempty"`
	// LocationTagNeedsResolution is true whenever the read isn't an exact
	// registry match. The diff above is still computed against the raw read
	// (see handleReconcilePreview) so there's something to show while the
	// operator resolves it; the frontend refetches via /api/reconcile/diff
	// once a possibly-different location tag is confirmed.
	LocationTagNeedsResolution bool     `json:"location_tag_needs_resolution,omitempty"`
	LocationTagCandidates      []string `json:"location_tag_candidates,omitempty"`
	// AssetTags is the tag list the diff was actually computed against — on
	// a fresh preview this is only the raw reads that exactly matched the
	// registry (see handleReconcilePreview); on /api/reconcile/diff and
	// /api/reconcile/apply it's whatever the caller explicitly supplied
	// (already operator-confirmed). The frontend echoes it back verbatim to
	// /api/reconcile/apply once the user approves.
	AssetTags []string            `json:"asset_tags"`
	New       []string            `json:"new"`
	Added     []string            `json:"added"`
	Moved     []movedItemResponse `json:"moved"`
	Removed   []string            `json:"removed"`
	// SuggestedRotation is only set on the first (straight) preview response
	// of the locate flow's dual-read cross-check — it tells the frontend
	// which way to rotate the same frame for the second, corroborating read.
	SuggestedRotation string `json:"suggested_rotation,omitempty"`
	// TagResolutions is only set on preview responses — one entry per raw
	// tag Gemini read, before shape-invalid ones are rejected outright.
	TagResolutions []tagResolutionResponse `json:"tag_resolutions,omitempty"`
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
		HasLocationTag: true,
		LocationTag:    diff.LocationTag,
		AssetTags:      assetTags,
		New:            newTags,
		Added:          added,
		Moved:          moved,
		Removed:        removed,
	}
}

// handleReconcilePreview implements the read-only half of README flow #2:
// a captured photo is analyzed for a location tag + visible asset tags,
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

	var found []string
	if analysis.HasLocationTag && analysis.LocationTag != "" {
		found = append(found, analysis.LocationTag)
	}
	found = append(found, analysis.AssetTags...)
	s.saveScan("locate", data, found)

	if !analysis.HasLocationTag || !locationTagPattern.MatchString(analysis.LocationTag) {
		writeJSON(w, http.StatusOK, reconcileDiffResponse{HasLocationTag: false})
		return
	}
	for _, tag := range analysis.AssetTags {
		if !assetTagPattern.MatchString(tag) {
			// Gemini read a well-formed location tag but at least one
			// asset tag failed the deterministic shape check — likely an
			// OCR misread. Reject the whole preview rather than reconciling
			// against a partially-garbled tag set, which could show real
			// items as falsely "removed" from this location.
			writeJSON(w, http.StatusOK, reconcileDiffResponse{HasLocationTag: false})
			return
		}
	}

	registeredLocationTags, err := s.store.ListRegisteredLocationTags(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	locationMatch := inventory.ResolveTag(analysis.LocationTag, registeredLocationTags)

	registeredAssetTags, err := s.store.ListRegisteredAssetTags(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	matches := inventory.ResolveTags(analysis.AssetTags, registeredAssetTags)
	// Only exact registry matches feed the diff — anything corrected,
	// ambiguous, or unmatched needs operator confirmation (surfaced via
	// TagResolutions below) before it can be trusted enough to reconcile
	// against.
	resolvedTags := make([]string, 0, len(matches))
	resolutions := make([]tagResolutionResponse, 0, len(matches))
	for _, m := range matches {
		switch m.Status {
		case inventory.TagStatusExact:
			resolvedTags = append(resolvedTags, m.Raw)
			resolutions = append(resolutions, tagResolutionResponse{Raw: m.Raw, Status: string(m.Status)})
		case inventory.TagStatusCorrected:
			resolutions = append(resolutions, tagResolutionResponse{Raw: m.Raw, Status: string(m.Status), Candidates: []string{m.Resolved}})
		default: // ambiguous / no_match
			resolutions = append(resolutions, tagResolutionResponse{Raw: m.Raw, Status: string(m.Status), Candidates: m.Candidates})
		}
	}

	diff, err := inventory.ComputeReconciliation(ctx, s.store, analysis.LocationTag, resolvedTags)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	resp := toReconcileDiffResponse(diff, resolvedTags)
	resp.SuggestedRotation = analysis.SuggestedRotation
	resp.TagResolutions = resolutions
	resp.RawLocationTag = analysis.LocationTag
	switch locationMatch.Status {
	case inventory.TagStatusCorrected:
		resp.LocationTagCorrected = true
		resp.LocationTagNeedsResolution = true
		resp.LocationTagCandidates = []string{locationMatch.Resolved}
	case inventory.TagStatusAmbiguous, inventory.TagStatusNoMatch:
		resp.LocationTagNeedsResolution = true
		resp.LocationTagCandidates = locationMatch.Candidates
	}
	writeJSON(w, http.StatusOK, resp)
}

type applyReconcileRequest struct {
	LocationTag string   `json:"location_tag"`
	AssetTags   []string `json:"asset_tags"`
}

// handleReconcileDiff recomputes the read-only diff for an explicit
// (location_tag, asset_tags) pair — no Gemini call, nothing written. This
// backs the tag-agreement review step: when two analyses of the same frame
// (e.g. straight vs. rotated) disagree on which tags are visible and the
// user resolves the disagreement by hand, the resulting tag list still
// needs a fresh diff against current DB state before it can be shown.
func (s *Server) handleReconcileDiff(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.CurrentUser(r.Context()); !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req applyReconcileRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !locationTagPattern.MatchString(req.LocationTag) {
		writeError(w, http.StatusBadRequest, `location_tag must be "@" followed by exactly 3 uppercase letters`)
		return
	}
	for _, tag := range req.AssetTags {
		if !assetTagPattern.MatchString(tag) {
			writeError(w, http.StatusBadRequest, "asset_tags must each be exactly 4 uppercase letters")
			return
		}
	}

	ctx := r.Context()
	diff, err := inventory.ComputeReconciliation(ctx, s.store, req.LocationTag, req.AssetTags)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toReconcileDiffResponse(diff, req.AssetTags))
}

// handleReconcileApply is the write half: the frontend resubmits the same
// (location_tag, asset_tags) the user approved in the preview. The diff is
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
	if !locationTagPattern.MatchString(req.LocationTag) {
		writeError(w, http.StatusBadRequest, `location_tag must be "@" followed by exactly 3 uppercase letters`)
		return
	}
	for _, tag := range req.AssetTags {
		if !assetTagPattern.MatchString(tag) {
			writeError(w, http.StatusBadRequest, "asset_tags must each be exactly 4 uppercase letters")
			return
		}
	}

	ctx := r.Context()
	diff, err := inventory.ComputeReconciliation(ctx, s.store, req.LocationTag, req.AssetTags)
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

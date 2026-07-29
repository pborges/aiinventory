package api

import (
	"errors"
	"io"
	"net/http"
	"regexp"

	"github.com/pborges/aiinventory/internal/auth"
	"github.com/pborges/aiinventory/internal/gemini"
	"github.com/pborges/aiinventory/internal/inventory"
	"github.com/pborges/aiinventory/internal/store"
)

const maxUploadBytes = 10 << 20 // 10MB — captured frames are downsized client-side well below this

var assetTagPattern = regexp.MustCompile(`^[A-Z]{4}$`)

type captureResponse struct {
	HasAssetTag      bool   `json:"has_asset_tag"`
	AssetTag         string `json:"asset_tag,omitempty"`
	ItemID           int64  `json:"item_id,omitempty"`
	ItemWasNew       bool   `json:"item_was_new,omitempty"`
	ItemGuess        string `json:"item_guess,omitempty"`
	ImageDescription string `json:"image_description,omitempty"`
}

type capturePreviewResponse struct {
	HasAssetTag bool `json:"has_asset_tag"`
	// AssetTag is the final tag to use — only set when it's an exact
	// registry match, requiring no operator input. Empty whenever
	// NeedsResolution is true.
	AssetTag string `json:"asset_tag,omitempty"`
	// RawAssetTag is what Gemini actually read, always set alongside
	// HasAssetTag — the deterministic tag-registry check runs on top of
	// this, it never replaces it.
	RawAssetTag string `json:"raw_asset_tag,omitempty"`
	// Corrected is true when there's a single, confident distance-1
	// registry candidate for RawAssetTag. It still requires a one-tap
	// confirm (NeedsResolution is also true) rather than auto-applying: a
	// single OCR read has no corroborating second signal, so a genuinely
	// different new tag that happens to be one letter off from an existing
	// one must not get silently merged into it.
	Corrected bool `json:"corrected,omitempty"`
	// NeedsResolution is true whenever the read isn't an exact registry
	// match — the operator must pick a candidate (Candidates[0] is the
	// suggested one when Corrected) or type the tag manually before
	// accepting.
	NeedsResolution  bool     `json:"needs_resolution,omitempty"`
	Candidates       []string `json:"candidates,omitempty"`
	ItemGuess        string   `json:"item_guess,omitempty"`
	ImageDescription string   `json:"image_description,omitempty"`
	ItemWillBeNew    bool     `json:"item_will_be_new,omitempty"`
}

func readUploadedImage(r *http.Request) (data []byte, contentType string, err error) {
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		return nil, "", err
	}
	file, header, err := r.FormFile("image")
	if err != nil {
		return nil, "", err
	}
	defer file.Close()

	data, err = io.ReadAll(io.LimitReader(file, maxUploadBytes))
	if err != nil {
		return nil, "", err
	}

	contentType = header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg"
	}
	return data, contentType, nil
}

// handleCapturePreview implements the read-only half of README flow #1: a
// captured photo is analyzed by Gemini for an asset tag, item identity, and
// per-image notes — nothing is written to the database yet. The user
// reviews this in the UI and either cancels (nothing happens) or accepts,
// which calls handleCaptureApply with the same image plus the asset tag/
// description this endpoint returned.
func (s *Server) handleCapturePreview(w http.ResponseWriter, r *http.Request) {
	if s.geminiClient() == nil {
		writeError(w, http.StatusServiceUnavailable, "AI features are disabled (GEMINI_API_KEY not configured)")
		return
	}
	if _, ok := auth.CurrentUser(r.Context()); !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	data, contentType, err := readUploadedImage(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid image upload")
		return
	}

	ctx := r.Context()
	model, prompt, err := s.resolveGeminiConfig(ctx, gemini.TagCapture)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	analysis, err := s.geminiClient().AnalyzeTagCapture(ctx, model, prompt, data, contentType)
	if err != nil {
		writeError(w, http.StatusBadGateway, "gemini request failed: "+err.Error())
		return
	}

	var foundTags []string
	if analysis.HasAssetTag && analysis.AssetTag != "" {
		foundTags = append(foundTags, analysis.AssetTag)
	}
	s.saveScan("item", data, foundTags)

	if !analysis.HasAssetTag || !assetTagPattern.MatchString(analysis.AssetTag) {
		// A misread (wrong letter count, stray digit, lowercase, etc.) can
		// still come back as "valid" JSON from Gemini's schema — the
		// deterministic shape check must catch it here, before the user
		// ever sees an accept screen for a tag that would fail at apply.
		writeJSON(w, http.StatusOK, capturePreviewResponse{HasAssetTag: false})
		return
	}

	registeredAssetTags, err := s.store.ListRegisteredAssetTags(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	match := inventory.ResolveTags([]string{analysis.AssetTag}, registeredAssetTags)[0]

	resp := capturePreviewResponse{
		HasAssetTag:      true,
		RawAssetTag:      analysis.AssetTag,
		ItemGuess:        analysis.ItemGuess,
		ImageDescription: analysis.Description,
	}
	switch match.Status {
	case inventory.TagStatusExact:
		resp.AssetTag = match.Resolved
		if _, err := s.store.GetItemByAssetTag(ctx, match.Resolved); err == nil {
			resp.ItemWillBeNew = false
		} else if errors.Is(err, store.ErrNotFound) {
			resp.ItemWillBeNew = true
		} else {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	case inventory.TagStatusCorrected:
		// A single OCR read has no corroborating second signal (unlike the
		// locate flow's dual-read cross-check), so even a "confident,
		// unique" distance-1 match isn't auto-applied here: it's offered as
		// the pre-selected suggestion, but a genuinely different new tag
		// that happens to be one letter off from an existing one must not
		// get silently merged into it without a human looking at it.
		resp.Corrected = true
		resp.NeedsResolution = true
		resp.Candidates = []string{match.Resolved}
	default: // ambiguous / no_match
		resp.NeedsResolution = true
		resp.Candidates = match.Candidates
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleCaptureApply is the write half, called only once the user accepts a
// preview: it trusts the asset tag and description the client echoes back
// (which came from this same user's own handleCapturePreview response
// moments earlier) rather than re-running Gemini, so accepting is instant
// and never risks a different read of the same photo.
func (s *Server) handleCaptureApply(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	data, contentType, err := readUploadedImage(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid image upload")
		return
	}

	assetTag := r.FormValue("asset_tag")
	if !assetTagPattern.MatchString(assetTag) {
		writeError(w, http.StatusBadRequest, "asset_tag must be 4 uppercase letters")
		return
	}
	description := r.FormValue("description")

	ctx := r.Context()
	result, err := inventory.Capture(ctx, s.store, user.ID, true, assetTag, data, contentType, description)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, captureResponse{
		HasAssetTag:      true,
		AssetTag:         result.Item.AssetTag,
		ItemID:           result.Item.ID,
		ItemWasNew:       result.ItemWasNew,
		ImageDescription: description,
	})
}

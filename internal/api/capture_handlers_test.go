package api

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pborges/aiinventory/internal/auth"
	"github.com/pborges/aiinventory/internal/gemini"
	"github.com/pborges/aiinventory/internal/store"
)

func newTestServerWithGemini(t *testing.T, g gemini.Client) (http.Handler, []*http.Cookie, *store.Store) {
	t.Helper()
	s := store.NewTestStore(t)
	codec := auth.NewCodec("test-secret")
	h := New(s, codec, g, "")
	w := doJSON(t, h, http.MethodPost, "/api/auth/bootstrap", credentials{Username: "alice", Password: "correcthorse"}, nil)
	return h, w.Result().Cookies(), s
}

// doCaptureUpload is the composite convenience helper every OTHER test file
// uses to quickly get an item into the store: it drives the real two-step
// preview -> apply flow (mirroring what the frontend does on Accept) and
// returns the apply response, so callers that just want "a captured item
// with this photo" don't need to know the flow is two calls.
func doCaptureUpload(t *testing.T, h http.Handler, cookies []*http.Cookie, imageBytes []byte) *httptest.ResponseRecorder {
	t.Helper()
	previewResp := doMultipartUpload(t, h, "/api/capture/preview", cookies, imageBytes, nil)
	var preview capturePreviewResponse
	if err := json.NewDecoder(previewResp.Body).Decode(&preview); err != nil {
		t.Fatalf("decode preview response: %v", err)
	}
	if !preview.HasAssetTag {
		// mirrors handleCapturePreview's own response shape closely enough
		// for callers that expect "no tag" to just inspect .HasAssetTag
		return previewResp
	}
	assetTag := preview.AssetTag
	if preview.NeedsResolution {
		// No confident registry match yet (a genuinely new tag that hasn't
		// been bulk-imported) — mirror what an operator would do: accept
		// the raw read as-is. It self-heals into the registry on apply.
		assetTag = preview.RawAssetTag
	}
	return doMultipartUpload(t, h, "/api/capture/apply", cookies, imageBytes, map[string]string{
		"asset_tag":   assetTag,
		"description": preview.ImageDescription,
	})
}

// registerTestTag registers tag in the tag registry directly against the
// test store, so preview handlers see it as a known tag — mirrors what the
// external label-printing script's import would do before a tag is ever
// scanned.
func registerTestTag(t *testing.T, s *store.Store, tag string) {
	t.Helper()
	if err := s.RegisterAssetTag(context.Background(), tag); err != nil {
		t.Fatalf("registerTestTag(%q): %v", tag, err)
	}
}

// registerTestLocationTag mirrors registerTestTag for the location-tag
// registry.
func registerTestLocationTag(t *testing.T, s *store.Store, tag string) {
	t.Helper()
	if err := s.RegisterLocationTag(context.Background(), tag); err != nil {
		t.Fatalf("registerTestLocationTag(%q): %v", tag, err)
	}
}

func doMultipartUpload(t *testing.T, h http.Handler, path string, cookies []*http.Cookie, imageBytes []byte, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("image", "capture.jpg")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	part.Write(imageBytes)
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatalf("WriteField(%s): %v", k, err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestCapturePreviewWithoutGeminiConfigured(t *testing.T) {
	h, cookies, _ := newTestServerWithGemini(t, nil)
	w := doMultipartUpload(t, h, "/api/capture/preview", cookies, []byte("fake-jpeg"), nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestCapturePreviewDoesNotWriteToStore(t *testing.T) {
	fake := &gemini.Fake{
		TagCaptureResult: gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "ZKEI", ItemGuess: "cordless drill", Description: "S/N 12345"},
	}
	h, cookies, _ := newTestServerWithGemini(t, fake)

	w := doMultipartUpload(t, h, "/api/capture/preview", cookies, []byte("fake-jpeg-bytes"), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var preview capturePreviewResponse
	if err := json.NewDecoder(w.Body).Decode(&preview); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// ZKEI isn't in the (empty) tag registry yet, so this preview can't
	// confidently resolve it — it comes back needing operator resolution
	// rather than pre-filled, which is itself proof nothing was written.
	if !preview.HasAssetTag || preview.RawAssetTag != "ZKEI" || !preview.NeedsResolution {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	if preview.ItemGuess != "cordless drill" {
		t.Errorf("ItemGuess = %q", preview.ItemGuess)
	}

	// the whole point of preview: nothing is written until apply is called
	search := doJSON(t, h, http.MethodGet, "/api/search", nil, cookies)
	var searchBody struct {
		Items []itemSummaryResponse `json:"items"`
	}
	json.NewDecoder(search.Body).Decode(&searchBody)
	if len(searchBody.Items) != 0 {
		t.Fatalf("items after preview only (no apply) = %+v, want none", searchBody.Items)
	}
}

func TestCaptureApplyCreatesNewItem(t *testing.T) {
	fake := &gemini.Fake{
		TagCaptureResult: gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "ZKEI", ItemGuess: "cordless drill", Description: "S/N 12345"},
	}
	h, cookies, _ := newTestServerWithGemini(t, fake)

	w := doMultipartUpload(t, h, "/api/capture/apply", cookies, []byte("fake-jpeg-bytes"), map[string]string{
		"asset_tag":   "ZKEI",
		"description": "S/N 12345",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp captureResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.HasAssetTag || resp.AssetTag != "ZKEI" || !resp.ItemWasNew {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestCaptureApplySetItemDescriptionPromotesNoteToItemDescription(t *testing.T) {
	fake := &gemini.Fake{
		TagCaptureResult: gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "ZKEI", Description: "S/N 12345"},
	}
	h, cookies, _ := newTestServerWithGemini(t, fake)

	w := doMultipartUpload(t, h, "/api/capture/apply", cookies, []byte("fake-jpeg-bytes"), map[string]string{
		"asset_tag":            "ZKEI",
		"description":          "S/N 12345",
		"set_item_description": "1",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	search := doJSON(t, h, http.MethodGet, "/api/search", nil, cookies)
	var searchBody struct {
		Items []itemSummaryResponse `json:"items"`
	}
	json.NewDecoder(search.Body).Decode(&searchBody)
	if len(searchBody.Items) != 1 || searchBody.Items[0].Description != "S/N 12345" {
		t.Fatalf("items = %+v, want one item with description %q", searchBody.Items, "S/N 12345")
	}
}

func TestCaptureApplyWithoutSetItemDescriptionLeavesItemDescriptionEmpty(t *testing.T) {
	fake := &gemini.Fake{
		TagCaptureResult: gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "ZKEI", Description: "S/N 12345"},
	}
	h, cookies, _ := newTestServerWithGemini(t, fake)

	w := doMultipartUpload(t, h, "/api/capture/apply", cookies, []byte("fake-jpeg-bytes"), map[string]string{
		"asset_tag":   "ZKEI",
		"description": "S/N 12345",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	search := doJSON(t, h, http.MethodGet, "/api/search", nil, cookies)
	var searchBody struct {
		Items []itemSummaryResponse `json:"items"`
	}
	json.NewDecoder(search.Body).Decode(&searchBody)
	if len(searchBody.Items) != 1 || searchBody.Items[0].Description != "" {
		t.Fatalf("items = %+v, want one item with an empty description (note saved on the photo only)", searchBody.Items)
	}
}

func TestCaptureApplyRejectsInvalidAssetTag(t *testing.T) {
	h, cookies, _ := newTestServerWithGemini(t, nil) // apply doesn't need gemini configured
	w := doMultipartUpload(t, h, "/api/capture/apply", cookies, []byte("photo"), map[string]string{
		"asset_tag":   "not-valid",
		"description": "",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", w.Code, w.Body.String())
	}
}

func TestCaptureFullFlowAppendsOnSecondPhotoOfSameTag(t *testing.T) {
	fake := &gemini.Fake{
		TagCaptureResult: gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "ZKEI", Description: "note"},
	}
	h, cookies, _ := newTestServerWithGemini(t, fake)

	first := doCaptureUpload(t, h, cookies, []byte("photo-1"))
	var firstResp captureResponse
	json.NewDecoder(first.Body).Decode(&firstResp)
	if !firstResp.ItemWasNew {
		t.Fatalf("first capture ItemWasNew = false, want true")
	}

	second := doCaptureUpload(t, h, cookies, []byte("photo-2"))
	var secondResp captureResponse
	json.NewDecoder(second.Body).Decode(&secondResp)
	if secondResp.ItemWasNew {
		t.Fatalf("second capture ItemWasNew = true, want false")
	}
	if secondResp.ItemID != firstResp.ItemID {
		t.Fatalf("second capture ItemID = %d, want %d (same item)", secondResp.ItemID, firstResp.ItemID)
	}
}

func TestCapturePreviewNoAssetTagFound(t *testing.T) {
	fake := &gemini.Fake{
		TagCaptureResult: gemini.TagCaptureResult{HasAssetTag: false},
	}
	h, cookies, _ := newTestServerWithGemini(t, fake)

	w := doMultipartUpload(t, h, "/api/capture/preview", cookies, []byte("photo-with-no-tag"), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp capturePreviewResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.HasAssetTag {
		t.Fatalf("HasAssetTag = true, want false")
	}
}

func TestCapturePreviewRejectsMalformedAssetTag(t *testing.T) {
	// Gemini's JSON schema only constrains asset_tag to a string — a misread
	// (wrong letter count, stray digit, lowercase) can still come back as
	// "valid" JSON. The deterministic shape check must catch it here, before
	// the user ever sees an accept screen for it.
	for _, tag := range []string{"ZK3I", "zkei", "ZKEIX", "ZKE"} {
		fake := &gemini.Fake{
			TagCaptureResult: gemini.TagCaptureResult{HasAssetTag: true, AssetTag: tag},
		}
		h, cookies, _ := newTestServerWithGemini(t, fake)

		w := doMultipartUpload(t, h, "/api/capture/preview", cookies, []byte("photo"), nil)
		if w.Code != http.StatusOK {
			t.Fatalf("tag %q: status = %d, body = %s", tag, w.Code, w.Body.String())
		}
		var resp capturePreviewResponse
		json.NewDecoder(w.Body).Decode(&resp)
		if resp.HasAssetTag {
			t.Errorf("tag %q: HasAssetTag = true, want false", tag)
		}
	}
}

func TestCapturePreviewExistingItemNotFlaggedAsNew(t *testing.T) {
	fake := &gemini.Fake{
		TagCaptureResult: gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "ZKEI"},
	}
	h, cookies, _ := newTestServerWithGemini(t, fake)

	doCaptureUpload(t, h, cookies, []byte("photo-1")) // creates ZKEI

	w := doMultipartUpload(t, h, "/api/capture/preview", cookies, []byte("photo-2"), nil)
	var preview capturePreviewResponse
	json.NewDecoder(w.Body).Decode(&preview)
	if preview.ItemWillBeNew {
		t.Fatalf("preview for an already-existing tag reported ItemWillBeNew = true")
	}
}

func TestCapturePreviewExactRegistryMatchPassesThroughSilently(t *testing.T) {
	fake := &gemini.Fake{
		TagCaptureResult: gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "OORB"},
	}
	h, cookies, s := newTestServerWithGemini(t, fake)
	registerTestTag(t, s, "OORB")

	w := doMultipartUpload(t, h, "/api/capture/preview", cookies, []byte("photo"), nil)
	var preview capturePreviewResponse
	json.NewDecoder(w.Body).Decode(&preview)
	if preview.NeedsResolution || preview.Corrected {
		t.Fatalf("exact registry match required resolution: %+v", preview)
	}
	if preview.AssetTag != "OORB" {
		t.Fatalf("AssetTag = %q, want OORB", preview.AssetTag)
	}
}

func TestCapturePreviewFlagsCorrectedReadForConfirmation(t *testing.T) {
	// The QORB -> OORB example: registry has only OORB, one letter off.
	// This must NOT auto-apply — a single OCR read has no corroborating
	// second signal, so even a unique distance-1 match still needs a
	// human tap rather than silently merging into the existing tag.
	fake := &gemini.Fake{
		TagCaptureResult: gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "QORB"},
	}
	h, cookies, s := newTestServerWithGemini(t, fake)
	registerTestTag(t, s, "OORB")

	w := doMultipartUpload(t, h, "/api/capture/preview", cookies, []byte("photo"), nil)
	var preview capturePreviewResponse
	json.NewDecoder(w.Body).Decode(&preview)
	if !preview.NeedsResolution || !preview.Corrected {
		t.Fatalf("corrected read should still need resolution: %+v", preview)
	}
	if preview.AssetTag != "" {
		t.Fatalf("AssetTag = %q, want empty until operator confirms", preview.AssetTag)
	}
	if len(preview.Candidates) != 1 || preview.Candidates[0] != "OORB" {
		t.Fatalf("Candidates = %v, want [OORB]", preview.Candidates)
	}
	if preview.RawAssetTag != "QORB" {
		t.Fatalf("RawAssetTag = %q, want QORB", preview.RawAssetTag)
	}
}

func TestCapturePreviewFlagsAmbiguousRead(t *testing.T) {
	fake := &gemini.Fake{
		TagCaptureResult: gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "QORB"},
	}
	h, cookies, s := newTestServerWithGemini(t, fake)
	registerTestTag(t, s, "OORB")
	registerTestTag(t, s, "QIRB")

	w := doMultipartUpload(t, h, "/api/capture/preview", cookies, []byte("photo"), nil)
	var preview capturePreviewResponse
	json.NewDecoder(w.Body).Decode(&preview)
	if !preview.NeedsResolution || preview.Corrected {
		t.Fatalf("tied candidates should be ambiguous, not corrected: %+v", preview)
	}
	if len(preview.Candidates) != 2 {
		t.Fatalf("Candidates = %v, want 2 tied candidates", preview.Candidates)
	}
}

func TestCapturePreviewFlagsNoMatchRead(t *testing.T) {
	fake := &gemini.Fake{
		TagCaptureResult: gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "QORB"},
	}
	h, cookies, s := newTestServerWithGemini(t, fake)
	registerTestTag(t, s, "ZZZZ") // distance 4, no candidates

	w := doMultipartUpload(t, h, "/api/capture/preview", cookies, []byte("photo"), nil)
	var preview capturePreviewResponse
	json.NewDecoder(w.Body).Decode(&preview)
	if !preview.NeedsResolution || preview.Corrected || len(preview.Candidates) != 0 {
		t.Fatalf("unmatched read = %+v, want NeedsResolution with no candidates", preview)
	}
}

func TestCaptureApplySelfHealsRegistry(t *testing.T) {
	fake := &gemini.Fake{
		TagCaptureResult: gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "ZKEI"},
	}
	h, cookies, _ := newTestServerWithGemini(t, fake)

	doCaptureUpload(t, h, cookies, []byte("photo-1")) // ZKEI starts unregistered, gets accepted as-read

	w := doMultipartUpload(t, h, "/api/capture/preview", cookies, []byte("photo-2"), nil)
	var preview capturePreviewResponse
	json.NewDecoder(w.Body).Decode(&preview)
	if preview.NeedsResolution || preview.AssetTag != "ZKEI" {
		t.Fatalf("second preview of the same tag = %+v, want a silent exact match after self-healing", preview)
	}
}

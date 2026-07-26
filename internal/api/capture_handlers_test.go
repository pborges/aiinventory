package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pborges/aiinventory/internal/auth"
	"github.com/pborges/aiinventory/internal/gemini"
	"github.com/pborges/aiinventory/internal/store"
)

func newTestServerWithGemini(t *testing.T, g gemini.Client) (http.Handler, []*http.Cookie) {
	t.Helper()
	s := store.NewTestStore(t)
	codec := auth.NewCodec("test-secret")
	h := New(s, codec, g)
	w := doJSON(t, h, http.MethodPost, "/api/auth/bootstrap", credentials{Username: "alice", Password: "correcthorse"}, nil)
	return h, w.Result().Cookies()
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
	return doMultipartUpload(t, h, "/api/capture/apply", cookies, imageBytes, map[string]string{
		"asset_tag":   preview.AssetTag,
		"description": preview.ImageDescription,
	})
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
	h, cookies := newTestServerWithGemini(t, nil)
	w := doMultipartUpload(t, h, "/api/capture/preview", cookies, []byte("fake-jpeg"), nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestCapturePreviewDoesNotWriteToStore(t *testing.T) {
	fake := &gemini.Fake{
		TagCaptureResult: gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "ZKEI", ItemGuess: "cordless drill", Description: "S/N 12345"},
	}
	h, cookies := newTestServerWithGemini(t, fake)

	w := doMultipartUpload(t, h, "/api/capture/preview", cookies, []byte("fake-jpeg-bytes"), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var preview capturePreviewResponse
	if err := json.NewDecoder(w.Body).Decode(&preview); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !preview.HasAssetTag || preview.AssetTag != "ZKEI" || !preview.ItemWillBeNew {
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
	h, cookies := newTestServerWithGemini(t, fake)

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

func TestCaptureApplyRejectsInvalidAssetTag(t *testing.T) {
	h, cookies := newTestServerWithGemini(t, nil) // apply doesn't need gemini configured
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
	h, cookies := newTestServerWithGemini(t, fake)

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
	h, cookies := newTestServerWithGemini(t, fake)

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

func TestCapturePreviewExistingItemNotFlaggedAsNew(t *testing.T) {
	fake := &gemini.Fake{
		TagCaptureResult: gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "ZKEI"},
	}
	h, cookies := newTestServerWithGemini(t, fake)

	doCaptureUpload(t, h, cookies, []byte("photo-1")) // creates ZKEI

	w := doMultipartUpload(t, h, "/api/capture/preview", cookies, []byte("photo-2"), nil)
	var preview capturePreviewResponse
	json.NewDecoder(w.Body).Decode(&preview)
	if preview.ItemWillBeNew {
		t.Fatalf("preview for an already-existing tag reported ItemWillBeNew = true")
	}
}

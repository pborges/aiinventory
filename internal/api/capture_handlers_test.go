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

func doCaptureUpload(t *testing.T, h http.Handler, cookies []*http.Cookie, imageBytes []byte) *httptest.ResponseRecorder {
	t.Helper()
	return doMultipartUpload(t, h, "/api/capture", cookies, imageBytes)
}

func doMultipartUpload(t *testing.T, h http.Handler, path string, cookies []*http.Cookie, imageBytes []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("image", "capture.jpg")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	part.Write(imageBytes)
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

func TestCaptureWithoutGeminiConfigured(t *testing.T) {
	h, cookies := newTestServerWithGemini(t, nil)
	w := doCaptureUpload(t, h, cookies, []byte("fake-jpeg"))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestCaptureCreatesNewItem(t *testing.T) {
	fake := &gemini.Fake{
		TagCaptureResult: gemini.TagCaptureResult{
			HasAssetTag: true,
			AssetTag:    "ZKEI",
			ItemGuess:   "cordless drill",
			Description: "S/N 12345",
		},
	}
	h, cookies := newTestServerWithGemini(t, fake)

	w := doCaptureUpload(t, h, cookies, []byte("fake-jpeg-bytes"))
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
	if resp.ItemGuess != "cordless drill" {
		t.Errorf("ItemGuess = %q", resp.ItemGuess)
	}
}

func TestCaptureAppendsOnSecondPhotoOfSameTag(t *testing.T) {
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

func TestCaptureNoAssetTagFound(t *testing.T) {
	fake := &gemini.Fake{
		TagCaptureResult: gemini.TagCaptureResult{HasAssetTag: false},
	}
	h, cookies := newTestServerWithGemini(t, fake)

	w := doCaptureUpload(t, h, cookies, []byte("photo-with-no-tag"))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp captureResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.HasAssetTag {
		t.Fatalf("HasAssetTag = true, want false")
	}
}

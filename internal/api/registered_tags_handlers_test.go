package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/pborges/aiinventory/internal/gemini"
)

// doTextFileUpload posts a "file" multipart field — the registry bulk-
// upload's field name, distinct from doMultipartUpload's "image" field.
func doTextFileUpload(t *testing.T, h http.Handler, path string, cookies []*http.Cookie, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", "tags.txt")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	part.Write(content)
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

func TestRegisteredAssetTagCRUDFlow(t *testing.T) {
	fake := &gemini.Fake{}
	h, cookies, _ := newTestServerWithGemini(t, fake)

	// create
	w := doJSON(t, h, http.MethodPost, "/api/tags", registeredTagRequest{Tag: "zkei"}, cookies)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", w.Code, w.Body.String())
	}
	var created map[string]registeredTagResponse
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created["tag"].Tag != "ZKEI" {
		t.Fatalf("created tag = %q, want ZKEI (lowercase input should be normalized)", created["tag"].Tag)
	}
	id := created["tag"].ID

	// idempotent re-create
	w = doJSON(t, h, http.MethodPost, "/api/tags", registeredTagRequest{Tag: "ZKEI"}, cookies)
	if w.Code != http.StatusCreated {
		t.Fatalf("re-create status = %d, body = %s", w.Code, w.Body.String())
	}

	// list
	w = doJSON(t, h, http.MethodGet, "/api/tags", nil, cookies)
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", w.Code, w.Body.String())
	}
	var list map[string][]registeredTagResponse
	if err := json.NewDecoder(w.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list["tags"]) != 1 {
		t.Fatalf("list = %+v, want 1 entry (idempotent re-create shouldn't duplicate)", list["tags"])
	}

	// delete
	w = doJSON(t, h, http.MethodDelete, "/api/tags/"+strconv.FormatInt(id, 10), nil, cookies)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", w.Code, w.Body.String())
	}

	w = doJSON(t, h, http.MethodGet, "/api/tags", nil, cookies)
	json.NewDecoder(w.Body).Decode(&list)
	if len(list["tags"]) != 0 {
		t.Fatalf("list after delete = %+v, want empty", list["tags"])
	}

	// delete again -> 404
	w = doJSON(t, h, http.MethodDelete, "/api/tags/"+strconv.FormatInt(id, 10), nil, cookies)
	if w.Code != http.StatusNotFound {
		t.Fatalf("second delete status = %d, want 404", w.Code)
	}
}

func TestCreateRegisteredAssetTagRejectsMalformed(t *testing.T) {
	fake := &gemini.Fake{}
	h, cookies, _ := newTestServerWithGemini(t, fake)

	for _, tag := range []string{"ZK3I", "ZKEIX", "ZK", ""} {
		w := doJSON(t, h, http.MethodPost, "/api/tags", registeredTagRequest{Tag: tag}, cookies)
		if w.Code != http.StatusBadRequest {
			t.Errorf("tag %q: status = %d, want 400", tag, w.Code)
		}
	}
}

func TestUploadRegisteredAssetTags(t *testing.T) {
	fake := &gemini.Fake{}
	h, cookies, s := newTestServerWithGemini(t, fake)
	registerTestTag(t, s, "AAAA")

	w := doTextFileUpload(t, h, "/api/tags/upload", cookies, []byte("aaaa\nBBBB\n\ncccc\n"))
	if w.Code != http.StatusOK {
		t.Fatalf("upload status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp registeredTagUploadResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Added != 2 || resp.Skipped != 1 {
		t.Fatalf("resp = %+v, want added=2 skipped=1 (AAAA already registered)", resp)
	}
}

func TestUploadRegisteredAssetTagsRejectsMalformedLine(t *testing.T) {
	fake := &gemini.Fake{}
	h, cookies, _ := newTestServerWithGemini(t, fake)

	w := doTextFileUpload(t, h, "/api/tags/upload", cookies, []byte("AAAA\nZK3I\n"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", w.Code, w.Body.String())
	}

	// nothing from the rejected file should have been imported
	list := doJSON(t, h, http.MethodGet, "/api/tags", nil, cookies)
	var decoded map[string][]registeredTagResponse
	json.NewDecoder(list.Body).Decode(&decoded)
	if len(decoded["tags"]) != 0 {
		t.Fatalf("tags = %+v, want empty (whole file should be rejected)", decoded["tags"])
	}
}

func TestRegisteredLocationTagCRUDFlow(t *testing.T) {
	fake := &gemini.Fake{}
	h, cookies, _ := newTestServerWithGemini(t, fake)

	w := doJSON(t, h, http.MethodPost, "/api/location-tags", registeredTagRequest{Tag: "@xyz"}, cookies)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", w.Code, w.Body.String())
	}
	var created map[string]registeredTagResponse
	json.NewDecoder(w.Body).Decode(&created)
	if created["tag"].Tag != "@XYZ" {
		t.Fatalf("created tag = %q, want @XYZ (lowercase input should be normalized)", created["tag"].Tag)
	}
	id := created["tag"].ID

	w = doJSON(t, h, http.MethodGet, "/api/location-tags", nil, cookies)
	var list map[string][]registeredTagResponse
	json.NewDecoder(w.Body).Decode(&list)
	if len(list["tags"]) != 1 {
		t.Fatalf("list = %+v, want 1 entry", list["tags"])
	}

	w = doJSON(t, h, http.MethodDelete, "/api/location-tags/"+strconv.FormatInt(id, 10), nil, cookies)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestUploadRegisteredLocationTags(t *testing.T) {
	fake := &gemini.Fake{}
	h, cookies, _ := newTestServerWithGemini(t, fake)

	w := doTextFileUpload(t, h, "/api/location-tags/upload", cookies, []byte("@AAA\n@bbb\n"))
	if w.Code != http.StatusOK {
		t.Fatalf("upload status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp registeredTagUploadResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Added != 2 || resp.Skipped != 0 {
		t.Fatalf("resp = %+v, want added=2 skipped=0", resp)
	}
}

func TestRegisteredTagRoutesRequireAuth(t *testing.T) {
	fake := &gemini.Fake{}
	h, _, _ := newTestServerWithGemini(t, fake)

	for _, req := range []struct {
		method, path string
	}{
		{http.MethodGet, "/api/tags"},
		{http.MethodPost, "/api/tags"},
		{http.MethodDelete, "/api/tags/1"},
		{http.MethodGet, "/api/location-tags"},
		{http.MethodPost, "/api/location-tags"},
		{http.MethodDelete, "/api/location-tags/1"},
	} {
		w := doJSON(t, h, req.method, req.path, nil, nil)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: status = %d, want 401", req.method, req.path, w.Code)
		}
	}
}

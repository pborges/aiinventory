package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONRejectsUnknownAndTrailingValues(t *testing.T) {
	for _, body := range []string{
		`{"name":"ok","unexpected":true}`,
		`{"name":"ok"} {"name":"again"}`,
	} {
		t.Run(body, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			w := httptest.NewRecorder()
			var value struct {
				Name string `json:"name"`
			}
			if decodeJSON(w, req, &value) {
				t.Fatal("decodeJSON accepted invalid request")
			}
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", w.Code)
			}
		})
	}
}

func TestDecodeJSONRejectsOversizedBody(t *testing.T) {
	body := `{"name":"` + strings.Repeat("x", maxJSONBodyBytes) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	w := httptest.NewRecorder()
	var value struct {
		Name string `json:"name"`
	}
	if decodeJSON(w, req, &value) {
		t.Fatal("decodeJSON accepted oversized request")
	}
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", w.Code)
	}
}

func TestDecodeOptionalJSONAcceptsEmptyChunkedBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
	req.ContentLength = -1
	w := httptest.NewRecorder()
	var value struct {
		Hint string `json:"hint"`
	}
	if !decodeOptionalJSON(w, req, &value) {
		t.Fatalf("empty optional body rejected with status %d", w.Code)
	}
}

package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/pborges/aiinventory/internal/version"
)

func TestVersionIsPublic(t *testing.T) {
	h, _ := newTestServer(t)

	w := doJSON(t, h, http.MethodGet, "/api/version", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (should not require auth)", w.Code)
	}
	var resp versionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Version != version.Version {
		t.Errorf("Version = %q, want %q", resp.Version, version.Version)
	}
}

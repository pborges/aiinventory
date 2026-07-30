package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/pborges/aiinventory/internal/gemini"
)

func loggedInServer(t *testing.T) (http.Handler, []*http.Cookie) {
	t.Helper()
	h, _ := newTestServer(t)
	w := doJSON(t, h, http.MethodPost, "/api/auth/bootstrap", credentials{Username: "alice", Password: "correcthorse"}, nil)
	return h, w.Result().Cookies()
}

func TestSettingsRequiresAuth(t *testing.T) {
	h, _ := newTestServer(t)
	w := doJSON(t, h, http.MethodGet, "/api/settings", nil, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestSettingsDefaultsBeforeAnyOverride(t *testing.T) {
	h, cookies := loggedInServer(t)

	w := doJSON(t, h, http.MethodGet, "/api/settings", nil, cookies)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp settingsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.GeminiAPIKeySet {
		t.Error("GeminiAPIKeySet = true, want false (no key configured yet)")
	}
	if resp.GeminiModel != "" {
		t.Errorf("GeminiModel = %q, want empty (no override set yet)", resp.GeminiModel)
	}
	if resp.GeminiModelDefault != gemini.DefaultModel {
		t.Errorf("GeminiModelDefault = %q, want %q", resp.GeminiModelDefault, gemini.DefaultModel)
	}
	if len(resp.Prompts) != 4 {
		t.Fatalf("got %d prompt entries, want 4", len(resp.Prompts))
	}
	tc, ok := resp.Prompts[string(gemini.TagCapture)]
	if !ok {
		t.Fatal("missing tag_capture prompt entry")
	}
	if tc.Override != "" {
		t.Errorf("tag_capture override = %q, want empty", tc.Override)
	}
	if tc.Default != gemini.DefaultPrompt(gemini.TagCapture) {
		t.Errorf("tag_capture default mismatch")
	}
	if !resp.LocationDualReadEnabled {
		t.Error("LocationDualReadEnabled = false, want true (enabled by default, before any override)")
	}
}

func TestSettingsLocationDualReadToggle(t *testing.T) {
	h, cookies := loggedInServer(t)

	w := doJSON(t, h, http.MethodPut, "/api/settings", updateSettingsRequest{LocationDualReadEnabled: boolPtr(false)}, cookies)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp settingsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.LocationDualReadEnabled {
		t.Error("LocationDualReadEnabled = true after disabling, want false")
	}

	// persists across a fresh request, not just echoed
	w = doJSON(t, h, http.MethodGet, "/api/settings", nil, cookies)
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.LocationDualReadEnabled {
		t.Error("after reload, LocationDualReadEnabled = true, want false")
	}

	// re-enabling flips it back
	w = doJSON(t, h, http.MethodPut, "/api/settings", updateSettingsRequest{LocationDualReadEnabled: boolPtr(true)}, cookies)
	json.NewDecoder(w.Body).Decode(&resp)
	if !resp.LocationDualReadEnabled {
		t.Error("LocationDualReadEnabled = false after re-enabling, want true")
	}
}

func TestSettingsUpdateRoundTrips(t *testing.T) {
	h, cookies := loggedInServer(t)

	update := updateSettingsRequest{
		GeminiModel: strPtr("gemini-2.5-pro"),
		Prompts: map[string]string{
			string(gemini.TagCapture): "custom tag capture prompt",
		},
	}
	w := doJSON(t, h, http.MethodPut, "/api/settings", update, cookies)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp settingsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.GeminiModel != "gemini-2.5-pro" {
		t.Errorf("GeminiModel = %q, want gemini-2.5-pro", resp.GeminiModel)
	}
	if resp.Prompts[string(gemini.TagCapture)].Override != "custom tag capture prompt" {
		t.Errorf("tag_capture override = %q", resp.Prompts[string(gemini.TagCapture)].Override)
	}
	// untouched prompt types keep an empty override
	if resp.Prompts[string(gemini.DuplicateDetection)].Override != "" {
		t.Errorf("duplicate_detection override should be untouched, got %q", resp.Prompts[string(gemini.DuplicateDetection)].Override)
	}

	// re-fetch on a fresh request to confirm persistence, not just an echoed response
	w = doJSON(t, h, http.MethodGet, "/api/settings", nil, cookies)
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.GeminiModel != "gemini-2.5-pro" {
		t.Errorf("after reload, GeminiModel = %q, want gemini-2.5-pro", resp.GeminiModel)
	}
}

func TestSettingsGeminiAPIKeySetAndClear(t *testing.T) {
	h, cookies := loggedInServer(t)

	// setting a key persists it and flips gemini_api_key_set, but never
	// echoes the raw value back
	w := doJSON(t, h, http.MethodPut, "/api/settings", updateSettingsRequest{GeminiAPIKey: strPtr("test-key-123")}, cookies)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp settingsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.GeminiAPIKeySet {
		t.Error("GeminiAPIKeySet = false after setting a key, want true")
	}
	if raw := w.Body.String(); strings.Contains(raw, "test-key-123") {
		t.Error("response body echoes the raw api key back")
	}

	// a fresh GET still reports it as set (persisted, not just echoed)
	w = doJSON(t, h, http.MethodGet, "/api/settings", nil, cookies)
	json.NewDecoder(w.Body).Decode(&resp)
	if !resp.GeminiAPIKeySet {
		t.Error("after reload, GeminiAPIKeySet = false, want true")
	}

	// clearing it (empty string, not omitted) unsets it again
	w = doJSON(t, h, http.MethodPut, "/api/settings", updateSettingsRequest{GeminiAPIKey: strPtr("")}, cookies)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", w.Code, w.Body.String())
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.GeminiAPIKeySet {
		t.Error("GeminiAPIKeySet = true after clearing, want false")
	}
}

func strPtr(s string) *string { return &s }

func boolPtr(b bool) *bool { return &b }

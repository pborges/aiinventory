package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestTagSheetSettingsRequiresAuth(t *testing.T) {
	h, _ := newTestServer(t)
	for _, path := range []string{"/api/tags/sheet/settings", "/api/location-tags/sheet/settings"} {
		w := doJSON(t, h, http.MethodGet, path, nil, nil)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("GET %s status = %d, want 401", path, w.Code)
		}
	}
}

func TestTagSheetSettingsDefaultsBeforeAnyOverride(t *testing.T) {
	h, cookies := loggedInServer(t)

	w := doJSON(t, h, http.MethodGet, "/api/tags/sheet/settings", nil, cookies)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp tagSheetSettingsRequest
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp != defaultTagSheetSettings {
		t.Errorf("settings before any override = %+v, want defaults %+v", resp, defaultTagSheetSettings)
	}
}

func TestTagSheetSettingsSaveRoundTrips(t *testing.T) {
	h, cookies := loggedInServer(t)

	custom := tagSheetSettingsRequest{
		Rows: 8, Cols: 5, PaddingMm: 3,
		CutSettings: tagSheetCutSettingsRequest{
			RasterSpeedMmMin: 4000, RasterPowerPct: 20, RasterAirAssist: true,
			OutlineSpeedMmMin: 2000, OutlinePowerPct: 10, OutlineAirAssist: true,
			CutSpeedMmMin: 500, CutPowerPct: 90, CutAirAssist: false,
		},
	}
	w := doJSON(t, h, http.MethodPut, "/api/tags/sheet/settings", custom, cookies)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp tagSheetSettingsRequest
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp != custom {
		t.Errorf("PUT response = %+v, want %+v", resp, custom)
	}

	// persists across a fresh request, not just echoed
	w = doJSON(t, h, http.MethodGet, "/api/tags/sheet/settings", nil, cookies)
	json.NewDecoder(w.Body).Decode(&resp)
	if resp != custom {
		t.Errorf("after reload, settings = %+v, want %+v", resp, custom)
	}

	// location-tag settings are independently scoped — still defaults
	w = doJSON(t, h, http.MethodGet, "/api/location-tags/sheet/settings", nil, cookies)
	var locResp tagSheetSettingsRequest
	json.NewDecoder(w.Body).Decode(&locResp)
	if locResp != defaultTagSheetSettings {
		t.Errorf("location-tags settings after saving only asset-tags = %+v, want untouched defaults %+v", locResp, defaultTagSheetSettings)
	}
}

func TestTagSheetSettingsRestoreDefaults(t *testing.T) {
	h, cookies := loggedInServer(t)

	custom := defaultTagSheetSettings
	custom.Rows = 12
	if w := doJSON(t, h, http.MethodPut, "/api/tags/sheet/settings", custom, cookies); w.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", w.Code, w.Body.String())
	}

	w := doJSON(t, h, http.MethodDelete, "/api/tags/sheet/settings", nil, cookies)
	if w.Code != http.StatusOK {
		t.Fatalf("DELETE status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp tagSheetSettingsRequest
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp != defaultTagSheetSettings {
		t.Errorf("DELETE response = %+v, want defaults %+v", resp, defaultTagSheetSettings)
	}

	// a subsequent GET reflects the restored default too, not just the echo
	w = doJSON(t, h, http.MethodGet, "/api/tags/sheet/settings", nil, cookies)
	json.NewDecoder(w.Body).Decode(&resp)
	if resp != defaultTagSheetSettings {
		t.Errorf("after restore, GET settings = %+v, want defaults %+v", resp, defaultTagSheetSettings)
	}
}

func TestTagSheetSettingsRejectsInvalid(t *testing.T) {
	h, cookies := loggedInServer(t)

	invalid := defaultTagSheetSettings
	invalid.Rows = 0
	if w := doJSON(t, h, http.MethodPut, "/api/tags/sheet/settings", invalid, cookies); w.Code != http.StatusBadRequest {
		t.Errorf("rows=0 status = %d, want 400", w.Code)
	}

	invalid = defaultTagSheetSettings
	invalid.CutSettings.CutPowerPct = 150
	if w := doJSON(t, h, http.MethodPut, "/api/tags/sheet/settings", invalid, cookies); w.Code != http.StatusBadRequest {
		t.Errorf("cut_power_pct=150 status = %d, want 400", w.Code)
	}
}

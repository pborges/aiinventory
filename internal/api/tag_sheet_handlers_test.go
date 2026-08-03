package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/pborges/aiinventory/internal/gemini"
)

// validCutSettingsReq is a plausible 20W-diode-on-basswood cut settings
// payload — the concrete values don't matter to most tests, they just need
// to pass tagSheetCutSettingsRequest.valid() so rows/cols/padding/codes
// validation is what's actually under test.
var validCutSettingsReq = tagSheetCutSettingsRequest{
	RasterSpeedMmMin: 3000, RasterPowerPct: 40,
	OutlineSpeedMmMin: 1500, OutlinePowerPct: 25,
	CutSpeedMmMin: 200, CutPowerPct: 100, CutAirAssist: true,
}

func TestGenerateAssetTagSheet(t *testing.T) {
	fake := &gemini.Fake{}
	h, cookies, _ := newTestServerWithGemini(t, fake)

	w := doJSON(t, h, http.MethodPost, "/api/tags/sheet", tagSheetRequest{Rows: 2, Cols: 2, PaddingMm: 4, CutSettings: validCutSettingsReq}, cookies)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp tagSheetResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Codes) != 4 {
		t.Fatalf("got %d codes, want 4", len(resp.Codes))
	}
	seen := make(map[string]bool)
	for _, c := range resp.Codes {
		if !assetTagPattern.MatchString(c) {
			t.Errorf("code %q does not match asset tag pattern", c)
		}
		if seen[c] {
			t.Errorf("duplicate code %q", c)
		}
		seen[c] = true
	}
	if !strings.Contains(resp.SVG, "<svg") {
		t.Error("SVG response does not look like an SVG document")
	}
	if !strings.Contains(resp.LBRN2, "<?xml") {
		t.Error("LBRN2 response does not look like an XML document")
	}
	if !strings.Contains(resp.LBRN2, `<speed Value="50"/>`) {
		t.Errorf("LBRN2 should reflect the requested raster speed (3000mm/min = 50mm/sec), body = %s", resp.LBRN2)
	}
	if !strings.Contains(resp.LBRN2, `<runBlower Value="1"/>`) {
		t.Error("LBRN2 should reflect the requested air assist (on for the cut layer)")
	}

	// nothing should be persisted by a preview alone
	list := doJSON(t, h, http.MethodGet, "/api/tags", nil, cookies)
	var decoded map[string][]registeredTagResponse
	json.NewDecoder(list.Body).Decode(&decoded)
	if len(decoded["tags"]) != 0 {
		t.Fatalf("tags after preview = %+v, want empty (preview must not register anything)", decoded["tags"])
	}
}

func TestGenerateAssetTagSheetExcludesKnownTags(t *testing.T) {
	fake := &gemini.Fake{}
	h, cookies, s := newTestServerWithGemini(t, fake)
	registerTestTag(t, s, "AAAA")

	// A 1x1 sheet has only one code, so if the generator can still exclude
	// a specific known tag, we can at least confirm it never reproduces it
	// even after many draws.
	for range 20 {
		w := doJSON(t, h, http.MethodPost, "/api/tags/sheet", tagSheetRequest{Rows: 1, Cols: 1, PaddingMm: 4, CutSettings: validCutSettingsReq}, cookies)
		var resp tagSheetResponse
		json.NewDecoder(w.Body).Decode(&resp)
		if len(resp.Codes) == 1 && resp.Codes[0] == "AAAA" {
			t.Fatalf("generated already-registered tag AAAA")
		}
	}
}

func TestGenerateTagSheetRejectsBadParams(t *testing.T) {
	fake := &gemini.Fake{}
	h, cookies, _ := newTestServerWithGemini(t, fake)

	for _, req := range []tagSheetRequest{
		{Rows: 0, Cols: 4, PaddingMm: 4, CutSettings: validCutSettingsReq},
		{Rows: 31, Cols: 4, PaddingMm: 4, CutSettings: validCutSettingsReq},
		{Rows: 4, Cols: 0, PaddingMm: 4, CutSettings: validCutSettingsReq},
		{Rows: 4, Cols: 21, PaddingMm: 4, CutSettings: validCutSettingsReq},
		{Rows: 4, Cols: 4, PaddingMm: -1, CutSettings: validCutSettingsReq},
		{Rows: 4, Cols: 4, PaddingMm: 51, CutSettings: validCutSettingsReq},
	} {
		w := doJSON(t, h, http.MethodPost, "/api/tags/sheet", req, cookies)
		if w.Code != http.StatusBadRequest {
			t.Errorf("req %+v: status = %d, want 400", req, w.Code)
		}
	}
}

func TestGenerateTagSheetRejectsBadCutSettings(t *testing.T) {
	fake := &gemini.Fake{}
	h, cookies, _ := newTestServerWithGemini(t, fake)

	base := validCutSettingsReq
	for _, cs := range []tagSheetCutSettingsRequest{
		func() tagSheetCutSettingsRequest { c := base; c.RasterSpeedMmMin = 0; return c }(),
		func() tagSheetCutSettingsRequest { c := base; c.RasterSpeedMmMin = -5; return c }(),
		func() tagSheetCutSettingsRequest { c := base; c.CutPowerPct = 0; return c }(),
		func() tagSheetCutSettingsRequest { c := base; c.CutPowerPct = 101; return c }(),
	} {
		req := tagSheetRequest{Rows: 2, Cols: 2, PaddingMm: 4, CutSettings: cs}
		w := doJSON(t, h, http.MethodPost, "/api/tags/sheet", req, cookies)
		if w.Code != http.StatusBadRequest {
			t.Errorf("cut settings %+v: status = %d, want 400", cs, w.Code)
		}
	}
}

func TestGenerateTagSheetWithCodesReRendersWithoutRerolling(t *testing.T) {
	fake := &gemini.Fake{}
	h, cookies, _ := newTestServerWithGemini(t, fake)

	first := doJSON(t, h, http.MethodPost, "/api/tags/sheet", tagSheetRequest{Rows: 1, Cols: 2, PaddingMm: 4, CutSettings: validCutSettingsReq}, cookies)
	var firstResp tagSheetResponse
	if err := json.NewDecoder(first.Body).Decode(&firstResp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	tweaked := validCutSettingsReq
	tweaked.CutPowerPct = 55
	second := doJSON(t, h, http.MethodPost, "/api/tags/sheet",
		tagSheetRequest{Rows: 1, Cols: 2, PaddingMm: 4, Codes: firstResp.Codes, CutSettings: tweaked}, cookies)
	if second.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", second.Code, second.Body.String())
	}
	var secondResp tagSheetResponse
	json.NewDecoder(second.Body).Decode(&secondResp)

	if len(secondResp.Codes) != len(firstResp.Codes) {
		t.Fatalf("codes = %v, want same codes as first response %v", secondResp.Codes, firstResp.Codes)
	}
	for i, c := range firstResp.Codes {
		if secondResp.Codes[i] != c {
			t.Fatalf("codes = %v, want unchanged codes %v (cut-settings-only tweak shouldn't reroll)", secondResp.Codes, firstResp.Codes)
		}
	}
	if !strings.Contains(secondResp.LBRN2, `<maxPower Value="55"/>`) {
		t.Errorf("re-rendered LBRN2 should reflect the tweaked cut power, body = %s", secondResp.LBRN2)
	}
}

func TestGenerateTagSheetRejectsMismatchedCodeCount(t *testing.T) {
	fake := &gemini.Fake{}
	h, cookies, _ := newTestServerWithGemini(t, fake)

	req := tagSheetRequest{Rows: 1, Cols: 2, PaddingMm: 4, Codes: []string{"AAAA"}, CutSettings: validCutSettingsReq}
	w := doJSON(t, h, http.MethodPost, "/api/tags/sheet", req, cookies)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestRegisterAssetTagSheet(t *testing.T) {
	fake := &gemini.Fake{}
	h, cookies, _ := newTestServerWithGemini(t, fake)

	w := doJSON(t, h, http.MethodPost, "/api/tags/sheet/register", tagSheetRegisterRequest{Codes: []string{"aaaa", "BBBB"}}, cookies)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp registeredTagUploadResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Added != 2 || resp.Skipped != 0 {
		t.Fatalf("resp = %+v, want added=2 skipped=0", resp)
	}

	list := doJSON(t, h, http.MethodGet, "/api/tags", nil, cookies)
	var decoded map[string][]registeredTagResponse
	json.NewDecoder(list.Body).Decode(&decoded)
	if len(decoded["tags"]) != 2 {
		t.Fatalf("tags after register = %+v, want 2 entries", decoded["tags"])
	}

	// re-registering the same codes should be a no-op, all skipped
	w = doJSON(t, h, http.MethodPost, "/api/tags/sheet/register", tagSheetRegisterRequest{Codes: []string{"AAAA", "BBBB"}}, cookies)
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Added != 0 || resp.Skipped != 2 {
		t.Fatalf("re-register resp = %+v, want added=0 skipped=2", resp)
	}
}

func TestRegisterAssetTagSheetRejectsMalformedCode(t *testing.T) {
	fake := &gemini.Fake{}
	h, cookies, _ := newTestServerWithGemini(t, fake)

	w := doJSON(t, h, http.MethodPost, "/api/tags/sheet/register", tagSheetRegisterRequest{Codes: []string{"AAAA", "ZK3I"}}, cookies)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", w.Code, w.Body.String())
	}

	list := doJSON(t, h, http.MethodGet, "/api/tags", nil, cookies)
	var decoded map[string][]registeredTagResponse
	json.NewDecoder(list.Body).Decode(&decoded)
	if len(decoded["tags"]) != 0 {
		t.Fatalf("tags = %+v, want empty (whole batch should be rejected)", decoded["tags"])
	}
}

func TestRegisterAssetTagSheetRejectsEmptyCodes(t *testing.T) {
	fake := &gemini.Fake{}
	h, cookies, _ := newTestServerWithGemini(t, fake)

	w := doJSON(t, h, http.MethodPost, "/api/tags/sheet/register", tagSheetRegisterRequest{Codes: nil}, cookies)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestGenerateLocationTagSheet(t *testing.T) {
	fake := &gemini.Fake{}
	h, cookies, _ := newTestServerWithGemini(t, fake)

	w := doJSON(t, h, http.MethodPost, "/api/location-tags/sheet", tagSheetRequest{Rows: 2, Cols: 2, PaddingMm: 4, CutSettings: validCutSettingsReq}, cookies)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp tagSheetResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Codes) != 4 {
		t.Fatalf("got %d codes, want 4", len(resp.Codes))
	}
	for _, c := range resp.Codes {
		if !locationTagPattern.MatchString(c) {
			t.Errorf("code %q does not match location tag pattern", c)
		}
	}
}

func TestRegisterLocationTagSheet(t *testing.T) {
	fake := &gemini.Fake{}
	h, cookies, _ := newTestServerWithGemini(t, fake)

	w := doJSON(t, h, http.MethodPost, "/api/location-tags/sheet/register", tagSheetRegisterRequest{Codes: []string{"@abc"}}, cookies)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp registeredTagUploadResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Added != 1 {
		t.Fatalf("resp = %+v, want added=1", resp)
	}

	list := doJSON(t, h, http.MethodGet, "/api/location-tags", nil, cookies)
	var decoded map[string][]registeredTagResponse
	json.NewDecoder(list.Body).Decode(&decoded)
	if len(decoded["tags"]) != 1 || decoded["tags"][0].Tag != "@ABC" {
		t.Fatalf("tags = %+v, want [@ABC]", decoded["tags"])
	}
}

func TestTagSheetRoutesRequireAuth(t *testing.T) {
	fake := &gemini.Fake{}
	h, _, _ := newTestServerWithGemini(t, fake)

	for _, req := range []struct{ method, path string }{
		{http.MethodPost, "/api/tags/sheet"},
		{http.MethodPost, "/api/tags/sheet/register"},
		{http.MethodPost, "/api/location-tags/sheet"},
		{http.MethodPost, "/api/location-tags/sheet/register"},
	} {
		w := doJSON(t, h, req.method, req.path, nil, nil)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: status = %d, want 401", req.method, req.path, w.Code)
		}
	}
}

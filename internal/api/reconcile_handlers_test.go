package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/pborges/aiinventory/internal/gemini"
)

func TestReconcilePreviewNoLocationTag(t *testing.T) {
	fake := &gemini.Fake{ReconciliationResult: gemini.ReconciliationResult{HasLocationTag: false}}
	h, cookies, _ := newTestServerWithGemini(t, fake)

	req := doMultipartUpload(t, h, "/api/reconcile/preview", cookies, []byte("photo"), nil)
	if req.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", req.Code, req.Body.String())
	}
	var resp reconcileDiffResponse
	json.NewDecoder(req.Body).Decode(&resp)
	if resp.HasLocationTag {
		t.Fatalf("HasLocationTag = true, want false")
	}
}

func TestReconcilePreviewAndApplyFullFlow(t *testing.T) {
	fake := &gemini.Fake{}
	h, cookies, _ := newTestServerWithGemini(t, fake)

	// seed: ZKEI unlinked, GKEI at @QRS, XDKW currently at @XYZ (about to be removed)
	fake.TagCaptureResult = gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "ZKEI"}
	doCaptureUpload(t, h, cookies, []byte("zkei-photo"))
	fake.TagCaptureResult = gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "GKEI"}
	doCaptureUpload(t, h, cookies, []byte("gkei-photo"))
	fake.TagCaptureResult = gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "XDKW"}
	doCaptureUpload(t, h, cookies, []byte("xdkw-photo"))

	// put GKEI at @QRS and XDKW at @XYZ via a first reconciliation each
	fake.ReconciliationResult = gemini.ReconciliationResult{HasLocationTag: true, LocationTag: "@QRS", AssetTags: []string{"GKEI"}}
	applyReconcile(t, h, cookies, "@QRS", []string{"GKEI"})
	fake.ReconciliationResult = gemini.ReconciliationResult{HasLocationTag: true, LocationTag: "@XYZ", AssetTags: []string{"XDKW"}}
	applyReconcile(t, h, cookies, "@XYZ", []string{"XDKW"})

	// now reconcile @XYZ against a frame containing ZKEI + GKEI (not XDKW):
	// ZKEI -> added, GKEI -> moved from @QRS, XDKW -> removed
	fake.ReconciliationResult = gemini.ReconciliationResult{HasLocationTag: true, LocationTag: "@XYZ", AssetTags: []string{"ZKEI", "GKEI"}}
	previewResp := doMultipartUpload(t, h, "/api/reconcile/preview", cookies, []byte("xyz-frame"), nil)
	if previewResp.Code != http.StatusOK {
		t.Fatalf("preview status = %d, body = %s", previewResp.Code, previewResp.Body.String())
	}
	var diff reconcileDiffResponse
	json.NewDecoder(previewResp.Body).Decode(&diff)

	if len(diff.Added) != 1 || diff.Added[0] != "ZKEI" {
		t.Errorf("Added = %v, want [ZKEI]", diff.Added)
	}
	if len(diff.Moved) != 1 || diff.Moved[0].AssetTag != "GKEI" || diff.Moved[0].FromLocation != "@QRS" {
		t.Errorf("Moved = %+v, want [{GKEI @QRS}]", diff.Moved)
	}
	if len(diff.Removed) != 1 || diff.Removed[0] != "XDKW" {
		t.Errorf("Removed = %v, want [XDKW]", diff.Removed)
	}

	// approve: apply the same location_tag + asset_tags
	applied := applyReconcile(t, h, cookies, "@XYZ", []string{"ZKEI", "GKEI"})
	if len(applied.Added) != 1 || len(applied.Moved) != 1 || len(applied.Removed) != 1 {
		t.Fatalf("applied diff mismatch: %+v", applied)
	}

	// re-preview against the same frame should now show no changes (already applied)
	noopResp := doMultipartUpload(t, h, "/api/reconcile/preview", cookies, []byte("xyz-frame-again"), nil)
	var noop reconcileDiffResponse
	json.NewDecoder(noopResp.Body).Decode(&noop)
	if len(noop.Added) != 0 || len(noop.Moved) != 0 || len(noop.Removed) != 0 {
		t.Errorf("expected no-op diff after apply, got %+v", noop)
	}
}

func TestReconcilePreviewResolvesTagsBeforeDiffing(t *testing.T) {
	fake := &gemini.Fake{}
	h, cookies, s := newTestServerWithGemini(t, fake)
	registerTestTag(t, s, "OORB")

	// Gemini reads a garbled "QORB" for a tag that's actually the
	// registered "OORB" — the raw read must not leak into the diff
	// (New/Added/etc.) at all, since it hasn't been confirmed yet.
	fake.ReconciliationResult = gemini.ReconciliationResult{HasLocationTag: true, LocationTag: "@XYZ", AssetTags: []string{"QORB"}}
	req := doMultipartUpload(t, h, "/api/reconcile/preview", cookies, []byte("photo"), nil)
	if req.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", req.Code, req.Body.String())
	}
	var resp reconcileDiffResponse
	json.NewDecoder(req.Body).Decode(&resp)

	if len(resp.AssetTags) != 0 || len(resp.New) != 0 {
		t.Fatalf("uncorrected read leaked into diff: AssetTags=%v New=%v", resp.AssetTags, resp.New)
	}
	if len(resp.TagResolutions) != 1 {
		t.Fatalf("TagResolutions = %+v, want 1 entry", resp.TagResolutions)
	}
	res := resp.TagResolutions[0]
	if res.Raw != "QORB" || res.Status != "corrected" || len(res.Candidates) != 1 || res.Candidates[0] != "OORB" {
		t.Fatalf("resolution = %+v, want {Raw:QORB Status:corrected Candidates:[OORB]}", res)
	}
}

func TestReconcilePreviewSurfacesAmbiguousTagResolution(t *testing.T) {
	fake := &gemini.Fake{}
	h, cookies, s := newTestServerWithGemini(t, fake)
	registerTestTag(t, s, "OORB")
	registerTestTag(t, s, "QIRB")

	fake.ReconciliationResult = gemini.ReconciliationResult{HasLocationTag: true, LocationTag: "@XYZ", AssetTags: []string{"QORB"}}
	req := doMultipartUpload(t, h, "/api/reconcile/preview", cookies, []byte("photo"), nil)
	var resp reconcileDiffResponse
	json.NewDecoder(req.Body).Decode(&resp)

	if len(resp.AssetTags) != 0 {
		t.Fatalf("ambiguous read leaked into diff: AssetTags=%v", resp.AssetTags)
	}
	if len(resp.TagResolutions) != 1 || resp.TagResolutions[0].Status != "ambiguous" || len(resp.TagResolutions[0].Candidates) != 2 {
		t.Fatalf("resolution = %+v, want ambiguous with 2 candidates", resp.TagResolutions)
	}
}

func TestReconcilePreviewExactMatchFeedsTheDiff(t *testing.T) {
	fake := &gemini.Fake{}
	h, cookies, s := newTestServerWithGemini(t, fake)
	registerTestTag(t, s, "ZKEI")

	fake.ReconciliationResult = gemini.ReconciliationResult{HasLocationTag: true, LocationTag: "@XYZ", AssetTags: []string{"ZKEI"}}
	req := doMultipartUpload(t, h, "/api/reconcile/preview", cookies, []byte("photo"), nil)
	var resp reconcileDiffResponse
	json.NewDecoder(req.Body).Decode(&resp)

	if len(resp.AssetTags) != 1 || resp.AssetTags[0] != "ZKEI" {
		t.Fatalf("AssetTags = %v, want [ZKEI]", resp.AssetTags)
	}
	if len(resp.New) != 1 || resp.New[0] != "ZKEI" {
		t.Fatalf("New = %v, want [ZKEI]", resp.New)
	}
	if len(resp.TagResolutions) != 1 || resp.TagResolutions[0].Status != "exact" {
		t.Fatalf("resolution = %+v, want exact", resp.TagResolutions)
	}
}

func TestReconcilePreviewResolvesLocationTagBeforeDiffing(t *testing.T) {
	fake := &gemini.Fake{}
	h, cookies, s := newTestServerWithGemini(t, fake)
	registerTestLocationTag(t, s, "@XYZ")

	// Gemini reads a garbled "@XY2"-shaped-but-wrong "@XYQ" for a location
	// that's actually the registered "@XYZ" — the diff should still be
	// computed against the raw read (per design), but the response must
	// flag it as needing resolution rather than silently trusting it.
	fake.ReconciliationResult = gemini.ReconciliationResult{HasLocationTag: true, LocationTag: "@XYQ", AssetTags: []string{"ZKEI"}}
	req := doMultipartUpload(t, h, "/api/reconcile/preview", cookies, []byte("photo"), nil)
	if req.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", req.Code, req.Body.String())
	}
	var resp reconcileDiffResponse
	json.NewDecoder(req.Body).Decode(&resp)

	if resp.RawLocationTag != "@XYQ" {
		t.Errorf("RawLocationTag = %q, want @XYQ", resp.RawLocationTag)
	}
	if !resp.LocationTagCorrected || !resp.LocationTagNeedsResolution {
		t.Fatalf("resp = %+v, want LocationTagCorrected and LocationTagNeedsResolution both true", resp)
	}
	if len(resp.LocationTagCandidates) != 1 || resp.LocationTagCandidates[0] != "@XYZ" {
		t.Fatalf("LocationTagCandidates = %v, want [@XYZ]", resp.LocationTagCandidates)
	}
}

func TestReconcilePreviewExactLocationTagNeedsNoResolution(t *testing.T) {
	fake := &gemini.Fake{}
	h, cookies, s := newTestServerWithGemini(t, fake)
	registerTestLocationTag(t, s, "@XYZ")

	fake.ReconciliationResult = gemini.ReconciliationResult{HasLocationTag: true, LocationTag: "@XYZ", AssetTags: []string{"ZKEI"}}
	req := doMultipartUpload(t, h, "/api/reconcile/preview", cookies, []byte("photo"), nil)
	var resp reconcileDiffResponse
	json.NewDecoder(req.Body).Decode(&resp)

	if resp.LocationTagNeedsResolution || resp.LocationTagCorrected {
		t.Fatalf("resp = %+v, want no resolution needed for an exact registry match", resp)
	}
	if resp.LocationTag != "@XYZ" {
		t.Errorf("LocationTag = %q, want @XYZ", resp.LocationTag)
	}
}

func TestReconcileApplySelfHealsLocationTagRegistry(t *testing.T) {
	fake := &gemini.Fake{}
	h, cookies, s := newTestServerWithGemini(t, fake)

	// @XYZ is never registered beforehand — apply it directly, as if the
	// operator resolved a brand-new location tag by hand.
	applyReconcile(t, h, cookies, "@XYZ", []string{"ZKEI"})

	tags, err := s.ListRegisteredLocationTags(t.Context())
	if err != nil {
		t.Fatalf("ListRegisteredLocationTags: %v", err)
	}
	if len(tags) != 1 || tags[0] != "@XYZ" {
		t.Fatalf("ListRegisteredLocationTags = %v, want [@XYZ]", tags)
	}
}

func TestReconcileApplySelfHealsRegistry(t *testing.T) {
	fake := &gemini.Fake{}
	h, cookies, _ := newTestServerWithGemini(t, fake)

	// ZKEI is never registered or captured beforehand — apply it directly
	// via reconcile, as if the operator resolved a tag-review row by hand.
	applyReconcile(t, h, cookies, "@XYZ", []string{"ZKEI"})

	fake.ReconciliationResult = gemini.ReconciliationResult{HasLocationTag: true, LocationTag: "@XYZ", AssetTags: []string{"ZKEI"}}
	req := doMultipartUpload(t, h, "/api/reconcile/preview", cookies, []byte("photo"), nil)
	var resp reconcileDiffResponse
	json.NewDecoder(req.Body).Decode(&resp)
	if len(resp.TagResolutions) != 1 || resp.TagResolutions[0].Status != "exact" {
		t.Fatalf("second preview after apply = %+v, want a silent exact match after self-healing", resp.TagResolutions)
	}
}

func TestReconcilePreviewRejectsMalformedLocationTag(t *testing.T) {
	// Gemini's JSON schema only constrains this to a string — a misread
	// (wrong letter count, stray digit, lowercase) can still come back as
	// "valid" JSON. The deterministic shape check must catch it before a
	// diff is ever computed/shown.
	for _, tag := range []string{"@XY", "@WXYZ", "@xyz", "@X1Z", "XYZ", "@"} {
		fake := &gemini.Fake{ReconciliationResult: gemini.ReconciliationResult{HasLocationTag: true, LocationTag: tag, AssetTags: []string{"ZKEI"}}}
		h, cookies, _ := newTestServerWithGemini(t, fake)

		req := doMultipartUpload(t, h, "/api/reconcile/preview", cookies, []byte("photo"), nil)
		if req.Code != http.StatusOK {
			t.Fatalf("tag %q: status = %d, body = %s", tag, req.Code, req.Body.String())
		}
		var resp reconcileDiffResponse
		json.NewDecoder(req.Body).Decode(&resp)
		if resp.HasLocationTag {
			t.Errorf("tag %q: HasLocationTag = true, want false", tag)
		}
	}
}

func TestReconcilePreviewRejectsMalformedAssetTag(t *testing.T) {
	// A valid location code alongside a garbled asset tag must reject the
	// whole preview rather than silently reconciling against a partial tag
	// set (which could show real items as falsely "removed").
	for _, tags := range [][]string{{"ZK3I"}, {"zkei"}, {"ZKEIX"}, {"ZKEI", "O0F9"}} {
		fake := &gemini.Fake{ReconciliationResult: gemini.ReconciliationResult{HasLocationTag: true, LocationTag: "@XYZ", AssetTags: tags}}
		h, cookies, _ := newTestServerWithGemini(t, fake)

		req := doMultipartUpload(t, h, "/api/reconcile/preview", cookies, []byte("photo"), nil)
		if req.Code != http.StatusOK {
			t.Fatalf("tags %v: status = %d, body = %s", tags, req.Code, req.Body.String())
		}
		var resp reconcileDiffResponse
		json.NewDecoder(req.Body).Decode(&resp)
		if resp.HasLocationTag {
			t.Errorf("tags %v: HasLocationTag = true, want false", tags)
		}
	}
}

func TestReconcilePreviewIncludesSuggestedRotation(t *testing.T) {
	// The locate flow's dual-read cross-check rotates the same frame using
	// whichever direction Gemini suggests on the first (straight) read —
	// that value has to survive onto the preview response for the frontend
	// to act on it.
	fake := &gemini.Fake{ReconciliationResult: gemini.ReconciliationResult{
		HasLocationTag:    true,
		LocationTag:       "@XYZ",
		AssetTags:         []string{"ZKEI"},
		SuggestedRotation: "counterclockwise",
	}}
	h, cookies, _ := newTestServerWithGemini(t, fake)

	req := doMultipartUpload(t, h, "/api/reconcile/preview", cookies, []byte("photo"), nil)
	if req.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", req.Code, req.Body.String())
	}
	var resp reconcileDiffResponse
	json.NewDecoder(req.Body).Decode(&resp)
	if resp.SuggestedRotation != "counterclockwise" {
		t.Errorf("SuggestedRotation = %q, want %q", resp.SuggestedRotation, "counterclockwise")
	}
}

func TestReconcileDiffComputesWithoutCallingGemini(t *testing.T) {
	// /api/reconcile/diff backs the tag-agreement review step after two
	// dual-read analyses disagree: the frontend already has a resolved tag
	// list by then and just needs a fresh diff, no further Gemini call.
	fake := &gemini.Fake{ReconciliationErr: errors.New("must not call Gemini")}
	h, cookies, _ := newTestServerWithGemini(t, fake)

	fake.TagCaptureResult = gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "ZKEI"}
	doCaptureUpload(t, h, cookies, []byte("zkei-photo"))

	w := doJSON(t, h, http.MethodPost, "/api/reconcile/diff", applyReconcileRequest{LocationTag: "@XYZ", AssetTags: []string{"ZKEI"}}, cookies)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp reconcileDiffResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// ZKEI already exists as an unlinked item (captured above), so linking it
	// to a location classifies as Added, not New (see ComputeReconciliation).
	if len(resp.Added) != 1 || resp.Added[0] != "ZKEI" {
		t.Errorf("Added = %v, want [ZKEI]", resp.Added)
	}
}

func TestReconcileDiffRejectsMalformedInput(t *testing.T) {
	fake := &gemini.Fake{}
	h, cookies, _ := newTestServerWithGemini(t, fake)

	w := doJSON(t, h, http.MethodPost, "/api/reconcile/diff", applyReconcileRequest{LocationTag: "@XY", AssetTags: []string{"ZKEI"}}, cookies)
	if w.Code != http.StatusBadRequest {
		t.Errorf("malformed location_tag: status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	w = doJSON(t, h, http.MethodPost, "/api/reconcile/diff", applyReconcileRequest{LocationTag: "@XYZ", AssetTags: []string{"zkei"}}, cookies)
	if w.Code != http.StatusBadRequest {
		t.Errorf("malformed asset_tags: status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestReconcileApplyRejectsMalformedInput(t *testing.T) {
	fake := &gemini.Fake{}
	h, cookies, _ := newTestServerWithGemini(t, fake)

	w := doJSON(t, h, http.MethodPost, "/api/reconcile/apply", applyReconcileRequest{LocationTag: "@XY", AssetTags: []string{"ZKEI"}}, cookies)
	if w.Code != http.StatusBadRequest {
		t.Errorf("malformed location_tag: status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	w = doJSON(t, h, http.MethodPost, "/api/reconcile/apply", applyReconcileRequest{LocationTag: "@XYZ", AssetTags: []string{"zkei"}}, cookies)
	if w.Code != http.StatusBadRequest {
		t.Errorf("malformed asset_tags: status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func applyReconcile(t *testing.T, h http.Handler, cookies []*http.Cookie, locationTag string, assetTags []string) reconcileDiffResponse {
	t.Helper()
	w := doJSON(t, h, http.MethodPost, "/api/reconcile/apply", applyReconcileRequest{LocationTag: locationTag, AssetTags: assetTags}, cookies)
	if w.Code != http.StatusOK {
		t.Fatalf("apply status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp reconcileDiffResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode apply response: %v", err)
	}
	return resp
}

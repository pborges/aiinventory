package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/pborges/aiinventory/internal/gemini"
)

func TestReconcilePreviewNoLocationCode(t *testing.T) {
	fake := &gemini.Fake{ReconciliationResult: gemini.ReconciliationResult{HasLocationCode: false}}
	h, cookies := newTestServerWithGemini(t, fake)

	req := doMultipartUpload(t, h, "/api/reconcile/preview", cookies, []byte("photo"), nil)
	if req.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", req.Code, req.Body.String())
	}
	var resp reconcileDiffResponse
	json.NewDecoder(req.Body).Decode(&resp)
	if resp.HasLocationCode {
		t.Fatalf("HasLocationCode = true, want false")
	}
}

func TestReconcilePreviewAndApplyFullFlow(t *testing.T) {
	fake := &gemini.Fake{}
	h, cookies := newTestServerWithGemini(t, fake)

	// seed: ZKEI unlinked, GKEI at @QRS, XDKW currently at @XYZ (about to be removed)
	fake.TagCaptureResult = gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "ZKEI"}
	doCaptureUpload(t, h, cookies, []byte("zkei-photo"))
	fake.TagCaptureResult = gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "GKEI"}
	doCaptureUpload(t, h, cookies, []byte("gkei-photo"))
	fake.TagCaptureResult = gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "XDKW"}
	doCaptureUpload(t, h, cookies, []byte("xdkw-photo"))

	// put GKEI at @QRS and XDKW at @XYZ via a first reconciliation each
	fake.ReconciliationResult = gemini.ReconciliationResult{HasLocationCode: true, LocationCode: "@QRS", AssetTags: []string{"GKEI"}}
	applyReconcile(t, h, cookies, "@QRS", []string{"GKEI"})
	fake.ReconciliationResult = gemini.ReconciliationResult{HasLocationCode: true, LocationCode: "@XYZ", AssetTags: []string{"XDKW"}}
	applyReconcile(t, h, cookies, "@XYZ", []string{"XDKW"})

	// now reconcile @XYZ against a frame containing ZKEI + GKEI (not XDKW):
	// ZKEI -> added, GKEI -> moved from @QRS, XDKW -> removed
	fake.ReconciliationResult = gemini.ReconciliationResult{HasLocationCode: true, LocationCode: "@XYZ", AssetTags: []string{"ZKEI", "GKEI"}}
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

	// approve: apply the same location_code + asset_tags
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

func TestReconcilePreviewRejectsMalformedLocationCode(t *testing.T) {
	// Gemini's JSON schema only constrains this to a string — a misread
	// (wrong letter count, stray digit, lowercase) can still come back as
	// "valid" JSON. The deterministic shape check must catch it before a
	// diff is ever computed/shown.
	for _, code := range []string{"@XY", "@WXYZ", "@xyz", "@X1Z", "XYZ", "@"} {
		fake := &gemini.Fake{ReconciliationResult: gemini.ReconciliationResult{HasLocationCode: true, LocationCode: code, AssetTags: []string{"ZKEI"}}}
		h, cookies := newTestServerWithGemini(t, fake)

		req := doMultipartUpload(t, h, "/api/reconcile/preview", cookies, []byte("photo"), nil)
		if req.Code != http.StatusOK {
			t.Fatalf("code %q: status = %d, body = %s", code, req.Code, req.Body.String())
		}
		var resp reconcileDiffResponse
		json.NewDecoder(req.Body).Decode(&resp)
		if resp.HasLocationCode {
			t.Errorf("code %q: HasLocationCode = true, want false", code)
		}
	}
}

func TestReconcilePreviewRejectsMalformedAssetTag(t *testing.T) {
	// A valid location code alongside a garbled asset tag must reject the
	// whole preview rather than silently reconciling against a partial tag
	// set (which could show real items as falsely "removed").
	for _, tags := range [][]string{{"ZK3I"}, {"zkei"}, {"ZKEIX"}, {"ZKEI", "O0F9"}} {
		fake := &gemini.Fake{ReconciliationResult: gemini.ReconciliationResult{HasLocationCode: true, LocationCode: "@XYZ", AssetTags: tags}}
		h, cookies := newTestServerWithGemini(t, fake)

		req := doMultipartUpload(t, h, "/api/reconcile/preview", cookies, []byte("photo"), nil)
		if req.Code != http.StatusOK {
			t.Fatalf("tags %v: status = %d, body = %s", tags, req.Code, req.Body.String())
		}
		var resp reconcileDiffResponse
		json.NewDecoder(req.Body).Decode(&resp)
		if resp.HasLocationCode {
			t.Errorf("tags %v: HasLocationCode = true, want false", tags)
		}
	}
}

func TestReconcileApplyRejectsMalformedInput(t *testing.T) {
	fake := &gemini.Fake{}
	h, cookies := newTestServerWithGemini(t, fake)

	w := doJSON(t, h, http.MethodPost, "/api/reconcile/apply", applyReconcileRequest{LocationCode: "@XY", AssetTags: []string{"ZKEI"}}, cookies)
	if w.Code != http.StatusBadRequest {
		t.Errorf("malformed location_code: status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	w = doJSON(t, h, http.MethodPost, "/api/reconcile/apply", applyReconcileRequest{LocationCode: "@XYZ", AssetTags: []string{"zkei"}}, cookies)
	if w.Code != http.StatusBadRequest {
		t.Errorf("malformed asset_tags: status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func applyReconcile(t *testing.T, h http.Handler, cookies []*http.Cookie, locationCode string, assetTags []string) reconcileDiffResponse {
	t.Helper()
	w := doJSON(t, h, http.MethodPost, "/api/reconcile/apply", applyReconcileRequest{LocationCode: locationCode, AssetTags: assetTags}, cookies)
	if w.Code != http.StatusOK {
		t.Fatalf("apply status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp reconcileDiffResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode apply response: %v", err)
	}
	return resp
}

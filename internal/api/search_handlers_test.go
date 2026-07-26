package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/pborges/aiinventory/internal/gemini"
)

func TestSearchAndImageServing(t *testing.T) {
	fake := &gemini.Fake{TagCaptureResult: gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "ZKEI", Description: "S/N 1"}}
	h, cookies := newTestServerWithGemini(t, fake)

	captureResp := doCaptureUpload(t, h, cookies, []byte("photo-bytes"))
	var captured captureResponse
	json.NewDecoder(captureResp.Body).Decode(&captured)

	w := doJSON(t, h, http.MethodGet, "/api/search", nil, cookies)
	if w.Code != http.StatusOK {
		t.Fatalf("search status = %d, body = %s", w.Code, w.Body.String())
	}
	var searchResp struct {
		Items []itemSummaryResponse `json:"items"`
	}
	json.NewDecoder(w.Body).Decode(&searchResp)
	if len(searchResp.Items) != 1 || searchResp.Items[0].AssetTag != "ZKEI" {
		t.Fatalf("search results = %+v", searchResp.Items)
	}
	if searchResp.Items[0].PrimaryImageID == nil {
		t.Fatal("expected a primary_image_id after one capture")
	}

	// the served image bytes match what was uploaded
	imgReq := doJSON(t, h, http.MethodGet, "/api/images/"+strconv.FormatInt(*searchResp.Items[0].PrimaryImageID, 10), nil, cookies)
	if imgReq.Code != http.StatusOK {
		t.Fatalf("image status = %d", imgReq.Code)
	}
	if imgReq.Body.String() != "photo-bytes" {
		t.Fatalf("image bytes = %q, want photo-bytes", imgReq.Body.String())
	}

	// no-description filter excludes it once a description exists
	filtered := doJSON(t, h, http.MethodGet, "/api/search?no_description=1", nil, cookies)
	var filteredResp struct {
		Items []itemSummaryResponse `json:"items"`
	}
	json.NewDecoder(filtered.Body).Decode(&filteredResp)
	if len(filteredResp.Items) != 1 {
		t.Fatalf("no_description results = %+v, want ZKEI (description not yet consolidated)", filteredResp.Items)
	}
}

func TestBulkDelete(t *testing.T) {
	fake := &gemini.Fake{TagCaptureResult: gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "ZKEI"}}
	h, cookies := newTestServerWithGemini(t, fake)

	captureResp := doCaptureUpload(t, h, cookies, []byte("photo"))
	var captured captureResponse
	json.NewDecoder(captureResp.Body).Decode(&captured)

	w := doJSON(t, h, http.MethodPost, "/api/items/bulk-delete", bulkItemIDsRequest{ItemIDs: []int64{captured.ItemID}}, cookies)
	if w.Code != http.StatusOK {
		t.Fatalf("bulk-delete status = %d, body = %s", w.Code, w.Body.String())
	}

	after := doJSON(t, h, http.MethodGet, "/api/search", nil, cookies)
	var afterResp struct {
		Items []itemSummaryResponse `json:"items"`
	}
	json.NewDecoder(after.Body).Decode(&afterResp)
	if len(afterResp.Items) != 0 {
		t.Fatalf("items after bulk delete = %+v, want none", afterResp.Items)
	}
}

func TestBulkRegenerateDescription(t *testing.T) {
	fake := &gemini.Fake{TagCaptureResult: gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "ZKEI", Description: "S/N 1"}}
	h, cookies := newTestServerWithGemini(t, fake)

	captureResp := doCaptureUpload(t, h, cookies, []byte("photo"))
	var captured captureResponse
	json.NewDecoder(captureResp.Body).Decode(&captured)

	fake.DescriptionResult = gemini.DescriptionResult{Description: "a cordless drill, S/N 1"}
	w := doJSON(t, h, http.MethodPost, "/api/items/bulk-regenerate-description", bulkItemIDsRequest{ItemIDs: []int64{captured.ItemID}}, cookies)
	if w.Code != http.StatusOK {
		t.Fatalf("bulk-regenerate status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		Results []regenerateDescriptionResult `json:"results"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Results) != 1 || resp.Results[0].Description != "a cordless drill, S/N 1" {
		t.Fatalf("results = %+v", resp.Results)
	}

	after := doJSON(t, h, http.MethodGet, "/api/search?no_description=1", nil, cookies)
	var afterResp struct {
		Items []itemSummaryResponse `json:"items"`
	}
	json.NewDecoder(after.Body).Decode(&afterResp)
	if len(afterResp.Items) != 0 {
		t.Fatalf("item should no longer match no_description after regeneration: %+v", afterResp.Items)
	}
}

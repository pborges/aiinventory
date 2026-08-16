package api

import (
	"encoding/json"
	"image/jpeg"
	"net/http"
	"strconv"
	"testing"

	"github.com/pborges/aiinventory/internal/gemini"
)

func TestSearchAndImageServing(t *testing.T) {
	fake := &gemini.Fake{TagCaptureResult: gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "ZKEI", Description: "S/N 1"}}
	h, cookies, _ := newTestServerWithGemini(t, fake)

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

	// the served bytes are a canonical, decodable JPEG regardless of upload
	// metadata or source encoding.
	imgReq := doJSON(t, h, http.MethodGet, "/api/images/"+strconv.FormatInt(*searchResp.Items[0].PrimaryImageID, 10), nil, cookies)
	if imgReq.Code != http.StatusOK {
		t.Fatalf("image status = %d", imgReq.Code)
	}
	if got := imgReq.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Fatalf("Content-Type = %q, want image/jpeg", got)
	}
	if got := imgReq.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if _, err := jpeg.Decode(imgReq.Body); err != nil {
		t.Fatalf("served image is not a JPEG: %v", err)
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

func TestSearchByLocationLabel(t *testing.T) {
	fake := &gemini.Fake{TagCaptureResult: gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "ZKEI"}}
	h, cookies, _ := newTestServerWithGemini(t, fake)

	captureResp := doCaptureUpload(t, h, cookies, []byte("photo"))
	var captured captureResponse
	json.NewDecoder(captureResp.Body).Decode(&captured)

	fake.ReconciliationResult = gemini.ReconciliationResult{HasLocationTag: true, LocationTag: "@XYZ", AssetTags: []string{"ZKEI"}}
	applyReconcile(t, h, cookies, "@XYZ", []string{"ZKEI"})

	locResp := doJSON(t, h, http.MethodGet, "/api/locations", nil, cookies)
	var locBody struct {
		Locations []locationResponse `json:"locations"`
	}
	json.NewDecoder(locResp.Body).Decode(&locBody)
	loc := locBody.Locations[0]

	createResp := doJSON(t, h, http.MethodPost, "/api/location-labels", labelRequest{Name: "warehouse", Color: "#a6e22e"}, cookies)
	var createBody struct {
		Label labelResponse `json:"label"`
	}
	json.NewDecoder(createResp.Body).Decode(&createBody)

	locIDStr := strconv.FormatInt(loc.ID, 10)
	doJSON(t, h, http.MethodPut, "/api/locations/"+locIDStr+"/labels", setLocationLabelsRequest{LabelIDs: []int64{createBody.Label.ID}}, cookies)

	labelIDStr := strconv.FormatInt(createBody.Label.ID, 10)
	matched := doJSON(t, h, http.MethodGet, "/api/search?location_label_id="+labelIDStr, nil, cookies)
	var matchedResp struct {
		Items []itemSummaryResponse `json:"items"`
	}
	json.NewDecoder(matched.Body).Decode(&matchedResp)
	if len(matchedResp.Items) != 1 || matchedResp.Items[0].AssetTag != "ZKEI" {
		t.Fatalf("location_label_id search results = %+v, want [ZKEI]", matchedResp.Items)
	}

	unmatched := doJSON(t, h, http.MethodGet, "/api/search?location_label_id=999999", nil, cookies)
	var unmatchedResp struct {
		Items []itemSummaryResponse `json:"items"`
	}
	json.NewDecoder(unmatched.Body).Decode(&unmatchedResp)
	if len(unmatchedResp.Items) != 0 {
		t.Fatalf("location_label_id search (unused label) results = %+v, want none", unmatchedResp.Items)
	}
}

func TestBulkDelete(t *testing.T) {
	fake := &gemini.Fake{TagCaptureResult: gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "ZKEI"}}
	h, cookies, _ := newTestServerWithGemini(t, fake)

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

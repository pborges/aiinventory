package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

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

func waitForDescriptionBatchDone(t *testing.T, h http.Handler, cookies []*http.Cookie) []descriptionBatchItemResponse {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		w := doJSON(t, h, http.MethodGet, "/api/items/bulk-regenerate-description/status", nil, cookies)
		var status struct {
			Running bool                           `json:"running"`
			Items   []descriptionBatchItemResponse `json:"items"`
		}
		json.NewDecoder(w.Body).Decode(&status)
		if !status.Running {
			return status.Items
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("description batch did not finish within the timeout")
	return nil
}

func TestBulkRegenerateDescription(t *testing.T) {
	fake := &gemini.Fake{TagCaptureResult: gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "ZKEI", Description: "S/N 1"}}
	h, cookies := newTestServerWithGemini(t, fake)

	captureResp := doCaptureUpload(t, h, cookies, []byte("photo"))
	var captured captureResponse
	json.NewDecoder(captureResp.Body).Decode(&captured)

	// status before any batch has run
	statusResp := doJSON(t, h, http.MethodGet, "/api/items/bulk-regenerate-description/status", nil, cookies)
	var status struct {
		Running bool `json:"running"`
	}
	json.NewDecoder(statusResp.Body).Decode(&status)
	if status.Running {
		t.Fatal("should not be running before any batch has started")
	}

	fake.DescriptionResult = gemini.DescriptionResult{Description: "a cordless drill, S/N 1"}
	runResp := doJSON(t, h, http.MethodPost, "/api/items/bulk-regenerate-description",
		bulkRegenerateDescriptionRequest{Items: []struct {
			ItemID int64  `json:"item_id"`
			Hint   string `json:"hint"`
		}{{ItemID: captured.ItemID, Hint: "blue enclosure"}}}, cookies)
	if runResp.Code != http.StatusAccepted {
		t.Fatalf("bulk-regenerate status = %d, body = %s", runResp.Code, runResp.Body.String())
	}

	items := waitForDescriptionBatchDone(t, h, cookies)
	if len(items) != 1 || items[0].Status != "done" || items[0].Description != "a cordless drill, S/N 1" {
		t.Fatalf("batch items = %+v", items)
	}
	if items[0].AssetTag != "ZKEI" {
		t.Errorf("AssetTag = %q, want ZKEI", items[0].AssetTag)
	}
	if items[0].Hint != "blue enclosure" {
		t.Errorf("Hint = %q, want blue enclosure", items[0].Hint)
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

func TestBulkRegenerateDescriptionRejectsConcurrentBatch(t *testing.T) {
	fake := &gemini.Fake{TagCaptureResult: gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "ZKEI"}}
	h, cookies := newTestServerWithGemini(t, fake)

	captureResp := doCaptureUpload(t, h, cookies, []byte("photo"))
	var captured captureResponse
	json.NewDecoder(captureResp.Body).Decode(&captured)

	// block the fake's Gemini call so the batch stays "running" long enough
	// to observe a genuine conflict
	unblock := make(chan struct{})
	fake.DescriptionFunc = func(_ string, _ []string, _ string) (gemini.DescriptionResult, error) {
		<-unblock
		return gemini.DescriptionResult{Description: "done"}, nil
	}
	defer close(unblock)

	items := []struct {
		ItemID int64  `json:"item_id"`
		Hint   string `json:"hint"`
	}{{ItemID: captured.ItemID}}

	first := doJSON(t, h, http.MethodPost, "/api/items/bulk-regenerate-description", bulkRegenerateDescriptionRequest{Items: items}, cookies)
	if first.Code != http.StatusAccepted {
		t.Fatalf("first batch status = %d, body = %s", first.Code, first.Body.String())
	}

	second := doJSON(t, h, http.MethodPost, "/api/items/bulk-regenerate-description", bulkRegenerateDescriptionRequest{Items: items}, cookies)
	if second.Code != http.StatusConflict {
		t.Fatalf("second (concurrent) batch status = %d, want 409, body = %s", second.Code, second.Body.String())
	}
}

package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/pborges/aiinventory/internal/gemini"
)

func TestGetItemDetail(t *testing.T) {
	fake := &gemini.Fake{TagCaptureResult: gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "ZKEI", Description: "S/N 1"}}
	h, cookies := newTestServerWithGemini(t, fake)

	captureResp := doCaptureUpload(t, h, cookies, []byte("photo-1"))
	var captured captureResponse
	json.NewDecoder(captureResp.Body).Decode(&captured)
	doCaptureUpload(t, h, cookies, []byte("photo-2")) // second image on the same item

	w := doJSON(t, h, http.MethodGet, "/api/items/"+strconv.FormatInt(captured.ItemID, 10), nil, cookies)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var detail itemDetailResponse
	json.NewDecoder(w.Body).Decode(&detail)

	if detail.AssetTag != "ZKEI" {
		t.Errorf("AssetTag = %q", detail.AssetTag)
	}
	if len(detail.Images) != 2 {
		t.Fatalf("got %d images, want 2", len(detail.Images))
	}
	if len(detail.Activity) != 2 { // item_created + image_added
		t.Fatalf("got %d activity entries, want 2: %+v", len(detail.Activity), detail.Activity)
	}
}

func TestGetItemDetailNotFound(t *testing.T) {
	h, cookies := newTestServerWithGemini(t, nil)
	w := doJSON(t, h, http.MethodGet, "/api/items/999", nil, cookies)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestUpdateItemDescription(t *testing.T) {
	fake := &gemini.Fake{TagCaptureResult: gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "ZKEI"}}
	h, cookies := newTestServerWithGemini(t, fake)
	captureResp := doCaptureUpload(t, h, cookies, []byte("photo"))
	var captured captureResponse
	json.NewDecoder(captureResp.Body).Decode(&captured)

	w := doJSON(t, h, http.MethodPut, "/api/items/"+strconv.FormatInt(captured.ItemID, 10), updateItemRequest{Description: "hand-edited description"}, cookies)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var detail itemDetailResponse
	json.NewDecoder(w.Body).Decode(&detail)
	if detail.Description != "hand-edited description" {
		t.Errorf("Description = %q", detail.Description)
	}

	found := false
	for _, a := range detail.Activity {
		if a.Action == "description_edited" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a description_edited activity entry, got %+v", detail.Activity)
	}
}

func TestReorderImagesEndpoint(t *testing.T) {
	fake := &gemini.Fake{TagCaptureResult: gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "ZKEI"}}
	h, cookies := newTestServerWithGemini(t, fake)

	first := doCaptureUpload(t, h, cookies, []byte("photo-1"))
	var firstResp captureResponse
	json.NewDecoder(first.Body).Decode(&firstResp)
	doCaptureUpload(t, h, cookies, []byte("photo-2"))

	detailResp := doJSON(t, h, http.MethodGet, "/api/items/"+strconv.FormatInt(firstResp.ItemID, 10), nil, cookies)
	var detail itemDetailResponse
	json.NewDecoder(detailResp.Body).Decode(&detail)
	if len(detail.Images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(detail.Images))
	}
	firstID, secondID := detail.Images[0].ID, detail.Images[1].ID

	reordered := doJSON(t, h, http.MethodPut, "/api/items/"+strconv.FormatInt(firstResp.ItemID, 10)+"/images/order",
		reorderImagesRequest{ImageIDs: []int64{secondID, firstID}}, cookies)
	if reordered.Code != http.StatusOK {
		t.Fatalf("reorder status = %d, body = %s", reordered.Code, reordered.Body.String())
	}
	var reorderedDetail itemDetailResponse
	json.NewDecoder(reordered.Body).Decode(&reorderedDetail)
	if reorderedDetail.Images[0].ID != secondID || reorderedDetail.Images[1].ID != firstID {
		t.Fatalf("order after reorder = %+v, want [%d %d]", reorderedDetail.Images, secondID, firstID)
	}
}

func TestDeleteImageEndpoint(t *testing.T) {
	fake := &gemini.Fake{TagCaptureResult: gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "ZKEI"}}
	h, cookies := newTestServerWithGemini(t, fake)

	first := doCaptureUpload(t, h, cookies, []byte("photo-1"))
	var firstResp captureResponse
	json.NewDecoder(first.Body).Decode(&firstResp)
	doCaptureUpload(t, h, cookies, []byte("photo-2"))

	detailResp := doJSON(t, h, http.MethodGet, "/api/items/"+strconv.FormatInt(firstResp.ItemID, 10), nil, cookies)
	var detail itemDetailResponse
	json.NewDecoder(detailResp.Body).Decode(&detail)
	if len(detail.Images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(detail.Images))
	}
	toDelete, toKeep := detail.Images[0].ID, detail.Images[1].ID

	deleteResp := doJSON(t, h, http.MethodDelete,
		"/api/items/"+strconv.FormatInt(firstResp.ItemID, 10)+"/images/"+strconv.FormatInt(toDelete, 10), nil, cookies)
	if deleteResp.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", deleteResp.Code, deleteResp.Body.String())
	}
	var afterDelete itemDetailResponse
	json.NewDecoder(deleteResp.Body).Decode(&afterDelete)
	if len(afterDelete.Images) != 1 || afterDelete.Images[0].ID != toKeep {
		t.Fatalf("images after delete = %+v, want just [%d]", afterDelete.Images, toKeep)
	}

	found := false
	for _, a := range afterDelete.Activity {
		if a.Action == "image_deleted" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an image_deleted activity entry, got %+v", afterDelete.Activity)
	}

	// deleting an image that belongs to a different item is rejected
	other := doCaptureUpload(t, h, cookies, []byte("photo-3"))
	var otherResp captureResponse
	json.NewDecoder(other.Body).Decode(&otherResp)
	crossDelete := doJSON(t, h, http.MethodDelete,
		"/api/items/"+strconv.FormatInt(firstResp.ItemID, 10)+"/images/"+strconv.FormatInt(otherResp.ItemID, 10), nil, cookies)
	if crossDelete.Code != http.StatusNotFound {
		t.Fatalf("cross-item delete status = %d, want 404", crossDelete.Code)
	}
}

func TestRegenerateItemDescriptionEndpoint(t *testing.T) {
	fake := &gemini.Fake{TagCaptureResult: gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "ZKEI", Description: "S/N 12345"}}
	h, cookies := newTestServerWithGemini(t, fake)

	captureResp := doCaptureUpload(t, h, cookies, []byte("photo"))
	var captured captureResponse
	json.NewDecoder(captureResp.Body).Decode(&captured)

	fake.DescriptionResult = gemini.DescriptionResult{Description: "a cordless drill, S/N 12345"}
	w := doJSON(t, h, http.MethodPost, "/api/items/"+strconv.FormatInt(captured.ItemID, 10)+"/regenerate-description", nil, cookies)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var detail itemDetailResponse
	json.NewDecoder(w.Body).Decode(&detail)
	if detail.Description != "a cordless drill, S/N 12345" {
		t.Errorf("Description = %q", detail.Description)
	}

	found := false
	for _, a := range detail.Activity {
		if a.Action == "description_regenerated" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a description_regenerated activity entry, got %+v", detail.Activity)
	}
}

func TestRegenerateItemDescriptionWithoutGemini(t *testing.T) {
	h, cookies := newTestServerWithGemini(t, nil)
	w := doJSON(t, h, http.MethodPost, "/api/items/1/regenerate-description", nil, cookies)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

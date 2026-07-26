package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/pborges/aiinventory/internal/gemini"
)

func TestTagCRUDFlow(t *testing.T) {
	h, cookies := newTestServerWithGemini(t, nil)

	createResp := doJSON(t, h, http.MethodPost, "/api/tags", tagRequest{Name: "fragile", Color: "#a6e22e"}, cookies)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createResp.Code, createResp.Body.String())
	}
	var createBody struct {
		Tag tagResponse `json:"tag"`
	}
	json.NewDecoder(createResp.Body).Decode(&createBody)
	if createBody.Tag.Name != "fragile" || createBody.Tag.Color != "#a6e22e" {
		t.Fatalf("created tag = %+v", createBody.Tag)
	}

	// duplicate name is rejected
	dupResp := doJSON(t, h, http.MethodPost, "/api/tags", tagRequest{Name: "fragile", Color: "#f92672"}, cookies)
	if dupResp.Code != http.StatusConflict {
		t.Fatalf("duplicate create status = %d, want 409", dupResp.Code)
	}

	// list includes it
	listResp := doJSON(t, h, http.MethodGet, "/api/tags", nil, cookies)
	var listBody struct {
		Tags []tagResponse `json:"tags"`
	}
	json.NewDecoder(listResp.Body).Decode(&listBody)
	if len(listBody.Tags) != 1 || listBody.Tags[0].ID != createBody.Tag.ID {
		t.Fatalf("list tags = %+v", listBody.Tags)
	}

	// rename + recolor
	idStr := strconv.FormatInt(createBody.Tag.ID, 10)
	updateResp := doJSON(t, h, http.MethodPut, "/api/tags/"+idStr, tagRequest{Name: "handle with care", Color: "#f92672"}, cookies)
	if updateResp.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", updateResp.Code, updateResp.Body.String())
	}
	var updateBody struct {
		Tag tagResponse `json:"tag"`
	}
	json.NewDecoder(updateResp.Body).Decode(&updateBody)
	if updateBody.Tag.Name != "handle with care" || updateBody.Tag.Color != "#f92672" {
		t.Fatalf("updated tag = %+v", updateBody.Tag)
	}

	// delete
	deleteResp := doJSON(t, h, http.MethodDelete, "/api/tags/"+idStr, nil, cookies)
	if deleteResp.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", deleteResp.Code, deleteResp.Body.String())
	}

	afterDeleteResp := doJSON(t, h, http.MethodGet, "/api/tags", nil, cookies)
	var afterDeleteBody struct {
		Tags []tagResponse `json:"tags"`
	}
	json.NewDecoder(afterDeleteResp.Body).Decode(&afterDeleteBody)
	if len(afterDeleteBody.Tags) != 0 {
		t.Fatalf("tags after delete = %+v, want none", afterDeleteBody.Tags)
	}
}

func TestCreateTagValidation(t *testing.T) {
	h, cookies := newTestServerWithGemini(t, nil)
	w := doJSON(t, h, http.MethodPost, "/api/tags", tagRequest{Name: "", Color: "#a6e22e"}, cookies)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestUpdateTagNotFound(t *testing.T) {
	h, cookies := newTestServerWithGemini(t, nil)
	w := doJSON(t, h, http.MethodPut, "/api/tags/999", tagRequest{Name: "x", Color: "#a6e22e"}, cookies)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestTagsRequireAuth(t *testing.T) {
	h, _ := newTestServerWithGemini(t, nil)
	w := doJSON(t, h, http.MethodGet, "/api/tags", nil, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestSetItemTags(t *testing.T) {
	fake := &gemini.Fake{TagCaptureResult: gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "ZKEI"}}
	h, cookies := newTestServerWithGemini(t, fake)

	captureResp := doCaptureUpload(t, h, cookies, []byte("photo"))
	var captured captureResponse
	json.NewDecoder(captureResp.Body).Decode(&captured)
	itemIDStr := strconv.FormatInt(captured.ItemID, 10)

	createResp := doJSON(t, h, http.MethodPost, "/api/tags", tagRequest{Name: "fragile", Color: "#a6e22e"}, cookies)
	var createBody struct {
		Tag tagResponse `json:"tag"`
	}
	json.NewDecoder(createResp.Body).Decode(&createBody)

	setResp := doJSON(t, h, http.MethodPut, "/api/items/"+itemIDStr+"/tags", setItemTagsRequest{TagIDs: []int64{createBody.Tag.ID}}, cookies)
	if setResp.Code != http.StatusOK {
		t.Fatalf("set item tags status = %d, body = %s", setResp.Code, setResp.Body.String())
	}
	var detail itemDetailResponse
	json.NewDecoder(setResp.Body).Decode(&detail)
	if len(detail.Tags) != 1 || detail.Tags[0].ID != createBody.Tag.ID {
		t.Fatalf("item tags after set = %+v, want [fragile]", detail.Tags)
	}

	found := false
	for _, a := range detail.Activity {
		if a.Action == "item_tags_updated" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an item_tags_updated activity entry, got %+v", detail.Activity)
	}

	// search results include the tag too
	searchResp := doJSON(t, h, http.MethodGet, "/api/search", nil, cookies)
	var searchBody struct {
		Items []itemSummaryResponse `json:"items"`
	}
	json.NewDecoder(searchResp.Body).Decode(&searchBody)
	if len(searchBody.Items) != 1 || len(searchBody.Items[0].Tags) != 1 || searchBody.Items[0].Tags[0].ID != createBody.Tag.ID {
		t.Fatalf("search results = %+v, want one item tagged [fragile]", searchBody.Items)
	}

	// filter search by tag_id
	filteredResp := doJSON(t, h, http.MethodGet, "/api/search?tag_id="+strconv.FormatInt(createBody.Tag.ID, 10), nil, cookies)
	var filteredBody struct {
		Items []itemSummaryResponse `json:"items"`
	}
	json.NewDecoder(filteredResp.Body).Decode(&filteredBody)
	if len(filteredBody.Items) != 1 {
		t.Fatalf("filtered search results = %+v, want 1 item", filteredBody.Items)
	}

	// clearing tags
	clearResp := doJSON(t, h, http.MethodPut, "/api/items/"+itemIDStr+"/tags", setItemTagsRequest{TagIDs: nil}, cookies)
	var clearedDetail itemDetailResponse
	json.NewDecoder(clearResp.Body).Decode(&clearedDetail)
	if len(clearedDetail.Tags) != 0 {
		t.Fatalf("item tags after clear = %+v, want none", clearedDetail.Tags)
	}
}

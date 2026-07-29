package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/pborges/aiinventory/internal/gemini"
)

func TestLabelCRUDFlow(t *testing.T) {
	h, cookies, _ := newTestServerWithGemini(t, nil)

	createResp := doJSON(t, h, http.MethodPost, "/api/labels", labelRequest{Name: "fragile", Color: "#a6e22e"}, cookies)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createResp.Code, createResp.Body.String())
	}
	var createBody struct {
		Label labelResponse `json:"label"`
	}
	json.NewDecoder(createResp.Body).Decode(&createBody)
	if createBody.Label.Name != "fragile" || createBody.Label.Color != "#a6e22e" {
		t.Fatalf("created label = %+v", createBody.Label)
	}

	// duplicate name is rejected
	dupResp := doJSON(t, h, http.MethodPost, "/api/labels", labelRequest{Name: "fragile", Color: "#f92672"}, cookies)
	if dupResp.Code != http.StatusConflict {
		t.Fatalf("duplicate create status = %d, want 409", dupResp.Code)
	}

	// list includes it
	listResp := doJSON(t, h, http.MethodGet, "/api/labels", nil, cookies)
	var listBody struct {
		Labels []labelResponse `json:"labels"`
	}
	json.NewDecoder(listResp.Body).Decode(&listBody)
	if len(listBody.Labels) != 1 || listBody.Labels[0].ID != createBody.Label.ID {
		t.Fatalf("list labels = %+v", listBody.Labels)
	}

	// rename + recolor
	idStr := strconv.FormatInt(createBody.Label.ID, 10)
	updateResp := doJSON(t, h, http.MethodPut, "/api/labels/"+idStr, labelRequest{Name: "handle with care", Color: "#f92672"}, cookies)
	if updateResp.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", updateResp.Code, updateResp.Body.String())
	}
	var updateBody struct {
		Label labelResponse `json:"label"`
	}
	json.NewDecoder(updateResp.Body).Decode(&updateBody)
	if updateBody.Label.Name != "handle with care" || updateBody.Label.Color != "#f92672" {
		t.Fatalf("updated label = %+v", updateBody.Label)
	}

	// delete
	deleteResp := doJSON(t, h, http.MethodDelete, "/api/labels/"+idStr, nil, cookies)
	if deleteResp.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", deleteResp.Code, deleteResp.Body.String())
	}

	afterDeleteResp := doJSON(t, h, http.MethodGet, "/api/labels", nil, cookies)
	var afterDeleteBody struct {
		Labels []labelResponse `json:"labels"`
	}
	json.NewDecoder(afterDeleteResp.Body).Decode(&afterDeleteBody)
	if len(afterDeleteBody.Labels) != 0 {
		t.Fatalf("labels after delete = %+v, want none", afterDeleteBody.Labels)
	}
}

func TestCreateLabelValidation(t *testing.T) {
	h, cookies, _ := newTestServerWithGemini(t, nil)
	w := doJSON(t, h, http.MethodPost, "/api/labels", labelRequest{Name: "", Color: "#a6e22e"}, cookies)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestUpdateLabelNotFound(t *testing.T) {
	h, cookies, _ := newTestServerWithGemini(t, nil)
	w := doJSON(t, h, http.MethodPut, "/api/labels/999", labelRequest{Name: "x", Color: "#a6e22e"}, cookies)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestLabelsRequireAuth(t *testing.T) {
	h, _, _ := newTestServerWithGemini(t, nil)
	w := doJSON(t, h, http.MethodGet, "/api/labels", nil, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestSetItemLabels(t *testing.T) {
	fake := &gemini.Fake{TagCaptureResult: gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "ZKEI"}}
	h, cookies, _ := newTestServerWithGemini(t, fake)

	captureResp := doCaptureUpload(t, h, cookies, []byte("photo"))
	var captured captureResponse
	json.NewDecoder(captureResp.Body).Decode(&captured)
	itemIDStr := strconv.FormatInt(captured.ItemID, 10)

	createResp := doJSON(t, h, http.MethodPost, "/api/labels", labelRequest{Name: "fragile", Color: "#a6e22e"}, cookies)
	var createBody struct {
		Label labelResponse `json:"label"`
	}
	json.NewDecoder(createResp.Body).Decode(&createBody)

	setResp := doJSON(t, h, http.MethodPut, "/api/items/"+itemIDStr+"/labels", setItemLabelsRequest{LabelIDs: []int64{createBody.Label.ID}}, cookies)
	if setResp.Code != http.StatusOK {
		t.Fatalf("set item labels status = %d, body = %s", setResp.Code, setResp.Body.String())
	}
	var detail itemDetailResponse
	json.NewDecoder(setResp.Body).Decode(&detail)
	if len(detail.Labels) != 1 || detail.Labels[0].ID != createBody.Label.ID {
		t.Fatalf("item labels after set = %+v, want [fragile]", detail.Labels)
	}

	found := false
	for _, a := range detail.Activity {
		if a.Action == "item_labels_updated" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an item_labels_updated activity entry, got %+v", detail.Activity)
	}

	// search results include the label too
	searchResp := doJSON(t, h, http.MethodGet, "/api/search", nil, cookies)
	var searchBody struct {
		Items []itemSummaryResponse `json:"items"`
	}
	json.NewDecoder(searchResp.Body).Decode(&searchBody)
	if len(searchBody.Items) != 1 || len(searchBody.Items[0].Labels) != 1 || searchBody.Items[0].Labels[0].ID != createBody.Label.ID {
		t.Fatalf("search results = %+v, want one item labeled [fragile]", searchBody.Items)
	}

	// filter search by label_id
	filteredResp := doJSON(t, h, http.MethodGet, "/api/search?label_id="+strconv.FormatInt(createBody.Label.ID, 10), nil, cookies)
	var filteredBody struct {
		Items []itemSummaryResponse `json:"items"`
	}
	json.NewDecoder(filteredResp.Body).Decode(&filteredBody)
	if len(filteredBody.Items) != 1 {
		t.Fatalf("filtered search results = %+v, want 1 item", filteredBody.Items)
	}

	// clearing labels
	clearResp := doJSON(t, h, http.MethodPut, "/api/items/"+itemIDStr+"/labels", setItemLabelsRequest{LabelIDs: nil}, cookies)
	var clearedDetail itemDetailResponse
	json.NewDecoder(clearResp.Body).Decode(&clearedDetail)
	if len(clearedDetail.Labels) != 0 {
		t.Fatalf("item labels after clear = %+v, want none", clearedDetail.Labels)
	}
}

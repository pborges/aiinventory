package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/pborges/aiinventory/internal/gemini"
)

func TestLocationLabelCRUDFlow(t *testing.T) {
	h, cookies, _ := newTestServerWithGemini(t, nil)

	createResp := doJSON(t, h, http.MethodPost, "/api/location-labels", labelRequest{Name: "warehouse", Color: "#a6e22e"}, cookies)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createResp.Code, createResp.Body.String())
	}
	var createBody struct {
		Label labelResponse `json:"label"`
	}
	json.NewDecoder(createResp.Body).Decode(&createBody)
	if createBody.Label.Name != "warehouse" || createBody.Label.Color != "#a6e22e" {
		t.Fatalf("created label = %+v", createBody.Label)
	}

	// duplicate name is rejected
	dupResp := doJSON(t, h, http.MethodPost, "/api/location-labels", labelRequest{Name: "warehouse", Color: "#f92672"}, cookies)
	if dupResp.Code != http.StatusConflict {
		t.Fatalf("duplicate create status = %d, want 409", dupResp.Code)
	}

	// list includes it
	listResp := doJSON(t, h, http.MethodGet, "/api/location-labels", nil, cookies)
	var listBody struct {
		Labels []labelResponse `json:"labels"`
	}
	json.NewDecoder(listResp.Body).Decode(&listBody)
	if len(listBody.Labels) != 1 || listBody.Labels[0].ID != createBody.Label.ID {
		t.Fatalf("list location labels = %+v", listBody.Labels)
	}

	// rename + recolor
	idStr := strconv.FormatInt(createBody.Label.ID, 10)
	updateResp := doJSON(t, h, http.MethodPut, "/api/location-labels/"+idStr, labelRequest{Name: "cold storage", Color: "#f92672"}, cookies)
	if updateResp.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", updateResp.Code, updateResp.Body.String())
	}
	var updateBody struct {
		Label labelResponse `json:"label"`
	}
	json.NewDecoder(updateResp.Body).Decode(&updateBody)
	if updateBody.Label.Name != "cold storage" || updateBody.Label.Color != "#f92672" {
		t.Fatalf("updated label = %+v", updateBody.Label)
	}

	// delete
	deleteResp := doJSON(t, h, http.MethodDelete, "/api/location-labels/"+idStr, nil, cookies)
	if deleteResp.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", deleteResp.Code, deleteResp.Body.String())
	}

	afterDeleteResp := doJSON(t, h, http.MethodGet, "/api/location-labels", nil, cookies)
	var afterDeleteBody struct {
		Labels []labelResponse `json:"labels"`
	}
	json.NewDecoder(afterDeleteResp.Body).Decode(&afterDeleteBody)
	if len(afterDeleteBody.Labels) != 0 {
		t.Fatalf("location labels after delete = %+v, want none", afterDeleteBody.Labels)
	}
}

func TestCreateLocationLabelValidation(t *testing.T) {
	h, cookies, _ := newTestServerWithGemini(t, nil)
	w := doJSON(t, h, http.MethodPost, "/api/location-labels", labelRequest{Name: "", Color: "#a6e22e"}, cookies)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestUpdateLocationLabelNotFound(t *testing.T) {
	h, cookies, _ := newTestServerWithGemini(t, nil)
	w := doJSON(t, h, http.MethodPut, "/api/location-labels/999", labelRequest{Name: "x", Color: "#a6e22e"}, cookies)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestLocationLabelsRequireAuth(t *testing.T) {
	h, _, _ := newTestServerWithGemini(t, nil)
	w := doJSON(t, h, http.MethodGet, "/api/location-labels", nil, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestSetLocationLabels(t *testing.T) {
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
	if len(locBody.Locations) != 1 {
		t.Fatalf("locations = %+v, want 1", locBody.Locations)
	}
	loc := locBody.Locations[0]
	if len(loc.Labels) != 0 {
		t.Fatalf("new location labels = %+v, want none", loc.Labels)
	}

	createResp := doJSON(t, h, http.MethodPost, "/api/location-labels", labelRequest{Name: "warehouse", Color: "#a6e22e"}, cookies)
	var createBody struct {
		Label labelResponse `json:"label"`
	}
	json.NewDecoder(createResp.Body).Decode(&createBody)

	locIDStr := strconv.FormatInt(loc.ID, 10)
	setResp := doJSON(t, h, http.MethodPut, "/api/locations/"+locIDStr+"/labels", setLocationLabelsRequest{LabelIDs: []int64{createBody.Label.ID}}, cookies)
	if setResp.Code != http.StatusOK {
		t.Fatalf("set location labels status = %d, body = %s", setResp.Code, setResp.Body.String())
	}
	var setBody struct {
		Location locationResponse `json:"location"`
	}
	json.NewDecoder(setResp.Body).Decode(&setBody)
	if len(setBody.Location.Labels) != 1 || setBody.Location.Labels[0].ID != createBody.Label.ID {
		t.Fatalf("location labels after set = %+v, want [warehouse]", setBody.Location.Labels)
	}

	// the list endpoint reflects it too
	listResp := doJSON(t, h, http.MethodGet, "/api/locations", nil, cookies)
	json.NewDecoder(listResp.Body).Decode(&locBody)
	if len(locBody.Locations[0].Labels) != 1 || locBody.Locations[0].Labels[0].ID != createBody.Label.ID {
		t.Fatalf("locations list labels = %+v, want [warehouse]", locBody.Locations[0].Labels)
	}

	// activity log picked up the change
	activityResp := doJSON(t, h, http.MethodGet, "/api/locations/"+locIDStr+"/activity", nil, cookies)
	var activityBody struct {
		Activity []activityResponse `json:"activity"`
	}
	json.NewDecoder(activityResp.Body).Decode(&activityBody)
	found := false
	for _, a := range activityBody.Activity {
		if a.Action == "location_labels_updated" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a location_labels_updated activity entry, got %+v", activityBody.Activity)
	}

	// clearing labels
	clearResp := doJSON(t, h, http.MethodPut, "/api/locations/"+locIDStr+"/labels", setLocationLabelsRequest{LabelIDs: nil}, cookies)
	var clearBody struct {
		Location locationResponse `json:"location"`
	}
	json.NewDecoder(clearResp.Body).Decode(&clearBody)
	if len(clearBody.Location.Labels) != 0 {
		t.Fatalf("location labels after clear = %+v, want none", clearBody.Location.Labels)
	}
}

func TestSetLocationLabelsRequiresAuth(t *testing.T) {
	h, _, _ := newTestServerWithGemini(t, nil)
	w := doJSON(t, h, http.MethodPut, "/api/locations/1/labels", setLocationLabelsRequest{LabelIDs: nil}, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/pborges/aiinventory/internal/gemini"
)

func TestLocationTagCRUDFlow(t *testing.T) {
	h, cookies := newTestServerWithGemini(t, nil)

	createResp := doJSON(t, h, http.MethodPost, "/api/location-tags", tagRequest{Name: "warehouse", Color: "#a6e22e"}, cookies)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createResp.Code, createResp.Body.String())
	}
	var createBody struct {
		Tag tagResponse `json:"tag"`
	}
	json.NewDecoder(createResp.Body).Decode(&createBody)
	if createBody.Tag.Name != "warehouse" || createBody.Tag.Color != "#a6e22e" {
		t.Fatalf("created tag = %+v", createBody.Tag)
	}

	// duplicate name is rejected
	dupResp := doJSON(t, h, http.MethodPost, "/api/location-tags", tagRequest{Name: "warehouse", Color: "#f92672"}, cookies)
	if dupResp.Code != http.StatusConflict {
		t.Fatalf("duplicate create status = %d, want 409", dupResp.Code)
	}

	// list includes it
	listResp := doJSON(t, h, http.MethodGet, "/api/location-tags", nil, cookies)
	var listBody struct {
		Tags []tagResponse `json:"tags"`
	}
	json.NewDecoder(listResp.Body).Decode(&listBody)
	if len(listBody.Tags) != 1 || listBody.Tags[0].ID != createBody.Tag.ID {
		t.Fatalf("list location tags = %+v", listBody.Tags)
	}

	// rename + recolor
	idStr := strconv.FormatInt(createBody.Tag.ID, 10)
	updateResp := doJSON(t, h, http.MethodPut, "/api/location-tags/"+idStr, tagRequest{Name: "cold storage", Color: "#f92672"}, cookies)
	if updateResp.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", updateResp.Code, updateResp.Body.String())
	}
	var updateBody struct {
		Tag tagResponse `json:"tag"`
	}
	json.NewDecoder(updateResp.Body).Decode(&updateBody)
	if updateBody.Tag.Name != "cold storage" || updateBody.Tag.Color != "#f92672" {
		t.Fatalf("updated tag = %+v", updateBody.Tag)
	}

	// delete
	deleteResp := doJSON(t, h, http.MethodDelete, "/api/location-tags/"+idStr, nil, cookies)
	if deleteResp.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", deleteResp.Code, deleteResp.Body.String())
	}

	afterDeleteResp := doJSON(t, h, http.MethodGet, "/api/location-tags", nil, cookies)
	var afterDeleteBody struct {
		Tags []tagResponse `json:"tags"`
	}
	json.NewDecoder(afterDeleteResp.Body).Decode(&afterDeleteBody)
	if len(afterDeleteBody.Tags) != 0 {
		t.Fatalf("location tags after delete = %+v, want none", afterDeleteBody.Tags)
	}
}

func TestCreateLocationTagValidation(t *testing.T) {
	h, cookies := newTestServerWithGemini(t, nil)
	w := doJSON(t, h, http.MethodPost, "/api/location-tags", tagRequest{Name: "", Color: "#a6e22e"}, cookies)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestUpdateLocationTagNotFound(t *testing.T) {
	h, cookies := newTestServerWithGemini(t, nil)
	w := doJSON(t, h, http.MethodPut, "/api/location-tags/999", tagRequest{Name: "x", Color: "#a6e22e"}, cookies)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestLocationTagsRequireAuth(t *testing.T) {
	h, _ := newTestServerWithGemini(t, nil)
	w := doJSON(t, h, http.MethodGet, "/api/location-tags", nil, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestSetLocationTags(t *testing.T) {
	fake := &gemini.Fake{TagCaptureResult: gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "ZKEI"}}
	h, cookies := newTestServerWithGemini(t, fake)

	captureResp := doCaptureUpload(t, h, cookies, []byte("photo"))
	var captured captureResponse
	json.NewDecoder(captureResp.Body).Decode(&captured)

	fake.ReconciliationResult = gemini.ReconciliationResult{HasLocationCode: true, LocationCode: "@XYZ", AssetTags: []string{"ZKEI"}}
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
	if len(loc.Tags) != 0 {
		t.Fatalf("new location tags = %+v, want none", loc.Tags)
	}

	createResp := doJSON(t, h, http.MethodPost, "/api/location-tags", tagRequest{Name: "warehouse", Color: "#a6e22e"}, cookies)
	var createBody struct {
		Tag tagResponse `json:"tag"`
	}
	json.NewDecoder(createResp.Body).Decode(&createBody)

	locIDStr := strconv.FormatInt(loc.ID, 10)
	setResp := doJSON(t, h, http.MethodPut, "/api/locations/"+locIDStr+"/tags", setLocationTagsRequest{TagIDs: []int64{createBody.Tag.ID}}, cookies)
	if setResp.Code != http.StatusOK {
		t.Fatalf("set location tags status = %d, body = %s", setResp.Code, setResp.Body.String())
	}
	var setBody struct {
		Location locationResponse `json:"location"`
	}
	json.NewDecoder(setResp.Body).Decode(&setBody)
	if len(setBody.Location.Tags) != 1 || setBody.Location.Tags[0].ID != createBody.Tag.ID {
		t.Fatalf("location tags after set = %+v, want [warehouse]", setBody.Location.Tags)
	}

	// the list endpoint reflects it too
	listResp := doJSON(t, h, http.MethodGet, "/api/locations", nil, cookies)
	json.NewDecoder(listResp.Body).Decode(&locBody)
	if len(locBody.Locations[0].Tags) != 1 || locBody.Locations[0].Tags[0].ID != createBody.Tag.ID {
		t.Fatalf("locations list tags = %+v, want [warehouse]", locBody.Locations[0].Tags)
	}

	// activity log picked up the change
	activityResp := doJSON(t, h, http.MethodGet, "/api/locations/"+locIDStr+"/activity", nil, cookies)
	var activityBody struct {
		Activity []activityResponse `json:"activity"`
	}
	json.NewDecoder(activityResp.Body).Decode(&activityBody)
	found := false
	for _, a := range activityBody.Activity {
		if a.Action == "location_tags_updated" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a location_tags_updated activity entry, got %+v", activityBody.Activity)
	}

	// clearing tags
	clearResp := doJSON(t, h, http.MethodPut, "/api/locations/"+locIDStr+"/tags", setLocationTagsRequest{TagIDs: nil}, cookies)
	var clearBody struct {
		Location locationResponse `json:"location"`
	}
	json.NewDecoder(clearResp.Body).Decode(&clearBody)
	if len(clearBody.Location.Tags) != 0 {
		t.Fatalf("location tags after clear = %+v, want none", clearBody.Location.Tags)
	}
}

func TestSetLocationTagsRequiresAuth(t *testing.T) {
	h, _ := newTestServerWithGemini(t, nil)
	w := doJSON(t, h, http.MethodPut, "/api/locations/1/tags", setLocationTagsRequest{TagIDs: nil}, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/pborges/aiinventory/internal/gemini"
)

func TestLocationViewFlow(t *testing.T) {
	fake := &gemini.Fake{}
	h, cookies, _ := newTestServerWithGemini(t, fake)

	// capture two items
	fake.TagCaptureResult = gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "ZKEI"}
	zkei := doCaptureUpload(t, h, cookies, []byte("zkei"))
	var zkeiResp captureResponse
	json.NewDecoder(zkei.Body).Decode(&zkeiResp)

	fake.TagCaptureResult = gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "GKEI"}
	gkei := doCaptureUpload(t, h, cookies, []byte("gkei"))
	var gkeiResp captureResponse
	json.NewDecoder(gkei.Body).Decode(&gkeiResp)

	// reconcile ZKEI into @XYZ (creates the location)
	fake.ReconciliationResult = gemini.ReconciliationResult{HasLocationTag: true, LocationTag: "@XYZ", AssetTags: []string{"ZKEI"}}
	applyReconcile(t, h, cookies, "@XYZ", []string{"ZKEI"})

	// list locations
	locResp := doJSON(t, h, http.MethodGet, "/api/locations", nil, cookies)
	if locResp.Code != http.StatusOK {
		t.Fatalf("list locations status = %d, body = %s", locResp.Code, locResp.Body.String())
	}
	var locations struct {
		Locations []locationResponse `json:"locations"`
	}
	json.NewDecoder(locResp.Body).Decode(&locations)
	if len(locations.Locations) != 1 || locations.Locations[0].LocationTag != "@XYZ" {
		t.Fatalf("locations = %+v, want one @XYZ", locations.Locations)
	}
	xyzID := locations.Locations[0].ID

	// items at @XYZ should be just ZKEI, with its full image set
	itemsResp := doJSON(t, h, http.MethodGet, "/api/locations/"+strconv.FormatInt(xyzID, 10)+"/items", nil, cookies)
	var itemsBody struct {
		Items []locationItemResponse `json:"items"`
	}
	json.NewDecoder(itemsResp.Body).Decode(&itemsBody)
	if len(itemsBody.Items) != 1 || itemsBody.Items[0].AssetTag != "ZKEI" {
		t.Fatalf("location items = %+v, want [ZKEI]", itemsBody.Items)
	}
	if len(itemsBody.Items[0].Images) != 1 {
		t.Fatalf("ZKEI images = %+v, want 1", itemsBody.Items[0].Images)
	}

	// drag GKEI onto @XYZ via the move endpoint
	moveResp := doJSON(t, h, http.MethodPost, "/api/locations/"+strconv.FormatInt(xyzID, 10)+"/move-item", moveItemRequest{ItemID: gkeiResp.ItemID}, cookies)
	if moveResp.Code != http.StatusOK {
		t.Fatalf("move status = %d, body = %s", moveResp.Code, moveResp.Body.String())
	}

	itemsResp2 := doJSON(t, h, http.MethodGet, "/api/locations/"+strconv.FormatInt(xyzID, 10)+"/items", nil, cookies)
	json.NewDecoder(itemsResp2.Body).Decode(&itemsBody)
	if len(itemsBody.Items) != 2 {
		t.Fatalf("after move, location items = %+v, want 2", itemsBody.Items)
	}

	// activity log for the location includes the reconciliation + the move
	activityResp := doJSON(t, h, http.MethodGet, "/api/locations/"+strconv.FormatInt(xyzID, 10)+"/activity", nil, cookies)
	var activityBody struct {
		Activity []activityResponse `json:"activity"`
	}
	json.NewDecoder(activityResp.Body).Decode(&activityBody)
	if len(activityBody.Activity) < 2 {
		t.Fatalf("location activity = %+v, want at least 2 entries", activityBody.Activity)
	}
}

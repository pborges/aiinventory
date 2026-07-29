package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/pborges/aiinventory/internal/gemini"
)

func waitForDuplicateRunDone(t *testing.T, h http.Handler, cookies []*http.Cookie) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		w := doJSON(t, h, http.MethodGet, "/api/duplicates/status", nil, cookies)
		var status duplicateStatusResponse
		json.NewDecoder(w.Body).Decode(&status)
		if !status.Running {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("duplicate run did not finish within the timeout")
}

func TestDuplicateFinderFullFlow(t *testing.T) {
	fake := &gemini.Fake{TagCaptureResult: gemini.TagCaptureResult{HasAssetTag: true}}
	h, cookies, _ := newTestServerWithGemini(t, fake)

	fake.TagCaptureResult.AssetTag = "ZKEI"
	zkei := doCaptureUpload(t, h, cookies, []byte("zkei"))
	var zkeiResp captureResponse
	json.NewDecoder(zkei.Body).Decode(&zkeiResp)

	fake.TagCaptureResult.AssetTag = "GKEI"
	doCaptureUpload(t, h, cookies, []byte("gkei"))

	// status before any run
	statusResp := doJSON(t, h, http.MethodGet, "/api/duplicates/status", nil, cookies)
	var status duplicateStatusResponse
	json.NewDecoder(statusResp.Body).Decode(&status)
	if status.Running {
		t.Fatal("should not be running before any run has started")
	}

	fake.DuplicateDetectionResult = gemini.DuplicateDetectionResult{
		Groups: []gemini.DuplicateGroupCandidate{{AssetTags: []string{"ZKEI", "GKEI"}, Reasoning: "same item"}},
	}
	runResp := doJSON(t, h, http.MethodPost, "/api/duplicates/run", nil, cookies)
	if runResp.Code != http.StatusAccepted {
		t.Fatalf("run status = %d, body = %s", runResp.Code, runResp.Body.String())
	}
	// Concurrent-start rejection (409) is covered at the unit level by
	// TestRunnerRejectsConcurrentStart against the Runner directly, which
	// doesn't depend on goroutine scheduling timing the way an HTTP-level
	// race would. Asserting it here too would be flaky: against the fake
	// Gemini client, the first run's goroutine may complete before a second
	// request is even issued.

	waitForDuplicateRunDone(t, h, cookies)

	groupsResp := doJSON(t, h, http.MethodGet, "/api/duplicates/groups", nil, cookies)
	var groupsBody struct {
		Groups []duplicateGroupResponse `json:"groups"`
	}
	json.NewDecoder(groupsResp.Body).Decode(&groupsBody)
	if len(groupsBody.Groups) != 1 || len(groupsBody.Groups[0].Items) != 2 {
		t.Fatalf("groups = %+v, want one group with 2 tags", groupsBody.Groups)
	}

	// dismiss it
	dismissResp := doJSON(t, h, http.MethodPost, "/api/duplicates/groups/"+strconv.FormatInt(groupsBody.Groups[0].ID, 10)+"/dismiss", nil, cookies)
	if dismissResp.Code != http.StatusNoContent {
		t.Fatalf("dismiss status = %d, body = %s", dismissResp.Code, dismissResp.Body.String())
	}

	after := doJSON(t, h, http.MethodGet, "/api/duplicates/groups", nil, cookies)
	json.NewDecoder(after.Body).Decode(&groupsBody)
	if len(groupsBody.Groups) != 0 {
		t.Fatalf("groups after dismiss = %+v, want none", groupsBody.Groups)
	}
}

func TestDuplicateFinderMergeViaAPI(t *testing.T) {
	fake := &gemini.Fake{}
	h, cookies, _ := newTestServerWithGemini(t, fake)

	fake.TagCaptureResult = gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "ZKEI"}
	zkei := doCaptureUpload(t, h, cookies, []byte("zkei"))
	var zkeiResp captureResponse
	json.NewDecoder(zkei.Body).Decode(&zkeiResp)

	fake.TagCaptureResult = gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "GKEI"}
	doCaptureUpload(t, h, cookies, []byte("gkei"))

	fake.DuplicateDetectionResult = gemini.DuplicateDetectionResult{
		Groups: []gemini.DuplicateGroupCandidate{{AssetTags: []string{"ZKEI", "GKEI"}, Reasoning: "same item"}},
	}
	doJSON(t, h, http.MethodPost, "/api/duplicates/run", nil, cookies)
	waitForDuplicateRunDone(t, h, cookies)

	groupsResp := doJSON(t, h, http.MethodGet, "/api/duplicates/groups", nil, cookies)
	var groupsBody struct {
		Groups []duplicateGroupResponse `json:"groups"`
	}
	json.NewDecoder(groupsResp.Body).Decode(&groupsBody)
	if len(groupsBody.Groups) != 1 {
		t.Fatalf("groups = %+v, want 1", groupsBody.Groups)
	}

	mergeResp := doJSON(t, h, http.MethodPost, "/api/duplicates/groups/"+strconv.FormatInt(groupsBody.Groups[0].ID, 10)+"/merge",
		mergeDuplicateGroupRequest{SurvivorItemID: zkeiResp.ItemID}, cookies)
	if mergeResp.Code != http.StatusNoContent {
		t.Fatalf("merge status = %d, body = %s", mergeResp.Code, mergeResp.Body.String())
	}

	// GKEI's asset tag should now be free for reuse
	fake.TagCaptureResult = gemini.TagCaptureResult{HasAssetTag: true, AssetTag: "GKEI"}
	reused := doCaptureUpload(t, h, cookies, []byte("gkei-again"))
	var reusedResp captureResponse
	json.NewDecoder(reused.Body).Decode(&reusedResp)
	if !reusedResp.ItemWasNew {
		t.Fatalf("GKEI should be free for reuse after merge, got ItemWasNew=false")
	}
}

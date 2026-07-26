package gemini

import "testing"

func TestParseJSONResponseTagCapture(t *testing.T) {
	var out TagCaptureResult
	err := parseJSONResponse(`{"has_asset_tag":true,"asset_tag":"ZKEI","item_guess":"cordless drill","description":"S/N 12345"}`, &out)
	if err != nil {
		t.Fatalf("parseJSONResponse: %v", err)
	}
	want := TagCaptureResult{HasAssetTag: true, AssetTag: "ZKEI", ItemGuess: "cordless drill", Description: "S/N 12345"}
	if out != want {
		t.Errorf("got %+v, want %+v", out, want)
	}
}

func TestParseJSONResponseReconciliation(t *testing.T) {
	var out ReconciliationResult
	err := parseJSONResponse(`{"has_location_code":true,"location_code":"@XYZ","asset_tags":["ZKEI","GKEI"]}`, &out)
	if err != nil {
		t.Fatalf("parseJSONResponse: %v", err)
	}
	if !out.HasLocationCode || out.LocationCode != "@XYZ" || len(out.AssetTags) != 2 {
		t.Errorf("got %+v", out)
	}
}

func TestParseJSONResponseDuplicateGroups(t *testing.T) {
	var out DuplicateDetectionResult
	err := parseJSONResponse(`{"groups":[{"asset_tags":["ZKEI","GKEI"],"reasoning":"same serial number"}]}`, &out)
	if err != nil {
		t.Fatalf("parseJSONResponse: %v", err)
	}
	if len(out.Groups) != 1 || out.Groups[0].Reasoning != "same serial number" {
		t.Errorf("got %+v", out)
	}
}

func TestParseJSONResponseMalformed(t *testing.T) {
	var out TagCaptureResult
	err := parseJSONResponse(`not json`, &out)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestParseJSONResponseEmptyGroups(t *testing.T) {
	var out DuplicateDetectionResult
	err := parseJSONResponse(`{"groups":[]}`, &out)
	if err != nil {
		t.Fatalf("parseJSONResponse: %v", err)
	}
	if len(out.Groups) != 0 {
		t.Errorf("got %+v, want empty groups", out)
	}
}

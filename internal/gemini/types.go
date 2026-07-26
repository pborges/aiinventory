// Package gemini wraps Gemini vision requests for aiinventory's four AI
// touchpoints. It knows nothing about settings/DB storage — callers resolve
// the model and prompt (default or overridden) and pass them in explicitly.
package gemini

type RequestType string

const (
	TagCapture              RequestType = "tag_capture"
	LocationReconciliation  RequestType = "location_reconciliation"
	DescriptionRegeneration RequestType = "description_regeneration"
	DuplicateDetection      RequestType = "duplicate_detection"
)

// TagCaptureResult is Gemini's read of a single captured photo during the
// asset-tagging flow (README flow #1).
type TagCaptureResult struct {
	HasAssetTag bool   `json:"has_asset_tag"`
	AssetTag    string `json:"asset_tag"`
	ItemGuess   string `json:"item_guess"`
	Description string `json:"description"`
}

// ReconciliationResult is Gemini's read of a single captured photo during
// the location-reconciliation flow (README flow #2).
type ReconciliationResult struct {
	HasLocationCode bool     `json:"has_location_code"`
	LocationCode    string   `json:"location_code"`
	AssetTags       []string `json:"asset_tags"`
}

// DescriptionResult is a consolidated item description synthesized from
// per-image notes (README's "Regenerate description" bulk action).
type DescriptionResult struct {
	Description string `json:"description"`
}

// AssetTagDescription is one item's identity + description, as fed into the
// duplicate finder.
type AssetTagDescription struct {
	AssetTag    string
	Description string
}

// DuplicateGroupCandidate is one candidate group of possibly-duplicate items
// flagged by the duplicate finder (README flow #5).
type DuplicateGroupCandidate struct {
	AssetTags []string `json:"asset_tags"`
	Reasoning string   `json:"reasoning"`
}

type DuplicateDetectionResult struct {
	Groups []DuplicateGroupCandidate `json:"groups"`
}

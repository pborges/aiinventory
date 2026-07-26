package gemini

import (
	"embed"
	"fmt"
)

//go:embed prompts/*.txt
var promptsFS embed.FS

var promptFiles = map[RequestType]string{
	TagCapture:              "prompts/tag_capture.txt",
	LocationReconciliation:  "prompts/location_reconciliation.txt",
	DescriptionRegeneration: "prompts/description_regeneration.txt",
	DuplicateDetection:      "prompts/duplicate_detection.txt",
}

// DefaultPrompt returns the built-in default prompt for t. This is the exact
// text the Settings page's "view default" shadowbox displays, and what's used
// whenever no override is configured — a single source of truth for both.
func DefaultPrompt(t RequestType) string {
	path, ok := promptFiles[t]
	if !ok {
		panic(fmt.Sprintf("gemini: no default prompt registered for request type %q", t))
	}
	data, err := promptsFS.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("gemini: embedded default prompt %q missing: %v", path, err))
	}
	return string(data)
}

package gemini

import "testing"

func TestDefaultPromptCoversAllRequestTypes(t *testing.T) {
	types := []RequestType{TagCapture, LocationReconciliation, DescriptionRegeneration, DuplicateDetection}
	for _, rt := range types {
		p := DefaultPrompt(rt)
		if p == "" {
			t.Errorf("DefaultPrompt(%q) is empty", rt)
		}
	}
}

func TestDefaultPromptPanicsOnUnknownType(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for unknown request type")
		}
	}()
	DefaultPrompt(RequestType("nonsense"))
}

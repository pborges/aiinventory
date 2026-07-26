package gemini

import "context"

// Fake is a hand-rolled, in-memory Client for tests — no mocking library,
// no network. Set the *Result/*Err fields (or the *Func hooks for
// per-call logic) before use; unset Func hooks fall back to returning the
// canned Result/Err.
type Fake struct {
	TagCaptureResult TagCaptureResult
	TagCaptureErr    error
	TagCaptureFunc   func(image []byte, mimeType string) (TagCaptureResult, error)

	ReconciliationResult ReconciliationResult
	ReconciliationErr    error
	ReconciliationFunc   func(image []byte, mimeType string) (ReconciliationResult, error)

	DescriptionResult DescriptionResult
	DescriptionErr    error

	DuplicateDetectionResult DuplicateDetectionResult
	DuplicateDetectionErr    error
}

var _ Client = (*Fake)(nil)

func (f *Fake) AnalyzeTagCapture(_ context.Context, _, _ string, image []byte, mimeType string) (TagCaptureResult, error) {
	if f.TagCaptureFunc != nil {
		return f.TagCaptureFunc(image, mimeType)
	}
	return f.TagCaptureResult, f.TagCaptureErr
}

func (f *Fake) AnalyzeReconciliation(_ context.Context, _, _ string, image []byte, mimeType string) (ReconciliationResult, error) {
	if f.ReconciliationFunc != nil {
		return f.ReconciliationFunc(image, mimeType)
	}
	return f.ReconciliationResult, f.ReconciliationErr
}

func (f *Fake) RegenerateDescription(_ context.Context, _, _ string, _ string, _ []string) (DescriptionResult, error) {
	return f.DescriptionResult, f.DescriptionErr
}

func (f *Fake) DetectDuplicates(_ context.Context, _, _ string, _ []AssetTagDescription) (DuplicateDetectionResult, error) {
	return f.DuplicateDetectionResult, f.DuplicateDetectionErr
}

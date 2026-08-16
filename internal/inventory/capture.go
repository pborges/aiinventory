// Package inventory holds the business logic that sits between the HTTP
// handlers and the store: capture orchestration, reconciliation diffing,
// description consolidation, and duplicate merging. It depends on narrow
// interfaces it defines itself, satisfied by *store.Store.
package inventory

import (
	"context"
	"errors"

	"github.com/pborges/aiinventory/internal/domain"
)

// ErrNoAssetTag is returned when a captured frame didn't contain a
// recognizable asset tag — the README's "capture is rejected" case.
var ErrNoAssetTag = errors.New("no asset tag found in frame")

type CaptureStore interface {
	ApplyCapture(ctx context.Context, userID int64, captureID, assetTag string, data []byte, contentType, description string, setItemDescription bool) (domain.Item, bool, error)
}

type CaptureResult struct {
	Item       domain.Item
	ItemWasNew bool
}

// Capture ingests one photo already analyzed by Gemini for the tag-capture
// flow (README flow #1). hasAssetTag/assetTag/itemGuess/imageDescription
// come from gemini.TagCaptureResult; the caller is responsible for calling
// Gemini first and translating hasAssetTag=false into not calling this at
// all (or handling ErrNoAssetTag, which this also guards against directly).
// The per-image note (imageDescription) is always saved onto the photo;
// setItemDescription additionally promotes that same text onto the item's
// consolidated description, skipping the separate RegenerateDescription
// call for the common case of a single clean read.
func Capture(ctx context.Context, s CaptureStore, userID int64, captureID string, hasAssetTag bool, assetTag string, imageData []byte, contentType, imageDescription string, setItemDescription bool) (CaptureResult, error) {
	if !hasAssetTag || assetTag == "" {
		return CaptureResult{}, ErrNoAssetTag
	}
	item, itemWasNew, err := s.ApplyCapture(ctx, userID, captureID, assetTag, imageData, contentType, imageDescription, setItemDescription)
	if err != nil {
		return CaptureResult{}, err
	}
	return CaptureResult{Item: item, ItemWasNew: itemWasNew}, nil
}

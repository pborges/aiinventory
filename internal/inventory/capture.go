// Package inventory holds the business logic that sits between the HTTP
// handlers and the store: capture orchestration, reconciliation diffing,
// description consolidation, and duplicate merging. It depends on narrow
// interfaces it defines itself, satisfied by *store.Store.
package inventory

import (
	"context"
	"errors"
	"fmt"

	"github.com/pborges/aiinventory/internal/domain"
	"github.com/pborges/aiinventory/internal/store"
)

// ErrNoAssetTag is returned when a captured frame didn't contain a
// recognizable asset tag — the README's "capture is rejected" case.
var ErrNoAssetTag = errors.New("no asset tag found in frame")

type CaptureStore interface {
	GetItemByAssetTag(ctx context.Context, tag string) (domain.Item, error)
	CreateItem(ctx context.Context, tag string) (domain.Item, error)
	AddImage(ctx context.Context, itemID int64, data []byte, contentType, description string, createdBy int64) (domain.Image, error)
	LogActivity(ctx context.Context, userID int64, action domain.ActivityAction, itemID, locationID *int64, detail string) error
	RegisterAssetTag(ctx context.Context, tag string) error
}

type CaptureResult struct {
	Item       domain.Item
	ItemWasNew bool
	Image      domain.Image
}

// Capture ingests one photo already analyzed by Gemini for the tag-capture
// flow (README flow #1). hasAssetTag/assetTag/itemGuess/imageDescription
// come from gemini.TagCaptureResult; the caller is responsible for calling
// Gemini first and translating hasAssetTag=false into not calling this at
// all (or handling ErrNoAssetTag, which this also guards against directly).
func Capture(ctx context.Context, s CaptureStore, userID int64, hasAssetTag bool, assetTag string, imageData []byte, contentType, imageDescription string) (CaptureResult, error) {
	if !hasAssetTag || assetTag == "" {
		return CaptureResult{}, ErrNoAssetTag
	}

	item, err := s.GetItemByAssetTag(ctx, assetTag)
	itemWasNew := false
	switch {
	case errors.Is(err, store.ErrNotFound):
		item, err = s.CreateItem(ctx, assetTag)
		if err != nil {
			return CaptureResult{}, fmt.Errorf("create item: %w", err)
		}
		itemWasNew = true
	case err != nil:
		return CaptureResult{}, fmt.Errorf("look up item: %w", err)
	}

	// Self-heal the tag registry with whatever tag was actually accepted —
	// an exact registry match, a confident auto-correction, or a manual
	// operator override — so it's an exact match on every future scan
	// regardless of how it got here.
	if err := s.RegisterAssetTag(ctx, assetTag); err != nil {
		return CaptureResult{}, fmt.Errorf("register tag: %w", err)
	}

	img, err := s.AddImage(ctx, item.ID, imageData, contentType, imageDescription, userID)
	if err != nil {
		return CaptureResult{}, fmt.Errorf("add image: %w", err)
	}

	action := domain.ActivityImageAdded
	if itemWasNew {
		action = domain.ActivityItemCreated
	}
	if err := s.LogActivity(ctx, userID, action, &item.ID, nil, ""); err != nil {
		return CaptureResult{}, fmt.Errorf("log activity: %w", err)
	}

	return CaptureResult{Item: item, ItemWasNew: itemWasNew, Image: img}, nil
}

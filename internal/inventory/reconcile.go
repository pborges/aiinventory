package inventory

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/pborges/aiinventory/internal/domain"
	"github.com/pborges/aiinventory/internal/store"
)

type ReconcileReadStore interface {
	GetLocationByCode(ctx context.Context, code string) (domain.Location, error)
	GetLocationByID(ctx context.Context, id int64) (domain.Location, error)
	ListItemsByLocation(ctx context.Context, locationID int64) ([]domain.Item, error)
	GetItemByAssetTag(ctx context.Context, tag string) (domain.Item, error)
}

// ComputeReconciliation is the pure, read-only half of README flow #2: given
// a location code and the set of asset tags Gemini read from the same
// frame, it classifies every change against the location's current
// contents (new / added / moved-from-elsewhere / removed) without writing
// anything. This is what the frontend shows for approval before the user
// confirms — see internal/store.ApplyReconciliation for the write side.
func ComputeReconciliation(ctx context.Context, s ReconcileReadStore, locationCode string, frameTags []string) (domain.ReconcileDiff, error) {
	diff := domain.ReconcileDiff{LocationCode: locationCode}

	var currentItems []domain.Item
	loc, err := s.GetLocationByCode(ctx, locationCode)
	switch {
	case errors.Is(err, store.ErrNotFound):
		// brand-new location code: nothing currently linked, it'll be created on apply
	case err != nil:
		return domain.ReconcileDiff{}, fmt.Errorf("look up location: %w", err)
	default:
		currentItems, err = s.ListItemsByLocation(ctx, loc.ID)
		if err != nil {
			return domain.ReconcileDiff{}, fmt.Errorf("list items by location: %w", err)
		}
	}

	currentSet := make(map[string]bool, len(currentItems))
	for _, it := range currentItems {
		currentSet[it.AssetTag] = true
	}

	frameSet := make(map[string]bool, len(frameTags))
	sortedFrameTags := make([]string, 0, len(frameTags))
	for _, t := range frameTags {
		if t == "" || frameSet[t] {
			continue
		}
		frameSet[t] = true
		sortedFrameTags = append(sortedFrameTags, t)
	}
	sort.Strings(sortedFrameTags)

	for _, tag := range sortedFrameTags {
		if currentSet[tag] {
			continue // already linked here, no change
		}
		item, err := s.GetItemByAssetTag(ctx, tag)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				// frame shows a tag that's never been captured as an item anywhere;
				// treat it as a new item to be created and linked to this location
				diff.New = append(diff.New, tag)
				continue
			}
			return domain.ReconcileDiff{}, fmt.Errorf("look up item %s: %w", tag, err)
		}
		if item.LocationID == nil {
			diff.Added = append(diff.Added, tag)
			continue
		}
		fromCode := ""
		if fromLoc, err := s.GetLocationByID(ctx, *item.LocationID); err == nil {
			fromCode = fromLoc.Code
		}
		diff.Moved = append(diff.Moved, domain.MovedItem{AssetTag: tag, FromLocation: fromCode})
	}

	removed := make([]string, 0)
	for tag := range currentSet {
		if !frameSet[tag] {
			removed = append(removed, tag)
		}
	}
	sort.Strings(removed)
	diff.Removed = removed

	return diff, nil
}

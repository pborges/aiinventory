package inventory

import (
	"context"
	"fmt"

	"github.com/pborges/aiinventory/internal/domain"
)

type MoveItemStore interface {
	GetItemByID(ctx context.Context, id int64) (domain.Item, error)
	GetLocationByID(ctx context.Context, id int64) (domain.Location, error)
	SetItemLocation(ctx context.Context, itemID int64, locationID *int64) error
	LogActivity(ctx context.Context, userID int64, action domain.ActivityAction, itemID, locationID *int64, detail string) error
}

// MoveItemToLocation implements the location view's drag-and-drop
// relocation (README flow #4): a manual, desktop alternative to the camera
// reconciliation flow, writing the same kind of item_moved activity entry.
func MoveItemToLocation(ctx context.Context, s MoveItemStore, userID, itemID, locationID int64) (domain.Item, error) {
	item, err := s.GetItemByID(ctx, itemID)
	if err != nil {
		return domain.Item{}, fmt.Errorf("look up item: %w", err)
	}
	newLoc, err := s.GetLocationByID(ctx, locationID)
	if err != nil {
		return domain.Item{}, fmt.Errorf("look up location: %w", err)
	}

	detail := fmt.Sprintf("moved to %s", newLoc.LocationTag)
	if item.LocationID != nil {
		if oldLoc, err := s.GetLocationByID(ctx, *item.LocationID); err == nil {
			detail = fmt.Sprintf("moved to %s (was %s)", newLoc.LocationTag, oldLoc.LocationTag)
		}
	}

	if err := s.SetItemLocation(ctx, itemID, &locationID); err != nil {
		return domain.Item{}, fmt.Errorf("set item location: %w", err)
	}
	if err := s.LogActivity(ctx, userID, domain.ActivityItemMoved, &itemID, &locationID, detail); err != nil {
		return domain.Item{}, fmt.Errorf("log activity: %w", err)
	}

	return s.GetItemByID(ctx, itemID)
}

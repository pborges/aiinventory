package inventory

import (
	"context"
	"fmt"

	"github.com/pborges/aiinventory/internal/domain"
)

type DeleteStore interface {
	GetItemByID(ctx context.Context, id int64) (domain.Item, error)
	DeleteItem(ctx context.Context, id int64) error
	LogActivity(ctx context.Context, userID int64, action domain.ActivityAction, itemID, locationID *int64, detail string) error
}

// DeleteItem implements the Search view's bulk "Delete" action: hard-delete
// (freeing the asset tag for reuse, per README), but log the activity entry
// *before* deleting so the tag stays in the record even after the item_id
// FK gets nulled out by the delete's ON DELETE SET NULL.
func DeleteItem(ctx context.Context, s DeleteStore, userID, itemID int64) error {
	item, err := s.GetItemByID(ctx, itemID)
	if err != nil {
		return fmt.Errorf("look up item: %w", err)
	}
	if err := s.LogActivity(ctx, userID, domain.ActivityItemDeleted, &itemID, nil, item.AssetTag); err != nil {
		return fmt.Errorf("log activity: %w", err)
	}
	if err := s.DeleteItem(ctx, itemID); err != nil {
		return fmt.Errorf("delete item: %w", err)
	}
	return nil
}

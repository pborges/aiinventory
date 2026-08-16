package inventory

import (
	"context"
)

type DeleteStore interface {
	DeleteItemWithActivity(ctx context.Context, userID, itemID int64) error
}

// DeleteItem implements the Search view's bulk "Delete" action: hard-delete
// (freeing the asset tag for reuse, per README), but log the activity entry
// *before* deleting so the tag stays in the record even after the item_id
// FK gets nulled out by the delete's ON DELETE SET NULL.
func DeleteItem(ctx context.Context, s DeleteStore, userID, itemID int64) error {
	return s.DeleteItemWithActivity(ctx, userID, itemID)
}

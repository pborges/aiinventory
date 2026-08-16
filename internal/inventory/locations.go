package inventory

import (
	"context"

	"github.com/pborges/aiinventory/internal/domain"
)

type MoveItemStore interface {
	MoveItemToLocationWithActivity(ctx context.Context, userID, itemID, locationID int64) (domain.Item, error)
}

// MoveItemToLocation implements the location view's drag-and-drop
// relocation (README flow #4): a manual, desktop alternative to the camera
// reconciliation flow, writing the same kind of item_moved activity entry.
func MoveItemToLocation(ctx context.Context, s MoveItemStore, userID, itemID, locationID int64) (domain.Item, error) {
	return s.MoveItemToLocationWithActivity(ctx, userID, itemID, locationID)
}

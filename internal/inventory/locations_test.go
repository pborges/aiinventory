package inventory

import (
	"context"
	"testing"

	"github.com/pborges/aiinventory/internal/domain"
	"github.com/pborges/aiinventory/internal/store"
)

type fakeMoveItemStore struct {
	items     map[int64]domain.Item
	locations map[int64]domain.Location
	activity  []loggedActivity
}

func (f *fakeMoveItemStore) GetItemByID(_ context.Context, id int64) (domain.Item, error) {
	it, ok := f.items[id]
	if !ok {
		return domain.Item{}, store.ErrNotFound
	}
	return it, nil
}

func (f *fakeMoveItemStore) GetLocationByID(_ context.Context, id int64) (domain.Location, error) {
	loc, ok := f.locations[id]
	if !ok {
		return domain.Location{}, store.ErrNotFound
	}
	return loc, nil
}

func (f *fakeMoveItemStore) SetItemLocation(_ context.Context, itemID int64, locationID *int64) error {
	it := f.items[itemID]
	it.LocationID = locationID
	f.items[itemID] = it
	return nil
}

func (f *fakeMoveItemStore) LogActivity(_ context.Context, userID int64, action domain.ActivityAction, itemID, locationID *int64, detail string) error {
	f.activity = append(f.activity, loggedActivity{userID: userID, action: action, itemID: itemID})
	return nil
}

func TestMoveItemToLocation(t *testing.T) {
	oldLocID, newLocID := int64(1), int64(2)
	f := &fakeMoveItemStore{
		items: map[int64]domain.Item{
			10: {ID: 10, AssetTag: "ZKEI", LocationID: &oldLocID},
		},
		locations: map[int64]domain.Location{
			1: {ID: 1, Code: "@QRS"},
			2: {ID: 2, Code: "@XYZ"},
		},
	}

	updated, err := MoveItemToLocation(context.Background(), f, 42, 10, newLocID)
	if err != nil {
		t.Fatalf("MoveItemToLocation: %v", err)
	}
	if updated.LocationID == nil || *updated.LocationID != newLocID {
		t.Fatalf("LocationID = %v, want %d", updated.LocationID, newLocID)
	}
	if len(f.activity) != 1 || f.activity[0].action != domain.ActivityItemMoved || f.activity[0].userID != 42 {
		t.Fatalf("activity = %+v", f.activity)
	}
}

func TestMoveItemToLocationFromUnlinked(t *testing.T) {
	f := &fakeMoveItemStore{
		items:     map[int64]domain.Item{10: {ID: 10, AssetTag: "ZKEI"}},
		locations: map[int64]domain.Location{2: {ID: 2, Code: "@XYZ"}},
	}

	updated, err := MoveItemToLocation(context.Background(), f, 42, 10, 2)
	if err != nil {
		t.Fatalf("MoveItemToLocation: %v", err)
	}
	if updated.LocationID == nil || *updated.LocationID != 2 {
		t.Fatalf("LocationID = %v, want 2", updated.LocationID)
	}
}

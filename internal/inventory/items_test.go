package inventory

import (
	"context"
	"testing"

	"github.com/pborges/aiinventory/internal/domain"
	"github.com/pborges/aiinventory/internal/store"
)

type fakeDeleteStore struct {
	items      map[int64]domain.Item
	deletedIDs []int64
	activity   []loggedActivity
}

func (f *fakeDeleteStore) GetItemByID(_ context.Context, id int64) (domain.Item, error) {
	it, ok := f.items[id]
	if !ok {
		return domain.Item{}, store.ErrNotFound
	}
	return it, nil
}

func (f *fakeDeleteStore) DeleteItem(_ context.Context, id int64) error {
	if _, ok := f.items[id]; !ok {
		return store.ErrNotFound
	}
	delete(f.items, id)
	f.deletedIDs = append(f.deletedIDs, id)
	return nil
}

func (f *fakeDeleteStore) LogActivity(_ context.Context, userID int64, action domain.ActivityAction, itemID, _ *int64, _ string) error {
	f.activity = append(f.activity, loggedActivity{userID: userID, action: action, itemID: itemID})
	return nil
}

func TestDeleteItemLogsBeforeDeleting(t *testing.T) {
	f := &fakeDeleteStore{items: map[int64]domain.Item{1: {ID: 1, AssetTag: "ZKEI"}}}

	if err := DeleteItem(context.Background(), f, 42, 1); err != nil {
		t.Fatalf("DeleteItem: %v", err)
	}

	if len(f.deletedIDs) != 1 || f.deletedIDs[0] != 1 {
		t.Fatalf("deletedIDs = %v, want [1]", f.deletedIDs)
	}
	if len(f.activity) != 1 || f.activity[0].action != domain.ActivityItemDeleted || f.activity[0].userID != 42 {
		t.Fatalf("activity = %+v", f.activity)
	}
	if _, ok := f.items[1]; ok {
		t.Fatal("item still present after DeleteItem")
	}
}

func TestDeleteItemMissing(t *testing.T) {
	f := &fakeDeleteStore{items: map[int64]domain.Item{}}
	if err := DeleteItem(context.Background(), f, 42, 999); err == nil {
		t.Fatal("expected error deleting a nonexistent item")
	}
	if len(f.activity) != 0 {
		t.Fatalf("no activity should be logged when the item lookup fails: %+v", f.activity)
	}
}

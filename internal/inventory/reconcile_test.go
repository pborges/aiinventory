package inventory

import (
	"context"
	"reflect"
	"testing"

	"github.com/pborges/aiinventory/internal/domain"
	"github.com/pborges/aiinventory/internal/store"
)

type fakeReconcileStore struct {
	locationsByCode map[string]domain.Location
	locationsByID   map[int64]domain.Location
	itemsByTag      map[string]domain.Item
	itemsByLocation map[int64][]domain.Item
}

func newFakeReconcileStore() *fakeReconcileStore {
	return &fakeReconcileStore{
		locationsByCode: map[string]domain.Location{},
		locationsByID:   map[int64]domain.Location{},
		itemsByTag:      map[string]domain.Item{},
		itemsByLocation: map[int64][]domain.Item{},
	}
}

func (f *fakeReconcileStore) addLocation(id int64, code string) domain.Location {
	loc := domain.Location{ID: id, Code: code}
	f.locationsByCode[code] = loc
	f.locationsByID[id] = loc
	return loc
}

func (f *fakeReconcileStore) addItem(tag string, locationID *int64) domain.Item {
	it := domain.Item{ID: int64(len(f.itemsByTag) + 1), AssetTag: tag, LocationID: locationID}
	f.itemsByTag[tag] = it
	if locationID != nil {
		f.itemsByLocation[*locationID] = append(f.itemsByLocation[*locationID], it)
	}
	return it
}

func (f *fakeReconcileStore) GetLocationByCode(_ context.Context, code string) (domain.Location, error) {
	loc, ok := f.locationsByCode[code]
	if !ok {
		return domain.Location{}, store.ErrNotFound
	}
	return loc, nil
}

func (f *fakeReconcileStore) GetLocationByID(_ context.Context, id int64) (domain.Location, error) {
	loc, ok := f.locationsByID[id]
	if !ok {
		return domain.Location{}, store.ErrNotFound
	}
	return loc, nil
}

func (f *fakeReconcileStore) ListItemsByLocation(_ context.Context, locationID int64) ([]domain.Item, error) {
	return f.itemsByLocation[locationID], nil
}

func (f *fakeReconcileStore) GetItemByAssetTag(_ context.Context, tag string) (domain.Item, error) {
	it, ok := f.itemsByTag[tag]
	if !ok {
		return domain.Item{}, store.ErrNotFound
	}
	return it, nil
}

func int64p(v int64) *int64 { return &v }

func TestComputeReconciliation_BrandNewLocation(t *testing.T) {
	f := newFakeReconcileStore()
	f.addItem("ZKEI", nil) // unlinked item, about to be added to @XYZ

	diff, err := ComputeReconciliation(context.Background(), f, "@XYZ", []string{"ZKEI"})
	if err != nil {
		t.Fatalf("ComputeReconciliation: %v", err)
	}
	if !reflect.DeepEqual(diff.Added, []string{"ZKEI"}) {
		t.Errorf("Added = %v, want [ZKEI]", diff.Added)
	}
	if len(diff.Moved) != 0 || len(diff.Removed) != 0 {
		t.Errorf("unexpected Moved/Removed: %+v", diff)
	}
}

func TestComputeReconciliation_AddedMovedRemoved(t *testing.T) {
	f := newFakeReconcileStore()
	oldLoc := f.addLocation(1, "@QRS")
	newLoc := f.addLocation(2, "@XYZ")

	f.addItem("ZKEI", nil)               // will be ADDED to @XYZ (was unlinked)
	f.addItem("GKEI", int64p(oldLoc.ID)) // will be MOVED from @QRS to @XYZ
	f.addItem("XDKW", int64p(newLoc.ID)) // currently at @XYZ, absent from frame -> REMOVED
	f.addItem("STAY", int64p(newLoc.ID)) // currently at @XYZ, present in frame -> unchanged

	diff, err := ComputeReconciliation(context.Background(), f, "@XYZ", []string{"ZKEI", "GKEI", "STAY"})
	if err != nil {
		t.Fatalf("ComputeReconciliation: %v", err)
	}

	if !reflect.DeepEqual(diff.Added, []string{"ZKEI"}) {
		t.Errorf("Added = %v, want [ZKEI]", diff.Added)
	}
	if len(diff.Moved) != 1 || diff.Moved[0].AssetTag != "GKEI" || diff.Moved[0].FromLocation != "@QRS" {
		t.Errorf("Moved = %+v, want [{GKEI @QRS}]", diff.Moved)
	}
	if !reflect.DeepEqual(diff.Removed, []string{"XDKW"}) {
		t.Errorf("Removed = %v, want [XDKW]", diff.Removed)
	}
}

func TestComputeReconciliation_UnknownFrameTagIsNew(t *testing.T) {
	f := newFakeReconcileStore()
	// "NOPE" was never captured as an item — it should be classified as New,
	// not silently dropped nor treated as Added (no item exists yet to relink)
	diff, err := ComputeReconciliation(context.Background(), f, "@XYZ", []string{"NOPE"})
	if err != nil {
		t.Fatalf("ComputeReconciliation: %v", err)
	}
	if !reflect.DeepEqual(diff.New, []string{"NOPE"}) {
		t.Errorf("New = %v, want [NOPE]", diff.New)
	}
	if len(diff.Added) != 0 || len(diff.Moved) != 0 || len(diff.Removed) != 0 {
		t.Errorf("expected only New for an unknown tag, got %+v", diff)
	}
}

func TestComputeReconciliation_DuplicateFrameTagsDeduped(t *testing.T) {
	f := newFakeReconcileStore()
	f.addItem("ZKEI", nil)

	diff, err := ComputeReconciliation(context.Background(), f, "@XYZ", []string{"ZKEI", "ZKEI", "ZKEI"})
	if err != nil {
		t.Fatalf("ComputeReconciliation: %v", err)
	}
	if len(diff.Added) != 1 {
		t.Errorf("Added = %v, want exactly one ZKEI despite 3 frame occurrences", diff.Added)
	}
}

func TestComputeReconciliation_NoChangeWhenFrameMatchesCurrent(t *testing.T) {
	f := newFakeReconcileStore()
	loc := f.addLocation(1, "@XYZ")
	f.addItem("ZKEI", int64p(loc.ID))

	diff, err := ComputeReconciliation(context.Background(), f, "@XYZ", []string{"ZKEI"})
	if err != nil {
		t.Fatalf("ComputeReconciliation: %v", err)
	}
	if len(diff.Added) != 0 || len(diff.Moved) != 0 || len(diff.Removed) != 0 {
		t.Errorf("expected no-op diff, got %+v", diff)
	}
}

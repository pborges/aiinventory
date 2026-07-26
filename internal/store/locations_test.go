package store

import (
	"context"
	"errors"
	"testing"

	"github.com/pborges/aiinventory/internal/domain"
)

func TestGetOrCreateLocation(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	user, _ := s.CreateFirstUser(ctx, "alice", "hash")

	if _, err := s.GetLocationByCode(ctx, "@XYZ"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetLocationByCode on missing code = %v, want ErrNotFound", err)
	}

	loc1, err := s.GetOrCreateLocation(ctx, "@XYZ", user.ID)
	if err != nil {
		t.Fatalf("GetOrCreateLocation: %v", err)
	}
	loc2, err := s.GetOrCreateLocation(ctx, "@XYZ", user.ID)
	if err != nil {
		t.Fatalf("GetOrCreateLocation second call: %v", err)
	}
	if loc1.ID != loc2.ID {
		t.Fatalf("GetOrCreateLocation created two rows for the same code: %d != %d", loc1.ID, loc2.ID)
	}
}

func TestApplyReconciliation(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	user, _ := s.CreateFirstUser(ctx, "alice", "hash")

	oldLoc, err := s.GetOrCreateLocation(ctx, "@QRS", user.ID)
	if err != nil {
		t.Fatalf("GetOrCreateLocation @QRS: %v", err)
	}

	zkei, _ := s.CreateItem(ctx, "ZKEI") // will be added to @XYZ (currently unlinked)
	gkei, _ := s.CreateItem(ctx, "GKEI") // will move from @QRS to @XYZ
	if err := s.SetItemLocation(ctx, gkei.ID, &oldLoc.ID); err != nil {
		t.Fatalf("SetItemLocation gkei: %v", err)
	}

	diff := domain.ReconcileDiff{
		LocationCode: "@XYZ",
		Added:        []string{zkei.AssetTag},
		Moved:        []domain.MovedItem{{AssetTag: gkei.AssetTag, FromLocation: "@QRS"}},
		Removed:      nil,
	}

	if err := s.ApplyReconciliation(ctx, user.ID, diff); err != nil {
		t.Fatalf("ApplyReconciliation: %v", err)
	}

	newLoc, err := s.GetLocationByCode(ctx, "@XYZ")
	if err != nil {
		t.Fatalf("GetLocationByCode @XYZ (should've been created): %v", err)
	}

	gotZkei, _ := s.GetItemByID(ctx, zkei.ID)
	if gotZkei.LocationID == nil || *gotZkei.LocationID != newLoc.ID {
		t.Errorf("ZKEI location = %v, want %d", gotZkei.LocationID, newLoc.ID)
	}
	gotGkei, _ := s.GetItemByID(ctx, gkei.ID)
	if gotGkei.LocationID == nil || *gotGkei.LocationID != newLoc.ID {
		t.Errorf("GKEI location = %v, want %d", gotGkei.LocationID, newLoc.ID)
	}

	items, err := s.ListItemsByLocation(ctx, oldLoc.ID)
	if err != nil || len(items) != 0 {
		t.Errorf("@QRS should now be empty, got %+v, %v", items, err)
	}

	activity, err := s.ListActivityForLocation(ctx, newLoc.ID)
	if err != nil {
		t.Fatalf("ListActivityForLocation: %v", err)
	}
	// one item_moved entry each for ZKEI/GKEI plus one location_reconciled summary
	if len(activity) != 3 {
		t.Fatalf("got %d activity entries for @XYZ, want 3: %+v", len(activity), activity)
	}
}

func TestApplyReconciliationRemoved(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	user, _ := s.CreateFirstUser(ctx, "alice", "hash")

	loc, _ := s.GetOrCreateLocation(ctx, "@XYZ", user.ID)
	xdkw, _ := s.CreateItem(ctx, "XDKW")
	if err := s.SetItemLocation(ctx, xdkw.ID, &loc.ID); err != nil {
		t.Fatalf("SetItemLocation: %v", err)
	}

	diff := domain.ReconcileDiff{LocationCode: "@XYZ", Removed: []string{"XDKW"}}
	if err := s.ApplyReconciliation(ctx, user.ID, diff); err != nil {
		t.Fatalf("ApplyReconciliation: %v", err)
	}

	got, err := s.GetItemByID(ctx, xdkw.ID)
	if err != nil {
		t.Fatalf("GetItemByID: %v", err)
	}
	if got.LocationID != nil {
		t.Errorf("XDKW location = %v, want nil (unlinked)", got.LocationID)
	}
}

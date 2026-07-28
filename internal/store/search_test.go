package store

import (
	"context"
	"testing"
)

func TestSearchItemsFilters(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	user, _ := s.CreateFirstUser(ctx, "alice", "hash")

	withDesc, _ := s.CreateItem(ctx, "ZKEI")
	s.UpdateItemDescription(ctx, withDesc.ID, "a cordless drill")

	_, err := s.CreateItem(ctx, "GKEI")
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}

	loc, _ := s.GetOrCreateLocation(ctx, "@XYZ", user.ID)
	located, _ := s.CreateItem(ctx, "XDKW")
	s.SetItemLocation(ctx, located.ID, &loc.ID)

	all, err := s.SearchItems(ctx, SearchParams{})
	if err != nil {
		t.Fatalf("SearchItems: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("got %d items with no filters, want 3", len(all))
	}

	noDescResults, err := s.SearchItems(ctx, SearchParams{NoDescription: true})
	if err != nil {
		t.Fatalf("SearchItems(NoDescription): %v", err)
	}
	if len(noDescResults) != 2 || !containsTag(noDescResults, "GKEI") || !containsTag(noDescResults, "XDKW") {
		t.Fatalf("NoDescription results = %+v, want [GKEI XDKW]", noDescResults)
	}

	noLocResults, err := s.SearchItems(ctx, SearchParams{NoLocation: true})
	if err != nil {
		t.Fatalf("SearchItems(NoLocation): %v", err)
	}
	if len(noLocResults) != 2 || !containsTag(noLocResults, "ZKEI") || !containsTag(noLocResults, "GKEI") {
		t.Fatalf("NoLocation results = %+v, want [ZKEI GKEI]", noLocResults)
	}

	byLoc, err := s.SearchItems(ctx, SearchParams{LocationID: &loc.ID})
	if err != nil {
		t.Fatalf("SearchItems(LocationID): %v", err)
	}
	if len(byLoc) != 1 || byLoc[0].AssetTag != "XDKW" || byLoc[0].LocationCode != "@XYZ" {
		t.Fatalf("LocationID results = %+v, want [XDKW @XYZ]", byLoc)
	}

	locTag, err := s.CreateLocationTag(ctx, "Garage", "#a6e22e")
	if err != nil {
		t.Fatalf("CreateLocationTag: %v", err)
	}
	if err := s.SetLocationTags(ctx, loc.ID, []int64{locTag.ID}); err != nil {
		t.Fatalf("SetLocationTags: %v", err)
	}

	byLocTag, err := s.SearchItems(ctx, SearchParams{LocationTagIDs: []int64{locTag.ID}})
	if err != nil {
		t.Fatalf("SearchItems(LocationTagIDs): %v", err)
	}
	if len(byLocTag) != 1 || byLocTag[0].AssetTag != "XDKW" {
		t.Fatalf("LocationTagIDs results = %+v, want [XDKW]", byLocTag)
	}

	otherLocTag, err := s.CreateLocationTag(ctx, "Attic", "#f92672")
	if err != nil {
		t.Fatalf("CreateLocationTag: %v", err)
	}
	byUnusedLocTag, err := s.SearchItems(ctx, SearchParams{LocationTagIDs: []int64{otherLocTag.ID}})
	if err != nil {
		t.Fatalf("SearchItems(LocationTagIDs unused): %v", err)
	}
	if len(byUnusedLocTag) != 0 {
		t.Fatalf("LocationTagIDs(unused) results = %+v, want none", byUnusedLocTag)
	}
}

func TestSearchItemsFullText(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	user, _ := s.CreateFirstUser(ctx, "alice", "hash")

	drill, _ := s.CreateItem(ctx, "ZKEI")
	s.UpdateItemDescription(ctx, drill.ID, "a cordless drill, model XR-500")

	// this item's consolidated description says nothing about "S/N 99887", but
	// a per-image note does — should still be findable by that serial number
	sawItem, _ := s.CreateItem(ctx, "GKEI")
	s.UpdateItemDescription(ctx, sawItem.ID, "a circular saw")
	s.AddImage(ctx, sawItem.ID, []byte("jpeg"), "image/jpeg", "S/N 99887 stamped on the base", user.ID)

	unrelated, _ := s.CreateItem(ctx, "XDKW")
	s.UpdateItemDescription(ctx, unrelated.ID, "a garden hose")

	t.Run("matches item description", func(t *testing.T) {
		results, err := s.SearchItems(ctx, SearchParams{Query: "drill"})
		if err != nil {
			t.Fatalf("SearchItems: %v", err)
		}
		if len(results) != 1 || results[0].AssetTag != "ZKEI" {
			t.Fatalf("results = %+v, want [ZKEI]", results)
		}
	})

	t.Run("matches per-image note not yet in consolidated description", func(t *testing.T) {
		results, err := s.SearchItems(ctx, SearchParams{Query: "99887"})
		if err != nil {
			t.Fatalf("SearchItems: %v", err)
		}
		if len(results) != 1 || results[0].AssetTag != "GKEI" {
			t.Fatalf("results = %+v, want [GKEI] (found via per-image note)", results)
		}
	})

	t.Run("no match", func(t *testing.T) {
		results, err := s.SearchItems(ctx, SearchParams{Query: "nonexistentword"})
		if err != nil {
			t.Fatalf("SearchItems: %v", err)
		}
		if len(results) != 0 {
			t.Fatalf("results = %+v, want none", results)
		}
	})

	_ = unrelated
}

func TestSearchItemsPrimaryImage(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	user, _ := s.CreateFirstUser(ctx, "alice", "hash")
	item, _ := s.CreateItem(ctx, "ZKEI")

	results, err := s.SearchItems(ctx, SearchParams{})
	if err != nil {
		t.Fatalf("SearchItems: %v", err)
	}
	if results[0].PrimaryImageID != nil {
		t.Fatalf("PrimaryImageID = %v, want nil before any image exists", results[0].PrimaryImageID)
	}

	first, _ := s.AddImage(ctx, item.ID, []byte("first"), "image/jpeg", "", user.ID)
	s.AddImage(ctx, item.ID, []byte("second"), "image/jpeg", "", user.ID)

	results, err = s.SearchItems(ctx, SearchParams{})
	if err != nil {
		t.Fatalf("SearchItems: %v", err)
	}
	if results[0].PrimaryImageID == nil || *results[0].PrimaryImageID != first.ID {
		t.Fatalf("PrimaryImageID = %v, want %d (the first/lowest-sort_order image)", results[0].PrimaryImageID, first.ID)
	}
}

func containsTag(items []ItemSummary, tag string) bool {
	for _, it := range items {
		if it.AssetTag == tag {
			return true
		}
	}
	return false
}

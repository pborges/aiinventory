package store

import (
	"context"
	"errors"
	"testing"
)

func TestCreateAndListLocationLabels(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)

	green, err := s.CreateLocationLabel(ctx, "warehouse", "#a6e22e")
	if err != nil {
		t.Fatalf("CreateLocationLabel: %v", err)
	}
	if green.Name != "warehouse" || green.Color != "#a6e22e" {
		t.Fatalf("CreateLocationLabel result = %+v, want name=warehouse color=#a6e22e", green)
	}

	if _, err := s.CreateLocationLabel(ctx, "office", "#f92672"); err != nil {
		t.Fatalf("CreateLocationLabel: %v", err)
	}

	labels, err := s.ListLocationLabels(ctx)
	if err != nil {
		t.Fatalf("ListLocationLabels: %v", err)
	}
	if len(labels) != 2 || labels[0].Name != "office" || labels[1].Name != "warehouse" {
		t.Fatalf("ListLocationLabels = %+v, want [office warehouse] ordered by name", labels)
	}
}

func TestCreateLocationLabelDuplicateName(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)

	if _, err := s.CreateLocationLabel(ctx, "warehouse", "#a6e22e"); err != nil {
		t.Fatalf("CreateLocationLabel: %v", err)
	}
	if _, err := s.CreateLocationLabel(ctx, "warehouse", "#f92672"); !errors.Is(err, ErrLocationLabelNameTaken) {
		t.Fatalf("CreateLocationLabel duplicate name = %v, want ErrLocationLabelNameTaken", err)
	}
}

func TestUpdateLocationLabel(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)

	label, _ := s.CreateLocationLabel(ctx, "warehouse", "#a6e22e")

	if err := s.UpdateLocationLabel(ctx, label.ID, "cold storage", "#f92672"); err != nil {
		t.Fatalf("UpdateLocationLabel: %v", err)
	}

	updated, err := s.GetLocationLabelByID(ctx, label.ID)
	if err != nil {
		t.Fatalf("GetLocationLabelByID: %v", err)
	}
	if updated.Name != "cold storage" || updated.Color != "#f92672" {
		t.Fatalf("GetLocationLabelByID after update = %+v, want name=cold storage color=#f92672", updated)
	}

	if err := s.UpdateLocationLabel(ctx, 9999, "nope", "#000000"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateLocationLabel missing id = %v, want ErrNotFound", err)
	}
}

func TestDeleteLocationLabelDetachesFromLocations(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	user, _ := s.CreateFirstUser(ctx, "alice", "hash")
	loc, _ := s.GetOrCreateLocation(ctx, "@XYZ", user.ID)
	label, _ := s.CreateLocationLabel(ctx, "warehouse", "#a6e22e")

	if err := s.SetLocationLabels(ctx, loc.ID, []int64{label.ID}); err != nil {
		t.Fatalf("SetLocationLabels: %v", err)
	}

	if err := s.DeleteLocationLabel(ctx, label.ID); err != nil {
		t.Fatalf("DeleteLocationLabel: %v", err)
	}

	labels, err := s.ListLabelsByLocation(ctx, loc.ID)
	if err != nil {
		t.Fatalf("ListLabelsByLocation: %v", err)
	}
	if len(labels) != 0 {
		t.Fatalf("ListLabelsByLocation after label deleted = %+v, want none", labels)
	}

	if err := s.DeleteLocationLabel(ctx, label.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete already-deleted label = %v, want ErrNotFound", err)
	}
}

func TestSetLocationLabelsReplacesFullSet(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	user, _ := s.CreateFirstUser(ctx, "alice", "hash")
	loc, _ := s.GetOrCreateLocation(ctx, "@XYZ", user.ID)
	a, _ := s.CreateLocationLabel(ctx, "a", "#a6e22e")
	b, _ := s.CreateLocationLabel(ctx, "b", "#f92672")
	c, _ := s.CreateLocationLabel(ctx, "c", "#66d9ef")

	if err := s.SetLocationLabels(ctx, loc.ID, []int64{a.ID, b.ID}); err != nil {
		t.Fatalf("SetLocationLabels: %v", err)
	}
	labels, _ := s.ListLabelsByLocation(ctx, loc.ID)
	if len(labels) != 2 {
		t.Fatalf("ListLabelsByLocation = %+v, want [a b]", labels)
	}

	// replace with a different set entirely
	if err := s.SetLocationLabels(ctx, loc.ID, []int64{c.ID}); err != nil {
		t.Fatalf("SetLocationLabels (replace): %v", err)
	}
	labels, _ = s.ListLabelsByLocation(ctx, loc.ID)
	if len(labels) != 1 || labels[0].ID != c.ID {
		t.Fatalf("ListLabelsByLocation after replace = %+v, want just [c]", labels)
	}

	// clearing entirely
	if err := s.SetLocationLabels(ctx, loc.ID, nil); err != nil {
		t.Fatalf("SetLocationLabels (clear): %v", err)
	}
	labels, _ = s.ListLabelsByLocation(ctx, loc.ID)
	if len(labels) != 0 {
		t.Fatalf("ListLabelsByLocation after clear = %+v, want none", labels)
	}
}

func TestListLabelsForLocations(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	user, _ := s.CreateFirstUser(ctx, "alice", "hash")
	loc1, _ := s.GetOrCreateLocation(ctx, "@AAA", user.ID)
	loc2, _ := s.GetOrCreateLocation(ctx, "@BBB", user.ID)
	loc3, _ := s.GetOrCreateLocation(ctx, "@CCC", user.ID)
	a, _ := s.CreateLocationLabel(ctx, "a", "#a6e22e")
	b, _ := s.CreateLocationLabel(ctx, "b", "#f92672")

	s.SetLocationLabels(ctx, loc1.ID, []int64{a.ID, b.ID})
	s.SetLocationLabels(ctx, loc2.ID, []int64{b.ID})
	// loc3 has no labels

	byLocation, err := s.ListLabelsForLocations(ctx, []int64{loc1.ID, loc2.ID, loc3.ID})
	if err != nil {
		t.Fatalf("ListLabelsForLocations: %v", err)
	}
	if len(byLocation[loc1.ID]) != 2 {
		t.Fatalf("byLocation[loc1] = %+v, want 2 labels", byLocation[loc1.ID])
	}
	if len(byLocation[loc2.ID]) != 1 || byLocation[loc2.ID][0].ID != b.ID {
		t.Fatalf("byLocation[loc2] = %+v, want just [b]", byLocation[loc2.ID])
	}
	if len(byLocation[loc3.ID]) != 0 {
		t.Fatalf("byLocation[loc3] = %+v, want none", byLocation[loc3.ID])
	}

	empty, err := s.ListLabelsForLocations(ctx, nil)
	if err != nil {
		t.Fatalf("ListLabelsForLocations(nil): %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("ListLabelsForLocations(nil) = %+v, want empty map", empty)
	}
}

func TestLocationLabelsIndependentFromItemLabels(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	user, _ := s.CreateFirstUser(ctx, "alice", "hash")
	loc, _ := s.GetOrCreateLocation(ctx, "@XYZ", user.ID)
	item, _ := s.CreateItem(ctx, "ZKEI")

	// same name is allowed in both pools since they're independent tables
	if _, err := s.CreateLabel(ctx, "fragile", "#a6e22e"); err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	locLabel, err := s.CreateLocationLabel(ctx, "fragile", "#f92672")
	if err != nil {
		t.Fatalf("CreateLocationLabel with same name as an item label: %v", err)
	}

	if err := s.SetLocationLabels(ctx, loc.ID, []int64{locLabel.ID}); err != nil {
		t.Fatalf("SetLocationLabels: %v", err)
	}

	itemLabels, err := s.ListLabelsByItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("ListLabelsByItem: %v", err)
	}
	if len(itemLabels) != 0 {
		t.Fatalf("item labels = %+v, want none (location label assignment shouldn't leak)", itemLabels)
	}

	locLabels, err := s.ListLabelsByLocation(ctx, loc.ID)
	if err != nil {
		t.Fatalf("ListLabelsByLocation: %v", err)
	}
	if len(locLabels) != 1 || locLabels[0].Color != "#f92672" {
		t.Fatalf("location labels = %+v, want just the location-pool fragile label", locLabels)
	}
}

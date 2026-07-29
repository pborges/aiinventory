package store

import (
	"context"
	"errors"
	"testing"
)

func TestCreateAndListLabels(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)

	green, err := s.CreateLabel(ctx, "fragile", "#a6e22e")
	if err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	if green.Name != "fragile" || green.Color != "#a6e22e" {
		t.Fatalf("CreateLabel result = %+v, want name=fragile color=#a6e22e", green)
	}

	if _, err := s.CreateLabel(ctx, "loaned", "#f92672"); err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}

	labels, err := s.ListLabels(ctx)
	if err != nil {
		t.Fatalf("ListLabels: %v", err)
	}
	if len(labels) != 2 || labels[0].Name != "fragile" || labels[1].Name != "loaned" {
		t.Fatalf("ListLabels = %+v, want [fragile loaned] ordered by name", labels)
	}
}

func TestCreateLabelDuplicateName(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)

	if _, err := s.CreateLabel(ctx, "fragile", "#a6e22e"); err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	if _, err := s.CreateLabel(ctx, "fragile", "#f92672"); !errors.Is(err, ErrLabelNameTaken) {
		t.Fatalf("CreateLabel duplicate name = %v, want ErrLabelNameTaken", err)
	}
}

func TestUpdateLabel(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)

	label, _ := s.CreateLabel(ctx, "fragile", "#a6e22e")

	if err := s.UpdateLabel(ctx, label.ID, "handle with care", "#f92672"); err != nil {
		t.Fatalf("UpdateLabel: %v", err)
	}

	updated, err := s.GetLabelByID(ctx, label.ID)
	if err != nil {
		t.Fatalf("GetLabelByID: %v", err)
	}
	if updated.Name != "handle with care" || updated.Color != "#f92672" {
		t.Fatalf("GetLabelByID after update = %+v, want name=handle with care color=#f92672", updated)
	}

	if err := s.UpdateLabel(ctx, 9999, "nope", "#000000"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateLabel missing id = %v, want ErrNotFound", err)
	}
}

func TestDeleteLabelDetachesFromItems(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	item, _ := s.CreateItem(ctx, "ZKEI")
	label, _ := s.CreateLabel(ctx, "fragile", "#a6e22e")

	if err := s.SetItemLabels(ctx, item.ID, []int64{label.ID}); err != nil {
		t.Fatalf("SetItemLabels: %v", err)
	}

	if err := s.DeleteLabel(ctx, label.ID); err != nil {
		t.Fatalf("DeleteLabel: %v", err)
	}

	labels, err := s.ListLabelsByItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("ListLabelsByItem: %v", err)
	}
	if len(labels) != 0 {
		t.Fatalf("ListLabelsByItem after label deleted = %+v, want none", labels)
	}

	if err := s.DeleteLabel(ctx, label.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete already-deleted label = %v, want ErrNotFound", err)
	}
}

func TestSetItemLabelsReplacesFullSet(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	item, _ := s.CreateItem(ctx, "ZKEI")
	a, _ := s.CreateLabel(ctx, "a", "#a6e22e")
	b, _ := s.CreateLabel(ctx, "b", "#f92672")
	c, _ := s.CreateLabel(ctx, "c", "#66d9ef")

	if err := s.SetItemLabels(ctx, item.ID, []int64{a.ID, b.ID}); err != nil {
		t.Fatalf("SetItemLabels: %v", err)
	}
	labels, _ := s.ListLabelsByItem(ctx, item.ID)
	if len(labels) != 2 {
		t.Fatalf("ListLabelsByItem = %+v, want [a b]", labels)
	}

	// replace with a different set entirely
	if err := s.SetItemLabels(ctx, item.ID, []int64{c.ID}); err != nil {
		t.Fatalf("SetItemLabels (replace): %v", err)
	}
	labels, _ = s.ListLabelsByItem(ctx, item.ID)
	if len(labels) != 1 || labels[0].ID != c.ID {
		t.Fatalf("ListLabelsByItem after replace = %+v, want just [c]", labels)
	}

	// clearing entirely
	if err := s.SetItemLabels(ctx, item.ID, nil); err != nil {
		t.Fatalf("SetItemLabels (clear): %v", err)
	}
	labels, _ = s.ListLabelsByItem(ctx, item.ID)
	if len(labels) != 0 {
		t.Fatalf("ListLabelsByItem after clear = %+v, want none", labels)
	}
}

func TestListLabelsForItems(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	item1, _ := s.CreateItem(ctx, "ZKEI")
	item2, _ := s.CreateItem(ctx, "GKEI")
	item3, _ := s.CreateItem(ctx, "XDKW")
	a, _ := s.CreateLabel(ctx, "a", "#a6e22e")
	b, _ := s.CreateLabel(ctx, "b", "#f92672")

	s.SetItemLabels(ctx, item1.ID, []int64{a.ID, b.ID})
	s.SetItemLabels(ctx, item2.ID, []int64{b.ID})
	// item3 has no labels

	byItem, err := s.ListLabelsForItems(ctx, []int64{item1.ID, item2.ID, item3.ID})
	if err != nil {
		t.Fatalf("ListLabelsForItems: %v", err)
	}
	if len(byItem[item1.ID]) != 2 {
		t.Fatalf("byItem[item1] = %+v, want 2 labels", byItem[item1.ID])
	}
	if len(byItem[item2.ID]) != 1 || byItem[item2.ID][0].ID != b.ID {
		t.Fatalf("byItem[item2] = %+v, want just [b]", byItem[item2.ID])
	}
	if len(byItem[item3.ID]) != 0 {
		t.Fatalf("byItem[item3] = %+v, want none", byItem[item3.ID])
	}

	empty, err := s.ListLabelsForItems(ctx, nil)
	if err != nil {
		t.Fatalf("ListLabelsForItems(nil): %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("ListLabelsForItems(nil) = %+v, want empty map", empty)
	}
}

func TestSearchItemsByLabel(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	fragile, _ := s.CreateLabel(ctx, "fragile", "#a6e22e")
	loaned, _ := s.CreateLabel(ctx, "loaned", "#f92672")

	tagged, _ := s.CreateItem(ctx, "ZKEI")
	s.SetItemLabels(ctx, tagged.ID, []int64{fragile.ID})

	otherTagged, _ := s.CreateItem(ctx, "GKEI")
	s.SetItemLabels(ctx, otherTagged.ID, []int64{loaned.ID})

	_, _ = s.CreateItem(ctx, "XDKW") // unlabeled

	results, err := s.SearchItems(ctx, SearchParams{LabelIDs: []int64{fragile.ID}})
	if err != nil {
		t.Fatalf("SearchItems: %v", err)
	}
	if len(results) != 1 || results[0].AssetTag != "ZKEI" {
		t.Fatalf("SearchItems(LabelIDs=[fragile]) = %+v, want [ZKEI]", results)
	}
	if len(results[0].Labels) != 1 || results[0].Labels[0].ID != fragile.ID {
		t.Fatalf("result.Labels = %+v, want [fragile]", results[0].Labels)
	}

	// OR semantics: matches either label
	orResults, err := s.SearchItems(ctx, SearchParams{LabelIDs: []int64{fragile.ID, loaned.ID}})
	if err != nil {
		t.Fatalf("SearchItems: %v", err)
	}
	if len(orResults) != 2 || !containsTag(orResults, "ZKEI") || !containsTag(orResults, "GKEI") {
		t.Fatalf("SearchItems(LabelIDs=[fragile,loaned]) = %+v, want [ZKEI GKEI]", orResults)
	}
}

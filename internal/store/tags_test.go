package store

import (
	"context"
	"errors"
	"testing"
)

func TestCreateAndListTags(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)

	green, err := s.CreateTag(ctx, "fragile", "#a6e22e")
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if green.Name != "fragile" || green.Color != "#a6e22e" {
		t.Fatalf("CreateTag result = %+v, want name=fragile color=#a6e22e", green)
	}

	if _, err := s.CreateTag(ctx, "loaned", "#f92672"); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	tags, err := s.ListTags(ctx)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if len(tags) != 2 || tags[0].Name != "fragile" || tags[1].Name != "loaned" {
		t.Fatalf("ListTags = %+v, want [fragile loaned] ordered by name", tags)
	}
}

func TestCreateTagDuplicateName(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)

	if _, err := s.CreateTag(ctx, "fragile", "#a6e22e"); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if _, err := s.CreateTag(ctx, "fragile", "#f92672"); !errors.Is(err, ErrTagNameTaken) {
		t.Fatalf("CreateTag duplicate name = %v, want ErrTagNameTaken", err)
	}
}

func TestUpdateTag(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)

	tag, _ := s.CreateTag(ctx, "fragile", "#a6e22e")

	if err := s.UpdateTag(ctx, tag.ID, "handle with care", "#f92672"); err != nil {
		t.Fatalf("UpdateTag: %v", err)
	}

	updated, err := s.GetTagByID(ctx, tag.ID)
	if err != nil {
		t.Fatalf("GetTagByID: %v", err)
	}
	if updated.Name != "handle with care" || updated.Color != "#f92672" {
		t.Fatalf("GetTagByID after update = %+v, want name=handle with care color=#f92672", updated)
	}

	if err := s.UpdateTag(ctx, 9999, "nope", "#000000"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateTag missing id = %v, want ErrNotFound", err)
	}
}

func TestDeleteTagDetachesFromItems(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	item, _ := s.CreateItem(ctx, "ZKEI")
	tag, _ := s.CreateTag(ctx, "fragile", "#a6e22e")

	if err := s.SetItemTags(ctx, item.ID, []int64{tag.ID}); err != nil {
		t.Fatalf("SetItemTags: %v", err)
	}

	if err := s.DeleteTag(ctx, tag.ID); err != nil {
		t.Fatalf("DeleteTag: %v", err)
	}

	tags, err := s.ListTagsByItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("ListTagsByItem: %v", err)
	}
	if len(tags) != 0 {
		t.Fatalf("ListTagsByItem after tag deleted = %+v, want none", tags)
	}

	if err := s.DeleteTag(ctx, tag.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete already-deleted tag = %v, want ErrNotFound", err)
	}
}

func TestSetItemTagsReplacesFullSet(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	item, _ := s.CreateItem(ctx, "ZKEI")
	a, _ := s.CreateTag(ctx, "a", "#a6e22e")
	b, _ := s.CreateTag(ctx, "b", "#f92672")
	c, _ := s.CreateTag(ctx, "c", "#66d9ef")

	if err := s.SetItemTags(ctx, item.ID, []int64{a.ID, b.ID}); err != nil {
		t.Fatalf("SetItemTags: %v", err)
	}
	tags, _ := s.ListTagsByItem(ctx, item.ID)
	if len(tags) != 2 {
		t.Fatalf("ListTagsByItem = %+v, want [a b]", tags)
	}

	// replace with a different set entirely
	if err := s.SetItemTags(ctx, item.ID, []int64{c.ID}); err != nil {
		t.Fatalf("SetItemTags (replace): %v", err)
	}
	tags, _ = s.ListTagsByItem(ctx, item.ID)
	if len(tags) != 1 || tags[0].ID != c.ID {
		t.Fatalf("ListTagsByItem after replace = %+v, want just [c]", tags)
	}

	// clearing entirely
	if err := s.SetItemTags(ctx, item.ID, nil); err != nil {
		t.Fatalf("SetItemTags (clear): %v", err)
	}
	tags, _ = s.ListTagsByItem(ctx, item.ID)
	if len(tags) != 0 {
		t.Fatalf("ListTagsByItem after clear = %+v, want none", tags)
	}
}

func TestListTagsForItems(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	item1, _ := s.CreateItem(ctx, "ZKEI")
	item2, _ := s.CreateItem(ctx, "GKEI")
	item3, _ := s.CreateItem(ctx, "XDKW")
	a, _ := s.CreateTag(ctx, "a", "#a6e22e")
	b, _ := s.CreateTag(ctx, "b", "#f92672")

	s.SetItemTags(ctx, item1.ID, []int64{a.ID, b.ID})
	s.SetItemTags(ctx, item2.ID, []int64{b.ID})
	// item3 has no tags

	byItem, err := s.ListTagsForItems(ctx, []int64{item1.ID, item2.ID, item3.ID})
	if err != nil {
		t.Fatalf("ListTagsForItems: %v", err)
	}
	if len(byItem[item1.ID]) != 2 {
		t.Fatalf("byItem[item1] = %+v, want 2 tags", byItem[item1.ID])
	}
	if len(byItem[item2.ID]) != 1 || byItem[item2.ID][0].ID != b.ID {
		t.Fatalf("byItem[item2] = %+v, want just [b]", byItem[item2.ID])
	}
	if len(byItem[item3.ID]) != 0 {
		t.Fatalf("byItem[item3] = %+v, want none", byItem[item3.ID])
	}

	empty, err := s.ListTagsForItems(ctx, nil)
	if err != nil {
		t.Fatalf("ListTagsForItems(nil): %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("ListTagsForItems(nil) = %+v, want empty map", empty)
	}
}

func TestSearchItemsByTag(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	fragile, _ := s.CreateTag(ctx, "fragile", "#a6e22e")
	loaned, _ := s.CreateTag(ctx, "loaned", "#f92672")

	tagged, _ := s.CreateItem(ctx, "ZKEI")
	s.SetItemTags(ctx, tagged.ID, []int64{fragile.ID})

	otherTagged, _ := s.CreateItem(ctx, "GKEI")
	s.SetItemTags(ctx, otherTagged.ID, []int64{loaned.ID})

	_, _ = s.CreateItem(ctx, "XDKW") // untagged

	results, err := s.SearchItems(ctx, SearchParams{TagIDs: []int64{fragile.ID}})
	if err != nil {
		t.Fatalf("SearchItems: %v", err)
	}
	if len(results) != 1 || results[0].AssetTag != "ZKEI" {
		t.Fatalf("SearchItems(TagIDs=[fragile]) = %+v, want [ZKEI]", results)
	}
	if len(results[0].Tags) != 1 || results[0].Tags[0].ID != fragile.ID {
		t.Fatalf("result.Tags = %+v, want [fragile]", results[0].Tags)
	}

	// OR semantics: matches either tag
	orResults, err := s.SearchItems(ctx, SearchParams{TagIDs: []int64{fragile.ID, loaned.ID}})
	if err != nil {
		t.Fatalf("SearchItems: %v", err)
	}
	if len(orResults) != 2 || !containsTag(orResults, "ZKEI") || !containsTag(orResults, "GKEI") {
		t.Fatalf("SearchItems(TagIDs=[fragile,loaned]) = %+v, want [ZKEI GKEI]", orResults)
	}
}

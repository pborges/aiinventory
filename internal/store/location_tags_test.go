package store

import (
	"context"
	"errors"
	"testing"
)

func TestCreateAndListLocationTags(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)

	green, err := s.CreateLocationTag(ctx, "warehouse", "#a6e22e")
	if err != nil {
		t.Fatalf("CreateLocationTag: %v", err)
	}
	if green.Name != "warehouse" || green.Color != "#a6e22e" {
		t.Fatalf("CreateLocationTag result = %+v, want name=warehouse color=#a6e22e", green)
	}

	if _, err := s.CreateLocationTag(ctx, "office", "#f92672"); err != nil {
		t.Fatalf("CreateLocationTag: %v", err)
	}

	tags, err := s.ListLocationTags(ctx)
	if err != nil {
		t.Fatalf("ListLocationTags: %v", err)
	}
	if len(tags) != 2 || tags[0].Name != "office" || tags[1].Name != "warehouse" {
		t.Fatalf("ListLocationTags = %+v, want [office warehouse] ordered by name", tags)
	}
}

func TestCreateLocationTagDuplicateName(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)

	if _, err := s.CreateLocationTag(ctx, "warehouse", "#a6e22e"); err != nil {
		t.Fatalf("CreateLocationTag: %v", err)
	}
	if _, err := s.CreateLocationTag(ctx, "warehouse", "#f92672"); !errors.Is(err, ErrLocationTagNameTaken) {
		t.Fatalf("CreateLocationTag duplicate name = %v, want ErrLocationTagNameTaken", err)
	}
}

func TestUpdateLocationTag(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)

	tag, _ := s.CreateLocationTag(ctx, "warehouse", "#a6e22e")

	if err := s.UpdateLocationTag(ctx, tag.ID, "cold storage", "#f92672"); err != nil {
		t.Fatalf("UpdateLocationTag: %v", err)
	}

	updated, err := s.GetLocationTagByID(ctx, tag.ID)
	if err != nil {
		t.Fatalf("GetLocationTagByID: %v", err)
	}
	if updated.Name != "cold storage" || updated.Color != "#f92672" {
		t.Fatalf("GetLocationTagByID after update = %+v, want name=cold storage color=#f92672", updated)
	}

	if err := s.UpdateLocationTag(ctx, 9999, "nope", "#000000"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateLocationTag missing id = %v, want ErrNotFound", err)
	}
}

func TestDeleteLocationTagDetachesFromLocations(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	user, _ := s.CreateFirstUser(ctx, "alice", "hash")
	loc, _ := s.GetOrCreateLocation(ctx, "@XYZ", user.ID)
	tag, _ := s.CreateLocationTag(ctx, "warehouse", "#a6e22e")

	if err := s.SetLocationTags(ctx, loc.ID, []int64{tag.ID}); err != nil {
		t.Fatalf("SetLocationTags: %v", err)
	}

	if err := s.DeleteLocationTag(ctx, tag.ID); err != nil {
		t.Fatalf("DeleteLocationTag: %v", err)
	}

	tags, err := s.ListTagsByLocation(ctx, loc.ID)
	if err != nil {
		t.Fatalf("ListTagsByLocation: %v", err)
	}
	if len(tags) != 0 {
		t.Fatalf("ListTagsByLocation after tag deleted = %+v, want none", tags)
	}

	if err := s.DeleteLocationTag(ctx, tag.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete already-deleted tag = %v, want ErrNotFound", err)
	}
}

func TestSetLocationTagsReplacesFullSet(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	user, _ := s.CreateFirstUser(ctx, "alice", "hash")
	loc, _ := s.GetOrCreateLocation(ctx, "@XYZ", user.ID)
	a, _ := s.CreateLocationTag(ctx, "a", "#a6e22e")
	b, _ := s.CreateLocationTag(ctx, "b", "#f92672")
	c, _ := s.CreateLocationTag(ctx, "c", "#66d9ef")

	if err := s.SetLocationTags(ctx, loc.ID, []int64{a.ID, b.ID}); err != nil {
		t.Fatalf("SetLocationTags: %v", err)
	}
	tags, _ := s.ListTagsByLocation(ctx, loc.ID)
	if len(tags) != 2 {
		t.Fatalf("ListTagsByLocation = %+v, want [a b]", tags)
	}

	// replace with a different set entirely
	if err := s.SetLocationTags(ctx, loc.ID, []int64{c.ID}); err != nil {
		t.Fatalf("SetLocationTags (replace): %v", err)
	}
	tags, _ = s.ListTagsByLocation(ctx, loc.ID)
	if len(tags) != 1 || tags[0].ID != c.ID {
		t.Fatalf("ListTagsByLocation after replace = %+v, want just [c]", tags)
	}

	// clearing entirely
	if err := s.SetLocationTags(ctx, loc.ID, nil); err != nil {
		t.Fatalf("SetLocationTags (clear): %v", err)
	}
	tags, _ = s.ListTagsByLocation(ctx, loc.ID)
	if len(tags) != 0 {
		t.Fatalf("ListTagsByLocation after clear = %+v, want none", tags)
	}
}

func TestListTagsForLocations(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	user, _ := s.CreateFirstUser(ctx, "alice", "hash")
	loc1, _ := s.GetOrCreateLocation(ctx, "@AAA", user.ID)
	loc2, _ := s.GetOrCreateLocation(ctx, "@BBB", user.ID)
	loc3, _ := s.GetOrCreateLocation(ctx, "@CCC", user.ID)
	a, _ := s.CreateLocationTag(ctx, "a", "#a6e22e")
	b, _ := s.CreateLocationTag(ctx, "b", "#f92672")

	s.SetLocationTags(ctx, loc1.ID, []int64{a.ID, b.ID})
	s.SetLocationTags(ctx, loc2.ID, []int64{b.ID})
	// loc3 has no tags

	byLocation, err := s.ListTagsForLocations(ctx, []int64{loc1.ID, loc2.ID, loc3.ID})
	if err != nil {
		t.Fatalf("ListTagsForLocations: %v", err)
	}
	if len(byLocation[loc1.ID]) != 2 {
		t.Fatalf("byLocation[loc1] = %+v, want 2 tags", byLocation[loc1.ID])
	}
	if len(byLocation[loc2.ID]) != 1 || byLocation[loc2.ID][0].ID != b.ID {
		t.Fatalf("byLocation[loc2] = %+v, want just [b]", byLocation[loc2.ID])
	}
	if len(byLocation[loc3.ID]) != 0 {
		t.Fatalf("byLocation[loc3] = %+v, want none", byLocation[loc3.ID])
	}

	empty, err := s.ListTagsForLocations(ctx, nil)
	if err != nil {
		t.Fatalf("ListTagsForLocations(nil): %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("ListTagsForLocations(nil) = %+v, want empty map", empty)
	}
}

func TestLocationTagsIndependentFromItemTags(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	user, _ := s.CreateFirstUser(ctx, "alice", "hash")
	loc, _ := s.GetOrCreateLocation(ctx, "@XYZ", user.ID)
	item, _ := s.CreateItem(ctx, "ZKEI")

	// same name is allowed in both pools since they're independent tables
	if _, err := s.CreateTag(ctx, "fragile", "#a6e22e"); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	locTag, err := s.CreateLocationTag(ctx, "fragile", "#f92672")
	if err != nil {
		t.Fatalf("CreateLocationTag with same name as an item tag: %v", err)
	}

	if err := s.SetLocationTags(ctx, loc.ID, []int64{locTag.ID}); err != nil {
		t.Fatalf("SetLocationTags: %v", err)
	}

	itemTags, err := s.ListTagsByItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("ListTagsByItem: %v", err)
	}
	if len(itemTags) != 0 {
		t.Fatalf("item tags = %+v, want none (location tag assignment shouldn't leak)", itemTags)
	}

	locTags, err := s.ListTagsByLocation(ctx, loc.ID)
	if err != nil {
		t.Fatalf("ListTagsByLocation: %v", err)
	}
	if len(locTags) != 1 || locTags[0].Color != "#f92672" {
		t.Fatalf("location tags = %+v, want just the location-pool fragile tag", locTags)
	}
}

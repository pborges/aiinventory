package store

import (
	"context"
	"testing"
	"time"
)

func TestRecordDuplicateRunAndListPending(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	user, _ := s.CreateFirstUser(ctx, "alice", "hash")
	s.CreateItem(ctx, "ZKEI")
	s.CreateItem(ctx, "GKEI")
	s.CreateItem(ctx, "XDKW")

	err := s.RecordDuplicateRun(ctx, "completed", user.ID, time.Now(), []DuplicateGroupCandidate{
		{AssetTags: []string{"ZKEI", "GKEI"}, Reasoning: "same serial number"},
		{AssetTags: []string{"XDKW"}, Reasoning: "degenerate: only one real tag"},  // should be dropped
		{AssetTags: []string{"NOPE", "ALSO-NOPE"}, Reasoning: "hallucinated tags"}, // should be dropped (0 resolvable members)
	})
	if err != nil {
		t.Fatalf("RecordDuplicateRun: %v", err)
	}

	groups, err := s.ListPendingDuplicateGroups(ctx)
	if err != nil {
		t.Fatalf("ListPendingDuplicateGroups: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("got %d pending groups, want 1 (degenerate/hallucinated groups dropped): %+v", len(groups), groups)
	}
	if len(groups[0].Items) != 2 {
		t.Fatalf("group Items = %+v, want 2 members (GKEI, ZKEI)", groups[0].Items)
	}
}

func TestDismissDuplicateGroup(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	user, _ := s.CreateFirstUser(ctx, "alice", "hash")
	s.CreateItem(ctx, "ZKEI")
	s.CreateItem(ctx, "GKEI")
	s.RecordDuplicateRun(ctx, "completed", user.ID, time.Now(), []DuplicateGroupCandidate{
		{AssetTags: []string{"ZKEI", "GKEI"}, Reasoning: "maybe dupes"},
	})
	groups, _ := s.ListPendingDuplicateGroups(ctx)
	if len(groups) != 1 {
		t.Fatalf("setup: got %d groups, want 1", len(groups))
	}

	if err := s.DismissDuplicateGroup(ctx, groups[0].ID, user.ID); err != nil {
		t.Fatalf("DismissDuplicateGroup: %v", err)
	}

	after, _ := s.ListPendingDuplicateGroups(ctx)
	if len(after) != 0 {
		t.Fatalf("after dismiss, pending groups = %+v, want none", after)
	}

	// dismissing again (already resolved) is an error
	if err := s.DismissDuplicateGroup(ctx, groups[0].ID, user.ID); err == nil {
		t.Fatal("expected error dismissing an already-resolved group")
	}
}

func TestMergeDuplicateGroup(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	user, _ := s.CreateFirstUser(ctx, "alice", "hash")

	survivor, _ := s.CreateItem(ctx, "ZKEI")
	loser, _ := s.CreateItem(ctx, "GKEI")
	s.AddImage(ctx, survivor.ID, []byte("survivor-1"), "image/jpeg", "S/N 123", user.ID)
	s.AddImage(ctx, loser.ID, []byte("loser-1"), "image/jpeg", "model XR-500", user.ID)
	s.AddImage(ctx, loser.ID, []byte("loser-2"), "image/jpeg", "", user.ID)

	loc, _ := s.GetOrCreateLocation(ctx, "@XYZ", user.ID)

	s.RecordDuplicateRun(ctx, "completed", user.ID, time.Now(), []DuplicateGroupCandidate{
		{AssetTags: []string{"ZKEI", "GKEI"}, Reasoning: "same item, matching S/N"},
	})
	groups, _ := s.ListPendingDuplicateGroups(ctx)
	if len(groups) != 1 {
		t.Fatalf("setup: got %d groups, want 1", len(groups))
	}

	if err := s.MergeDuplicateGroup(ctx, user.ID, groups[0].ID, survivor.ID, &loc.ID); err != nil {
		t.Fatalf("MergeDuplicateGroup: %v", err)
	}

	// loser is gone, asset tag freed for reuse
	if _, err := s.GetItemByID(ctx, loser.ID); err == nil {
		t.Fatal("loser item should have been deleted")
	}
	if _, err := s.CreateItem(ctx, "GKEI"); err != nil {
		t.Fatalf("GKEI should be free for reuse after merge: %v", err)
	}

	// survivor now has all 3 images, contiguous sort_order
	images, err := s.ListImageMetaByItem(ctx, survivor.ID)
	if err != nil {
		t.Fatalf("ListImageMetaByItem: %v", err)
	}
	if len(images) != 3 {
		t.Fatalf("survivor has %d images, want 3", len(images))
	}
	for i, img := range images {
		if img.SortOrder != i {
			t.Errorf("images[%d].SortOrder = %d, want %d (contiguous)", i, img.SortOrder, i)
		}
	}

	// survivor's location was set
	gotSurvivor, err := s.GetItemByID(ctx, survivor.ID)
	if err != nil || gotSurvivor.LocationID == nil || *gotSurvivor.LocationID != loc.ID {
		t.Fatalf("survivor location = %v, %v, want %d", gotSurvivor.LocationID, err, loc.ID)
	}

	// group is no longer pending
	if pending, _ := s.ListPendingDuplicateGroups(ctx); len(pending) != 0 {
		t.Fatalf("pending groups after merge = %+v, want none", pending)
	}

	// activity was logged
	activity, err := s.ListActivityForItem(ctx, survivor.ID)
	if err != nil || len(activity) == 0 || activity[0].Action != "items_merged" {
		t.Fatalf("activity = %+v, %v, want an items_merged entry", activity, err)
	}
}

func TestMergeDuplicateGroupDismissesStaleOverlappingGroups(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	user, _ := s.CreateFirstUser(ctx, "alice", "hash")

	a, _ := s.CreateItem(ctx, "AAAA")
	b, _ := s.CreateItem(ctx, "BBBB")
	c, _ := s.CreateItem(ctx, "CCCC")

	// two overlapping candidate groups from (maybe) different runs: {A,B} and {B,C}
	s.RecordDuplicateRun(ctx, "completed", user.ID, time.Now(), []DuplicateGroupCandidate{
		{AssetTags: []string{"AAAA", "BBBB"}, Reasoning: "group 1"},
	})
	s.RecordDuplicateRun(ctx, "completed", user.ID, time.Now(), []DuplicateGroupCandidate{
		{AssetTags: []string{"BBBB", "CCCC"}, Reasoning: "group 2"},
	})
	groups, err := s.ListPendingDuplicateGroups(ctx)
	if err != nil || len(groups) != 2 {
		t.Fatalf("setup: groups = %+v, %v, want 2", groups, err)
	}

	// resolve group 1 first: merge B into A. B no longer exists, so group 2
	// ({B,C}) is left with only 1 real member and should auto-dismiss.
	var group1ID int64
	for _, g := range groups {
		if len(g.Items) != 2 {
			continue
		}
		for _, m := range g.Items {
			if m.AssetTag == "AAAA" {
				group1ID = g.ID
			}
		}
	}
	if group1ID == 0 {
		t.Fatalf("could not find group 1 in %+v", groups)
	}

	if err := s.MergeDuplicateGroup(ctx, user.ID, group1ID, a.ID, nil); err != nil {
		t.Fatalf("MergeDuplicateGroup: %v", err)
	}

	remaining, err := s.ListPendingDuplicateGroups(ctx)
	if err != nil {
		t.Fatalf("ListPendingDuplicateGroups: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("remaining pending groups = %+v, want none (group 2 should auto-dismiss as stale)", remaining)
	}

	_ = b
	_ = c
}

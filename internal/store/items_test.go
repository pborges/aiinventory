package store

import (
	"context"
	"errors"
	"testing"
)

func TestItemsAndImages(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	user, err := s.CreateFirstUser(ctx, "alice", "hash")
	if err != nil {
		t.Fatalf("CreateFirstUser: %v", err)
	}

	if _, err := s.GetItemByAssetTag(ctx, "ZKEI"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetItemByAssetTag on missing tag = %v, want ErrNotFound", err)
	}

	item, err := s.CreateItem(ctx, "ZKEI")
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	if item.AssetTag != "ZKEI" || item.Description != "" || item.LocationID != nil {
		t.Fatalf("unexpected item: %+v", item)
	}

	if _, err := s.CreateItem(ctx, "ZKEI"); !errors.Is(err, ErrAssetTagTaken) {
		t.Fatalf("duplicate CreateItem = %v, want ErrAssetTagTaken", err)
	}

	img1, err := s.AddImage(ctx, item.ID, []byte("fake-jpeg-1"), "image/jpeg", "S/N 111", user.ID)
	if err != nil {
		t.Fatalf("AddImage 1: %v", err)
	}
	if img1.SortOrder != 0 {
		t.Fatalf("first image sort_order = %d, want 0", img1.SortOrder)
	}

	img2, err := s.AddImage(ctx, item.ID, []byte("fake-jpeg-2"), "image/jpeg", "model XYZ", user.ID)
	if err != nil {
		t.Fatalf("AddImage 2: %v", err)
	}
	if img2.SortOrder != 1 {
		t.Fatalf("second image sort_order = %d, want 1", img2.SortOrder)
	}

	images, err := s.ListImagesByItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("ListImagesByItem: %v", err)
	}
	if len(images) != 2 || images[0].ID != img1.ID || images[1].ID != img2.ID {
		t.Fatalf("ListImagesByItem order wrong: %+v", images)
	}

	if err := s.UpdateItemDescription(ctx, item.ID, "a cordless drill"); err != nil {
		t.Fatalf("UpdateItemDescription: %v", err)
	}
	got, err := s.GetItemByID(ctx, item.ID)
	if err != nil || got.Description != "a cordless drill" {
		t.Fatalf("GetItemByID after update = %+v, %v", got, err)
	}

	// deleting an item cascades to its images (hard delete, per README)
	if err := s.DeleteItem(ctx, item.ID); err != nil {
		t.Fatalf("DeleteItem: %v", err)
	}
	if _, err := s.GetItemByID(ctx, item.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetItemByID after delete = %v, want ErrNotFound", err)
	}
	if images, err := s.ListImagesByItem(ctx, item.ID); err != nil || len(images) != 0 {
		t.Fatalf("images after item delete = %+v, %v, want none (cascade)", images, err)
	}

	// the asset tag is free for reuse after a hard delete
	if _, err := s.CreateItem(ctx, "ZKEI"); err != nil {
		t.Fatalf("recreate item with freed asset tag: %v", err)
	}
}

func TestActivityLog(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	user, err := s.CreateFirstUser(ctx, "alice", "hash")
	if err != nil {
		t.Fatalf("CreateFirstUser: %v", err)
	}
	item, err := s.CreateItem(ctx, "ZKEI")
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}

	if err := s.LogActivity(ctx, user.ID, "item_created", &item.ID, nil, ""); err != nil {
		t.Fatalf("LogActivity: %v", err)
	}

	entries, err := s.ListActivityForItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("ListActivityForItem: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d activity entries, want 1", len(entries))
	}
	if entries[0].Username != "alice" || entries[0].Action != "item_created" {
		t.Fatalf("unexpected activity entry: %+v", entries[0])
	}
}

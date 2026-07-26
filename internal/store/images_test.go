package store

import (
	"context"
	"errors"
	"testing"
)

func TestReorderImages(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	user, _ := s.CreateFirstUser(ctx, "alice", "hash")
	item, _ := s.CreateItem(ctx, "ZKEI")

	img1, _ := s.AddImage(ctx, item.ID, []byte("1"), "image/jpeg", "", user.ID)
	img2, _ := s.AddImage(ctx, item.ID, []byte("2"), "image/jpeg", "", user.ID)
	img3, _ := s.AddImage(ctx, item.ID, []byte("3"), "image/jpeg", "", user.ID)

	// reverse the order: img3 becomes primary
	if err := s.ReorderImages(ctx, item.ID, []int64{img3.ID, img1.ID, img2.ID}); err != nil {
		t.Fatalf("ReorderImages: %v", err)
	}

	images, err := s.ListImageMetaByItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("ListImageMetaByItem: %v", err)
	}
	if len(images) != 3 {
		t.Fatalf("got %d images, want 3", len(images))
	}
	if images[0].ID != img3.ID || images[1].ID != img1.ID || images[2].ID != img2.ID {
		t.Fatalf("order after reorder = [%d %d %d], want [%d %d %d]",
			images[0].ID, images[1].ID, images[2].ID, img3.ID, img1.ID, img2.ID)
	}
	if images[0].Data != nil {
		t.Error("ListImageMetaByItem should not populate blob Data")
	}
}

func TestReorderImagesRejectsForeignImage(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	user, _ := s.CreateFirstUser(ctx, "alice", "hash")
	item1, _ := s.CreateItem(ctx, "ZKEI")
	item2, _ := s.CreateItem(ctx, "GKEI")

	img1, _ := s.AddImage(ctx, item1.ID, []byte("1"), "image/jpeg", "", user.ID)
	otherImg, _ := s.AddImage(ctx, item2.ID, []byte("2"), "image/jpeg", "", user.ID)

	if err := s.ReorderImages(ctx, item1.ID, []int64{img1.ID, otherImg.ID}); err == nil {
		t.Fatal("expected an error reordering an image that belongs to a different item")
	}
}

func TestDeleteImage(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	user, _ := s.CreateFirstUser(ctx, "alice", "hash")
	item, _ := s.CreateItem(ctx, "ZKEI")

	img1, _ := s.AddImage(ctx, item.ID, []byte("1"), "image/jpeg", "", user.ID)
	img2, _ := s.AddImage(ctx, item.ID, []byte("2"), "image/jpeg", "", user.ID)

	if err := s.DeleteImage(ctx, item.ID, img1.ID); err != nil {
		t.Fatalf("DeleteImage: %v", err)
	}

	images, err := s.ListImageMetaByItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("ListImageMetaByItem: %v", err)
	}
	if len(images) != 1 || images[0].ID != img2.ID {
		t.Fatalf("images after delete = %+v, want just [%d]", images, img2.ID)
	}

	// deleting again is a no-op error, not a panic
	if err := s.DeleteImage(ctx, item.ID, img1.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete already-deleted image = %v, want ErrNotFound", err)
	}
}

func TestDeleteImageRejectsForeignImage(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	user, _ := s.CreateFirstUser(ctx, "alice", "hash")
	item1, _ := s.CreateItem(ctx, "ZKEI")
	item2, _ := s.CreateItem(ctx, "GKEI")

	otherImg, _ := s.AddImage(ctx, item2.ID, []byte("2"), "image/jpeg", "", user.ID)

	if err := s.DeleteImage(ctx, item1.ID, otherImg.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete image belonging to a different item = %v, want ErrNotFound", err)
	}

	// confirm it wasn't actually deleted
	images, _ := s.ListImageMetaByItem(ctx, item2.ID)
	if len(images) != 1 {
		t.Fatalf("image should not have been deleted: %+v", images)
	}
}

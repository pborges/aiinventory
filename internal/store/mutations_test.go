package store

import (
	"context"
	"errors"
	"testing"
)

func TestDeleteItemsWithActivityRollsBackEntireBatch(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	user, _ := s.CreateFirstUser(ctx, "alice", "hash")
	first, _ := s.CreateItem(ctx, "ZKEI")
	second, _ := s.CreateItem(ctx, "GKEI")

	err := s.DeleteItemsWithActivity(ctx, user.ID, []int64{first.ID, 999999})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteItemsWithActivity error = %v, want ErrNotFound", err)
	}
	if _, err := s.GetItemByID(ctx, first.ID); err != nil {
		t.Fatalf("first item was partially deleted: %v", err)
	}
	if _, err := s.GetItemByID(ctx, second.ID); err != nil {
		t.Fatalf("second item changed: %v", err)
	}
	activity, err := s.ListActivityForItem(ctx, first.ID)
	if err != nil || len(activity) != 0 {
		t.Fatalf("activity was partially committed: %+v, %v", activity, err)
	}
}

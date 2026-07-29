package store

import (
	"testing"

	"github.com/pborges/aiinventory/internal/domain"
)

func TestBulkRegisterAssetTagsIdempotent(t *testing.T) {
	ctx := t.Context()
	s := NewTestStore(t)

	added, skipped, err := s.BulkRegisterAssetTags(ctx, []string{"AAAA", "BBBB"})
	if err != nil {
		t.Fatalf("BulkRegisterAssetTags: %v", err)
	}
	if added != 2 || skipped != 0 {
		t.Fatalf("first call added=%d skipped=%d, want added=2 skipped=0", added, skipped)
	}

	added, skipped, err = s.BulkRegisterAssetTags(ctx, []string{"AAAA", "BBBB", "CCCC"})
	if err != nil {
		t.Fatalf("BulkRegisterAssetTags second call: %v", err)
	}
	if added != 1 || skipped != 2 {
		t.Fatalf("second call added=%d skipped=%d, want added=1 skipped=2", added, skipped)
	}

	tags, err := s.ListRegisteredAssetTags(ctx)
	if err != nil {
		t.Fatalf("ListRegisteredAssetTags: %v", err)
	}
	want := []string{"AAAA", "BBBB", "CCCC"}
	if len(tags) != len(want) {
		t.Fatalf("ListRegisteredAssetTags = %v, want %v", tags, want)
	}
	for i, tag := range want {
		if tags[i] != tag {
			t.Fatalf("ListRegisteredAssetTags = %v, want %v", tags, want)
		}
	}

	count, err := s.CountRegisteredAssetTags(ctx)
	if err != nil {
		t.Fatalf("CountRegisteredAssetTags: %v", err)
	}
	if count != 3 {
		t.Fatalf("CountRegisteredAssetTags = %d, want 3", count)
	}
}

func TestRegisterAssetTagNoOpIfAlreadyRegistered(t *testing.T) {
	ctx := t.Context()
	s := NewTestStore(t)

	if err := s.RegisterAssetTag(ctx, "ZKEI"); err != nil {
		t.Fatalf("RegisterAssetTag: %v", err)
	}
	if err := s.RegisterAssetTag(ctx, "ZKEI"); err != nil {
		t.Fatalf("RegisterAssetTag second call: %v", err)
	}

	count, err := s.CountRegisteredAssetTags(ctx)
	if err != nil {
		t.Fatalf("CountRegisteredAssetTags: %v", err)
	}
	if count != 1 {
		t.Fatalf("CountRegisteredAssetTags = %d, want 1", count)
	}
}

func TestApplyReconciliationRegistersNewTags(t *testing.T) {
	ctx := t.Context()
	s := NewTestStore(t)
	user, _ := s.CreateFirstUser(ctx, "alice", "hash")

	diff := domain.ReconcileDiff{LocationTag: "@XYZ", New: []string{"ZKEI"}}
	if err := s.ApplyReconciliation(ctx, user.ID, diff); err != nil {
		t.Fatalf("ApplyReconciliation: %v", err)
	}

	tags, err := s.ListRegisteredAssetTags(ctx)
	if err != nil {
		t.Fatalf("ListRegisteredAssetTags: %v", err)
	}
	if len(tags) != 1 || tags[0] != "ZKEI" {
		t.Fatalf("ListRegisteredAssetTags = %v, want [ZKEI]", tags)
	}
}

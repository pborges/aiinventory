package store

import (
	"testing"

	"github.com/pborges/aiinventory/internal/domain"
)

func TestBulkRegisterLocationTagsIdempotent(t *testing.T) {
	ctx := t.Context()
	s := NewTestStore(t)

	added, skipped, err := s.BulkRegisterLocationTags(ctx, []string{"@AAA", "@BBB"})
	if err != nil {
		t.Fatalf("BulkRegisterLocationTags: %v", err)
	}
	if added != 2 || skipped != 0 {
		t.Fatalf("first call added=%d skipped=%d, want added=2 skipped=0", added, skipped)
	}

	added, skipped, err = s.BulkRegisterLocationTags(ctx, []string{"@AAA", "@BBB", "@CCC"})
	if err != nil {
		t.Fatalf("BulkRegisterLocationTags second call: %v", err)
	}
	if added != 1 || skipped != 2 {
		t.Fatalf("second call added=%d skipped=%d, want added=1 skipped=2", added, skipped)
	}

	tags, err := s.ListRegisteredLocationTags(ctx)
	if err != nil {
		t.Fatalf("ListRegisteredLocationTags: %v", err)
	}
	want := []string{"@AAA", "@BBB", "@CCC"}
	if len(tags) != len(want) {
		t.Fatalf("ListRegisteredLocationTags = %v, want %v", tags, want)
	}
	for i, tag := range want {
		if tags[i] != tag {
			t.Fatalf("ListRegisteredLocationTags = %v, want %v", tags, want)
		}
	}

	count, err := s.CountRegisteredLocationTags(ctx)
	if err != nil {
		t.Fatalf("CountRegisteredLocationTags: %v", err)
	}
	if count != 3 {
		t.Fatalf("CountRegisteredLocationTags = %d, want 3", count)
	}
}

func TestRegisterLocationTagNoOpIfAlreadyRegistered(t *testing.T) {
	ctx := t.Context()
	s := NewTestStore(t)

	if err := s.RegisterLocationTag(ctx, "@XYZ"); err != nil {
		t.Fatalf("RegisterLocationTag: %v", err)
	}
	if err := s.RegisterLocationTag(ctx, "@XYZ"); err != nil {
		t.Fatalf("RegisterLocationTag second call: %v", err)
	}

	count, err := s.CountRegisteredLocationTags(ctx)
	if err != nil {
		t.Fatalf("CountRegisteredLocationTags: %v", err)
	}
	if count != 1 {
		t.Fatalf("CountRegisteredLocationTags = %d, want 1", count)
	}
}

func TestGetOrCreateLocationRegistersTag(t *testing.T) {
	ctx := t.Context()
	s := NewTestStore(t)
	user, _ := s.CreateFirstUser(ctx, "alice", "hash")

	if _, err := s.GetOrCreateLocation(ctx, "@XYZ", user.ID); err != nil {
		t.Fatalf("GetOrCreateLocation: %v", err)
	}
	// second call, same tag — should stay a no-op, not error
	if _, err := s.GetOrCreateLocation(ctx, "@XYZ", user.ID); err != nil {
		t.Fatalf("GetOrCreateLocation second call: %v", err)
	}

	tags, err := s.ListRegisteredLocationTags(ctx)
	if err != nil {
		t.Fatalf("ListRegisteredLocationTags: %v", err)
	}
	if len(tags) != 1 || tags[0] != "@XYZ" {
		t.Fatalf("ListRegisteredLocationTags = %v, want [@XYZ]", tags)
	}
}

func TestApplyReconciliationRegistersLocationTag(t *testing.T) {
	ctx := t.Context()
	s := NewTestStore(t)
	user, _ := s.CreateFirstUser(ctx, "alice", "hash")

	diff := domain.ReconcileDiff{LocationTag: "@XYZ", New: []string{"ZKEI"}}
	if err := s.ApplyReconciliation(ctx, user.ID, diff); err != nil {
		t.Fatalf("ApplyReconciliation: %v", err)
	}

	tags, err := s.ListRegisteredLocationTags(ctx)
	if err != nil {
		t.Fatalf("ListRegisteredLocationTags: %v", err)
	}
	if len(tags) != 1 || tags[0] != "@XYZ" {
		t.Fatalf("ListRegisteredLocationTags = %v, want [@XYZ]", tags)
	}
}

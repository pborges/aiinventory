package store

import "testing"

func TestListAllKnownAssetTagsUnionsRegistryAndItems(t *testing.T) {
	ctx := t.Context()
	s := NewTestStore(t)

	if _, _, err := s.BulkRegisterAssetTags(ctx, []string{"AAAA", "BBBB"}); err != nil {
		t.Fatalf("BulkRegisterAssetTags: %v", err)
	}
	// BBBB also gets a real item — the union must dedupe it, not double it.
	if _, err := s.CreateItem(ctx, "BBBB"); err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	if _, err := s.CreateItem(ctx, "CCCC"); err != nil {
		t.Fatalf("CreateItem: %v", err)
	}

	got, err := s.ListAllKnownAssetTags(ctx)
	if err != nil {
		t.Fatalf("ListAllKnownAssetTags: %v", err)
	}
	want := []string{"AAAA", "BBBB", "CCCC"}
	if len(got) != len(want) {
		t.Fatalf("ListAllKnownAssetTags = %v, want %v", got, want)
	}
	for i, tag := range want {
		if got[i] != tag {
			t.Fatalf("ListAllKnownAssetTags = %v, want %v", got, want)
		}
	}
}

func TestListAllKnownLocationTagsUnionsRegistryAndLocations(t *testing.T) {
	ctx := t.Context()
	s := NewTestStore(t)
	user, _ := s.CreateFirstUser(ctx, "alice", "hash")

	if _, _, err := s.BulkRegisterLocationTags(ctx, []string{"@AAA", "@BBB"}); err != nil {
		t.Fatalf("BulkRegisterLocationTags: %v", err)
	}
	// @BBB also gets a real locations row — the union must dedupe it.
	if _, err := s.GetOrCreateLocation(ctx, "@BBB", user.ID); err != nil {
		t.Fatalf("GetOrCreateLocation: %v", err)
	}
	if _, err := s.GetOrCreateLocation(ctx, "@CCC", user.ID); err != nil {
		t.Fatalf("GetOrCreateLocation: %v", err)
	}

	got, err := s.ListAllKnownLocationTags(ctx)
	if err != nil {
		t.Fatalf("ListAllKnownLocationTags: %v", err)
	}
	want := []string{"@AAA", "@BBB", "@CCC"}
	if len(got) != len(want) {
		t.Fatalf("ListAllKnownLocationTags = %v, want %v", got, want)
	}
	for i, tag := range want {
		if got[i] != tag {
			t.Fatalf("ListAllKnownLocationTags = %v, want %v", got, want)
		}
	}
}

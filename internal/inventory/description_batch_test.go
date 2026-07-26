package inventory

import (
	"context"
	"errors"
	"testing"

	"github.com/pborges/aiinventory/internal/domain"
	"github.com/pborges/aiinventory/internal/gemini"
)

func TestDescriptionBatchTryStartRejectsConcurrent(t *testing.T) {
	b := &DescriptionBatch{}
	if !b.TryStart([]DescriptionBatchRequest{{ItemID: 1}}, nil) {
		t.Fatal("first TryStart should succeed")
	}
	if b.TryStart([]DescriptionBatchRequest{{ItemID: 2}}, nil) {
		t.Fatal("second TryStart should fail while a batch is running")
	}
	b.finish()
	if !b.TryStart([]DescriptionBatchRequest{{ItemID: 3}}, nil) {
		t.Fatal("TryStart should succeed again after finish")
	}
}

func TestRunDescriptionBatchUpdatesProgressAndFrees(t *testing.T) {
	f := &fakeDescriptionStore{
		items: map[int64]domain.Item{
			1: {ID: 1, AssetTag: "ZKEI"},
			2: {ID: 2, AssetTag: "GKEI"},
		},
		images: map[int64][]domain.Image{
			1: {{ID: 10, Description: "S/N 1"}},
			2: {{ID: 20, Description: "S/N 2"}},
		},
	}
	fake := &gemini.Fake{
		DescriptionFunc: func(assetTag string, _ []string, _ string) (gemini.DescriptionResult, error) {
			if assetTag == "GKEI" {
				return gemini.DescriptionResult{}, errors.New("gemini unavailable for this item")
			}
			return gemini.DescriptionResult{Description: "described " + assetTag}, nil
		},
	}

	b := &DescriptionBatch{}
	if !b.TryStart([]DescriptionBatchRequest{{ItemID: 1, Hint: "hint1"}, {ItemID: 2}}, map[int64]string{1: "ZKEI", 2: "GKEI"}) {
		t.Fatal("TryStart should succeed")
	}

	RunDescriptionBatch(context.Background(), f, fake, b, 42, "model", "prompt")

	running, items := b.Snapshot()
	if running {
		t.Error("batch should no longer be running after RunDescriptionBatch returns")
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}

	byID := map[int64]BatchItem{items[0].ItemID: items[0], items[1].ItemID: items[1]}
	if byID[1].Status != BatchItemDone || byID[1].Description != "described ZKEI" {
		t.Errorf("item 1 = %+v", byID[1])
	}
	if byID[2].Status != BatchItemError || byID[2].Error == "" {
		t.Errorf("item 2 = %+v", byID[2])
	}
	if f.updatedIDs[1] != "described ZKEI" {
		t.Errorf("item 1 description not persisted: %v", f.updatedIDs)
	}
	if _, ok := f.updatedIDs[2]; ok {
		t.Errorf("item 2 should not have been persisted after a gemini error: %v", f.updatedIDs)
	}
}

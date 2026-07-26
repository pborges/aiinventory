package inventory

import (
	"context"
	"testing"

	"github.com/pborges/aiinventory/internal/domain"
	"github.com/pborges/aiinventory/internal/gemini"
	"github.com/pborges/aiinventory/internal/store"
)

type fakeDescriptionStore struct {
	items      map[int64]domain.Item
	images     map[int64][]domain.Image
	updatedIDs map[int64]string
	activity   []loggedActivity
}

func (f *fakeDescriptionStore) GetItemByID(_ context.Context, id int64) (domain.Item, error) {
	it, ok := f.items[id]
	if !ok {
		return domain.Item{}, store.ErrNotFound
	}
	return it, nil
}

func (f *fakeDescriptionStore) ListImageMetaByItem(_ context.Context, itemID int64) ([]domain.Image, error) {
	return f.images[itemID], nil
}

func (f *fakeDescriptionStore) UpdateItemDescription(_ context.Context, id int64, description string) error {
	if f.updatedIDs == nil {
		f.updatedIDs = map[int64]string{}
	}
	f.updatedIDs[id] = description
	return nil
}

func (f *fakeDescriptionStore) LogActivity(_ context.Context, userID int64, action domain.ActivityAction, itemID, _ *int64, _ string) error {
	f.activity = append(f.activity, loggedActivity{userID: userID, action: action, itemID: itemID})
	return nil
}

func TestRegenerateDescription(t *testing.T) {
	f := &fakeDescriptionStore{
		items: map[int64]domain.Item{1: {ID: 1, AssetTag: "ZKEI"}},
		images: map[int64][]domain.Image{
			1: {
				{ID: 10, Description: "S/N 12345"},
				{ID: 11, Description: "model XR-500"},
				{ID: 12, Description: ""}, // no note yet — should be excluded, not passed as an empty string
			},
		},
	}
	fake := &gemini.Fake{DescriptionResult: gemini.DescriptionResult{Description: "cordless drill, S/N 12345, model XR-500"}}

	desc, err := RegenerateDescription(context.Background(), f, fake, 42, "gemini-2.5-flash", "prompt text", 1)
	if err != nil {
		t.Fatalf("RegenerateDescription: %v", err)
	}
	if desc != "cordless drill, S/N 12345, model XR-500" {
		t.Errorf("desc = %q", desc)
	}
	if f.updatedIDs[1] != desc {
		t.Errorf("UpdateItemDescription not called with the result: %+v", f.updatedIDs)
	}
	if len(f.activity) != 1 || f.activity[0].action != domain.ActivityDescriptionRegenerated {
		t.Errorf("activity = %+v", f.activity)
	}
}

func TestRegenerateDescriptionGeminiError(t *testing.T) {
	f := &fakeDescriptionStore{items: map[int64]domain.Item{1: {ID: 1, AssetTag: "ZKEI"}}}
	fake := &gemini.Fake{DescriptionErr: context.DeadlineExceeded}

	if _, err := RegenerateDescription(context.Background(), f, fake, 42, "model", "prompt", 1); err == nil {
		t.Fatal("expected an error when Gemini fails")
	}
	if len(f.updatedIDs) != 0 || len(f.activity) != 0 {
		t.Fatalf("nothing should be written when Gemini fails: updated=%v activity=%v", f.updatedIDs, f.activity)
	}
}

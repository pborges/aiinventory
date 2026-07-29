package inventory

import (
	"context"
	"errors"
	"testing"

	"github.com/pborges/aiinventory/internal/domain"
	"github.com/pborges/aiinventory/internal/store"
)

// fakeCaptureStore is a minimal in-memory implementation of CaptureStore,
// hand-rolled per the project's "no mocking library" testing convention.
type fakeCaptureStore struct {
	itemsByTag    map[string]domain.Item
	nextItemID    int64
	images        []domain.Image
	nextImgID     int64
	activity      []loggedActivity
	registeredTag []string
}

type loggedActivity struct {
	userID int64
	action domain.ActivityAction
	itemID *int64
}

func newFakeCaptureStore() *fakeCaptureStore {
	return &fakeCaptureStore{itemsByTag: map[string]domain.Item{}}
}

func (f *fakeCaptureStore) GetItemByAssetTag(_ context.Context, tag string) (domain.Item, error) {
	it, ok := f.itemsByTag[tag]
	if !ok {
		return domain.Item{}, store.ErrNotFound
	}
	return it, nil
}

func (f *fakeCaptureStore) CreateItem(_ context.Context, tag string) (domain.Item, error) {
	f.nextItemID++
	it := domain.Item{ID: f.nextItemID, AssetTag: tag}
	f.itemsByTag[tag] = it
	return it, nil
}

func (f *fakeCaptureStore) AddImage(_ context.Context, itemID int64, data []byte, contentType, description string, createdBy int64) (domain.Image, error) {
	f.nextImgID++
	img := domain.Image{ID: f.nextImgID, ItemID: itemID, Data: data, ContentType: contentType, Description: description, CreatedBy: createdBy}
	f.images = append(f.images, img)
	return img, nil
}

func (f *fakeCaptureStore) LogActivity(_ context.Context, userID int64, action domain.ActivityAction, itemID, _ *int64, _ string) error {
	f.activity = append(f.activity, loggedActivity{userID: userID, action: action, itemID: itemID})
	return nil
}

func (f *fakeCaptureStore) RegisterAssetTag(_ context.Context, tag string) error {
	f.registeredTag = append(f.registeredTag, tag)
	return nil
}

func TestCaptureCreatesNewItem(t *testing.T) {
	s := newFakeCaptureStore()
	res, err := Capture(context.Background(), s, 1, true, "ZKEI", []byte("jpeg"), "image/jpeg", "S/N 123")
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if !res.ItemWasNew {
		t.Error("ItemWasNew = false, want true for a brand-new tag")
	}
	if res.Item.AssetTag != "ZKEI" {
		t.Errorf("AssetTag = %q, want ZKEI", res.Item.AssetTag)
	}
	if len(s.activity) != 1 || s.activity[0].action != domain.ActivityItemCreated {
		t.Fatalf("activity log = %+v, want one item_created entry", s.activity)
	}
}

func TestCaptureAppendsToExistingItem(t *testing.T) {
	s := newFakeCaptureStore()
	first, err := Capture(context.Background(), s, 1, true, "ZKEI", []byte("jpeg1"), "image/jpeg", "S/N 123")
	if err != nil {
		t.Fatalf("first Capture: %v", err)
	}

	second, err := Capture(context.Background(), s, 1, true, "ZKEI", []byte("jpeg2"), "image/jpeg", "model XYZ")
	if err != nil {
		t.Fatalf("second Capture: %v", err)
	}

	if second.ItemWasNew {
		t.Error("ItemWasNew = true on second capture of the same tag, want false")
	}
	if second.Item.ID != first.Item.ID {
		t.Errorf("second capture created a different item: %d != %d", second.Item.ID, first.Item.ID)
	}
	if len(s.images) != 2 {
		t.Fatalf("got %d images, want 2", len(s.images))
	}
	if s.activity[1].action != domain.ActivityImageAdded {
		t.Errorf("second activity action = %q, want image_added", s.activity[1].action)
	}
}

func TestCaptureSelfHealsRegistry(t *testing.T) {
	s := newFakeCaptureStore()
	if _, err := Capture(context.Background(), s, 1, true, "ZKEI", []byte("jpeg"), "image/jpeg", "S/N 123"); err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if len(s.registeredTag) != 1 || s.registeredTag[0] != "ZKEI" {
		t.Fatalf("registeredTag = %v, want [ZKEI]", s.registeredTag)
	}
}

func TestCaptureRejectsMissingAssetTag(t *testing.T) {
	s := newFakeCaptureStore()
	_, err := Capture(context.Background(), s, 1, false, "", []byte("jpeg"), "image/jpeg", "")
	if !errors.Is(err, ErrNoAssetTag) {
		t.Fatalf("err = %v, want ErrNoAssetTag", err)
	}
	if len(s.images) != 0 || len(s.activity) != 0 {
		t.Fatalf("no item/image/activity should be created when there's no asset tag: images=%v activity=%v", s.images, s.activity)
	}
}

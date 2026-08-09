package tagsheet

import (
	"math"
	"testing"
)

func approxEqual(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestLayoutSheetDimensionsAndGrid(t *testing.T) {
	codes := make([]string, 32)
	for i := range codes {
		codes[i] = "AAAA"
	}
	sheet, err := Layout(KindAsset, codes, 8, 4, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !approxEqual(sheet.WidthMm, 260, 1e-9) {
		t.Errorf("WidthMm = %v, want 260", sheet.WidthMm)
	}
	if !approxEqual(sheet.HeightMm, 244, 1e-9) {
		t.Errorf("HeightMm = %v, want 244", sheet.HeightMm)
	}
	if len(sheet.Tags) != 32 {
		t.Fatalf("got %d tags, want 32", len(sheet.Tags))
	}

	// Row-major order: tag[5] is row 1, col 1 (0-indexed).
	tag5 := sheet.Tags[5]
	if !approxEqual(tag5.X, 68, 1e-9) || !approxEqual(tag5.Y, 34, 1e-9) {
		t.Errorf("tag[5] origin = (%v, %v), want (68, 34)", tag5.X, tag5.Y)
	}
}

func TestLayoutRejectsCodeCountMismatch(t *testing.T) {
	if _, err := Layout(KindAsset, []string{"AAAA"}, 2, 2, 4); err == nil {
		t.Fatal("expected an error when len(codes) != rows*cols")
	}
}

func TestLayoutAssetTextPlacement(t *testing.T) {
	sheet, err := Layout(KindAsset, []string{"WXYZ"}, 1, 1, 4)
	if err != nil {
		t.Fatal(err)
	}
	tag := sheet.Tags[0]
	box := bboxOf(tag.Text...)

	if !approxEqual(box.Min.X, tag.X+assetTextLeftMarginMm, 0.05) {
		t.Errorf("text min X = %v, want ≈ %v", box.Min.X, tag.X+assetTextLeftMarginMm)
	}
	wantCenterY := tag.Y + TagHeightMm/2
	if !approxEqual(box.centerY(), wantCenterY, 0.05) {
		t.Errorf("text center Y = %v, want ≈ %v", box.centerY(), wantCenterY)
	}
	if box.Min.X < tag.X-0.01 || box.Max.X > tag.X+TagWidthMm+0.01 {
		t.Errorf("text X extent [%v, %v] falls outside tag width [%v, %v]", box.Min.X, box.Max.X, tag.X, tag.X+TagWidthMm)
	}
	if box.Min.Y < tag.Y-0.01 || box.Max.Y > tag.Y+TagHeightMm+0.01 {
		t.Errorf("text Y extent [%v, %v] falls outside tag height [%v, %v]", box.Min.Y, box.Max.Y, tag.Y, tag.Y+TagHeightMm)
	}
}

func TestLayoutLocationTextPlacement(t *testing.T) {
	sheet, err := Layout(KindLocation, []string{"@ABC"}, 1, 1, 4)
	if err != nil {
		t.Fatal(err)
	}
	tag := sheet.Tags[0]
	box := bboxOf(tag.Text...)

	wantCenterX := tag.X + TagWidthMm/2
	wantCenterY := tag.Y + TagHeightMm/2
	if !approxEqual(box.centerX(), wantCenterX, 0.01) {
		t.Errorf("text center X = %v, want ≈ %v", box.centerX(), wantCenterX)
	}
	if !approxEqual(box.centerY(), wantCenterY, 0.01) {
		t.Errorf("text center Y = %v, want ≈ %v", box.centerY(), wantCenterY)
	}

	inkWidth := box.Max.X - box.Min.X
	if inkWidth < 40 || inkWidth > 60 {
		t.Errorf("location text ink width = %v, want roughly 40-60mm on a 60mm tag", inkWidth)
	}
}

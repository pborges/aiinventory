package tagsheet

import (
	"testing"

	"golang.org/x/image/font/sfnt"
)

func TestLoadOCRFontUnitsPerEm(t *testing.T) {
	f, err := sfnt.Parse(ocrbTTF)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.UnitsPerEm(); got != 1024 {
		t.Errorf("UnitsPerEm() = %d, want 1024", got)
	}
}

func TestLoadOCRFontGlyphs(t *testing.T) {
	f, err := loadOCRFont()
	if err != nil {
		t.Fatal(err)
	}

	if got := f.glyphs['A'].advance; got != 532 {
		t.Errorf("advance('A') = %v, want 532", got)
	}
	if f.capHeight <= 0 {
		t.Errorf("capHeight = %v, want > 0", f.capHeight)
	}

	for _, r := range ocrGlyphs {
		g, ok := f.glyphs[r]
		if !ok {
			t.Errorf("missing glyph %q", r)
			continue
		}
		if len(g.path) == 0 {
			t.Errorf("glyph %q has an empty path", r)
			continue
		}
		if g.path[0].Op != OpMove {
			t.Errorf("glyph %q path does not start with OpMove", r)
		}
		for _, seg := range g.path {
			if seg.Op == OpCube {
				t.Errorf("glyph %q contains an OpCube segment; ocrb.ttf is expected to be quadratic-curve-only", r)
			}
		}
	}
}

func TestLoadOCRFontCached(t *testing.T) {
	f1, err := loadOCRFont()
	if err != nil {
		t.Fatal(err)
	}
	f2, err := loadOCRFont()
	if err != nil {
		t.Fatal(err)
	}
	if f1 != f2 {
		t.Error("loadOCRFont() should return the same cached instance on repeated calls")
	}
}

package tagsheet

import (
	_ "embed"
	"fmt"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

//go:embed ocrb.ttf
var ocrbTTF []byte

// glyph is one character's outline (font units, y-down, baseline at y=0)
// and pen advance (also font units) — everything layoutTextMm needs to
// place a run of text.
type glyph struct {
	path    Path
	advance float64
}

// ocrFont is the parsed OCR-B font, pre-extracted down to just the glyphs
// tagsheet ever renders (A-Z and @), plus two measured reference heights
// used to turn a target mm size into a font-unit scale factor: capHeight
// (from 'X') for the letters, atHeight (the '@' glyph's own full ink
// height) for '@'. '@' isn't a cap-height glyph — in this font its own ink
// is only ~80% of a capital's height — so scaling it by capHeight the same
// way as a letter renders it visibly smaller than the letters beside it;
// atHeight lets layoutLocationText target '@'s own ink height instead, so
// "@XYZ" reads as one uniform size the way the reference design does.
type ocrFont struct {
	capHeight float64
	atHeight  float64
	glyphs    map[rune]glyph
}

// loadOCRFont parses the embedded font once and caches the result — safe
// for concurrent use, since every caller (SVG/lbrn2 preview requests) just
// reads the immutable result.
var loadOCRFont = sync.OnceValues(func() (*ocrFont, error) {
	return parseOCRFont(ocrbTTF)
})

// ocrGlyphs is every character tagsheet needs from the font: the 26
// uppercase letters for asset/location codes, plus '@' for location tags.
const ocrGlyphs = "ABCDEFGHIJKLMNOPQRSTUVWXYZ@"

func parseOCRFont(data []byte) (*ocrFont, error) {
	f, err := sfnt.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parse ocrb.ttf: %w", err)
	}

	// Pass ppem = unitsPerEm so LoadGlyph/GlyphAdvance apply a 1:1 scale —
	// the returned 26.6 fixed-point coordinates are then exactly the raw
	// font-unit values (divide by 64), letting layoutTextMm do its own
	// mm-per-unit scaling from a single measured cap height.
	ppem := fixed.I(int(f.UnitsPerEm()))
	var buf sfnt.Buffer

	glyphs := make(map[rune]glyph, len(ocrGlyphs))
	for _, r := range ocrGlyphs {
		gi, err := f.GlyphIndex(&buf, r)
		if err != nil {
			return nil, fmt.Errorf("glyph index %q: %w", r, err)
		}
		if gi == 0 {
			return nil, fmt.Errorf("glyph %q not found in ocrb.ttf", r)
		}
		segs, err := f.LoadGlyph(&buf, gi, ppem, nil)
		if err != nil {
			return nil, fmt.Errorf("load glyph %q: %w", r, err)
		}
		// segs becomes invalid the next time buf is reused (next loop
		// iteration), so copy it out into our own Path now.
		path := pathFromSegments(segs)

		adv, err := f.GlyphAdvance(&buf, gi, ppem, font.HintingNone)
		if err != nil {
			return nil, fmt.Errorf("glyph advance %q: %w", r, err)
		}
		glyphs[r] = glyph{path: path, advance: fixed26_6ToFloat(adv)}
	}

	capHeight := -bboxOf(glyphs['X'].path).Min.Y
	if capHeight <= 0 {
		return nil, fmt.Errorf("measured non-positive cap height (%v) from ocrb.ttf 'X' glyph", capHeight)
	}

	atBox := bboxOf(glyphs['@'].path)
	atHeight := atBox.Max.Y - atBox.Min.Y
	if atHeight <= 0 {
		return nil, fmt.Errorf("measured non-positive '@' ink height (%v) from ocrb.ttf", atHeight)
	}

	return &ocrFont{capHeight: capHeight, atHeight: atHeight, glyphs: glyphs}, nil
}

// pathFromSegments converts sfnt's Segments (fixed-point, TrueType
// quadratic curves only) into our own Path of float64 mm-agnostic font
// units — copied eagerly since Segments is only valid until the next
// sfnt.Buffer reuse.
func pathFromSegments(segs sfnt.Segments) Path {
	path := make(Path, 0, len(segs))
	for _, seg := range segs {
		switch seg.Op {
		case sfnt.SegmentOpMoveTo:
			path = append(path, Seg{Op: OpMove, Pts: [3]Point{pointFromFixed(seg.Args[0])}})
		case sfnt.SegmentOpLineTo:
			path = append(path, Seg{Op: OpLine, Pts: [3]Point{pointFromFixed(seg.Args[0])}})
		case sfnt.SegmentOpQuadTo:
			path = append(path, Seg{Op: OpQuad, Pts: [3]Point{
				pointFromFixed(seg.Args[0]),
				pointFromFixed(seg.Args[1]),
			}})
		case sfnt.SegmentOpCubeTo:
			path = append(path, Seg{Op: OpCube, Pts: [3]Point{
				pointFromFixed(seg.Args[0]),
				pointFromFixed(seg.Args[1]),
				pointFromFixed(seg.Args[2]),
			}})
		}
	}
	return path
}

func pointFromFixed(p fixed.Point26_6) Point {
	return Point{X: fixed26_6ToFloat(p.X), Y: fixed26_6ToFloat(p.Y)}
}

func fixed26_6ToFloat(v fixed.Int26_6) float64 {
	return float64(v) / 64.0
}

package tagsheet

import (
	"fmt"
	"math"
)

// rect is an axis-aligned bounding box in whatever coordinate space its
// Path(s) were computed in (font units or mm, depending on caller).
type rect struct {
	Min, Max Point
}

// centerX/centerY are the midpoints tagLayout centering math aligns text
// bounding boxes to.
func (r rect) centerX() float64 { return (r.Min.X + r.Max.X) / 2 }
func (r rect) centerY() float64 { return (r.Min.Y + r.Max.Y) / 2 }

// Layout places codes into a rows×cols grid of tags (row-major: left to
// right, then top to bottom) and lays out each tag's rendered text —
// rotated/positioned per the fixed geometry spec for kind. paddingMm is the
// gap between adjacent tags; there is no outer sheet margin.
func Layout(kind Kind, codes []string, rows, cols int, paddingMm float64) (Sheet, error) {
	if len(codes) != rows*cols {
		return Sheet{}, fmt.Errorf("tagsheet: got %d codes, need %d for a %d×%d sheet", len(codes), rows*cols, cols, rows)
	}

	f, err := loadOCRFont()
	if err != nil {
		return Sheet{}, err
	}

	sheet := Sheet{
		WidthMm:  float64(cols)*TagWidthMm + float64(cols-1)*paddingMm,
		HeightMm: float64(rows)*TagHeightMm + float64(rows-1)*paddingMm,
		Tags:     make([]TagLayout, 0, len(codes)),
	}

	i := 0
	for row := range rows {
		for col := range cols {
			code := codes[i]
			i++
			tagX := float64(col) * (TagWidthMm + paddingMm)
			tagY := float64(row) * (TagHeightMm + paddingMm)

			var text []Path
			switch kind {
			case KindAsset:
				text = layoutAssetText(f, code, tagX, tagY)
			case KindLocation:
				text = layoutLocationText(f, code, tagX, tagY)
			}
			sheet.Tags = append(sheet.Tags, TagLayout{Code: code, X: tagX, Y: tagY, Text: text})
		}
	}
	return sheet, nil
}

// layoutAssetText lays out code (a 4-letter asset tag) at AssetFontCapMm,
// rotates it 90° CCW so it reads bottom-to-top, then pins it 1.6mm from the
// tag's left edge and centers it vertically — matched against the user's
// working LaserTag.lbrn2 (glyph band starts 1.61mm from the left edge on
// the same 60×26mm tag).
func layoutAssetText(f *ocrFont, code string, tagX, tagY float64) []Path {
	scale := AssetFontCapMm / f.capHeight
	paths := layoutTextMm(f, code, func(int, rune) float64 { return scale })
	paths = rotateCCW(paths)
	box := bboxOf(paths...)

	dx := (tagX + assetTextLeftMarginMm) - box.Min.X
	dy := (tagY + TagHeightMm/2) - box.centerY()
	return translate(paths, dx, dy)
}

// layoutLocationText lays out code (e.g. "@XYZ") at one uniform rendered
// ink height (LocationFontCapMm) — scaling "@" off its own ink height
// (ocrFont.atHeight) rather than the letters' cap height, so it reads as
// the same size rather than visibly smaller — sharing one baseline, and
// centers the combined ink bounding box on the tag's midpoint.
func layoutLocationText(f *ocrFont, code string, tagX, tagY float64) []Path {
	letterScale := LocationFontCapMm / f.capHeight
	atScale := LocationFontCapMm / f.atHeight
	paths := layoutTextMm(f, code, func(i int, r rune) float64 {
		if r == '@' {
			return atScale
		}
		return letterScale
	})
	box := bboxOf(paths...)

	dx := (tagX + TagWidthMm/2) - box.centerX()
	dy := (tagY + TagHeightMm/2) - box.centerY()
	return translate(paths, dx, dy)
}

// layoutTextMm lays text out left-to-right as a pen-advance run starting at
// the origin, sharing one baseline. scaleFor returns the font-unit-to-mm
// scale for the i-th rune (r) of text, letting callers use a different
// reference metric per character — e.g. the location tag's "@", scaled off
// its own ink height rather than the letters' cap height. The result is in
// mm but not yet positioned on any particular tag — callers translate
// (and, for asset tags, rotate) it into place.
func layoutTextMm(f *ocrFont, text string, scaleFor func(i int, r rune) float64) []Path {
	paths := make([]Path, 0, len(text))
	pen := 0.0
	i := 0
	for _, r := range text {
		scale := scaleFor(i, r)
		g := f.glyphs[r]
		paths = append(paths, scaleAndOffsetPath(g.path, scale, pen, 0))
		pen += g.advance * scale
		i++
	}
	return paths
}

// scaleAndOffsetPath multiplies every point by scale (applied before the
// offset, so scale is in the glyph's local font-unit space) and adds
// (dx, dy).
func scaleAndOffsetPath(p Path, scale, dx, dy float64) Path {
	out := make(Path, len(p))
	for i, seg := range p {
		out[i] = seg
		for j := range seg.Pts {
			out[i].Pts[j] = Point{X: seg.Pts[j].X*scale + dx, Y: seg.Pts[j].Y*scale + dy}
		}
	}
	return out
}

// rotateCCW rotates every path 90° about the origin via (x, y) → (y, −x).
// In this package's y-down mm space, that reads bottom-to-top with cap
// tops toward what was the left edge — verified against the user's
// working LaserTag.lbrn2 asset tags.
func rotateCCW(paths []Path) []Path {
	out := make([]Path, len(paths))
	for i, p := range paths {
		rp := make(Path, len(p))
		for j, seg := range p {
			rp[j] = seg
			for k := range seg.Pts {
				pt := seg.Pts[k]
				rp[j].Pts[k] = Point{X: pt.Y, Y: -pt.X}
			}
		}
		out[i] = rp
	}
	return out
}

// translate adds (dx, dy) to every point of every path.
func translate(paths []Path, dx, dy float64) []Path {
	out := make([]Path, len(paths))
	for i, p := range paths {
		out[i] = scaleAndOffsetPath(p, 1, dx, dy)
	}
	return out
}

// bboxOf returns the union bounding box of one or more paths, considering
// every point referenced by any segment (including bezier control points —
// a conservative over-estimate for curved glyphs, exact for the
// straight-line 'X' cap-height measurement).
func bboxOf(paths ...Path) rect {
	box := rect{
		Min: Point{X: math.Inf(1), Y: math.Inf(1)},
		Max: Point{X: math.Inf(-1), Y: math.Inf(-1)},
	}
	for _, p := range paths {
		for _, seg := range p {
			n := 1
			switch seg.Op {
			case OpQuad:
				n = 2
			case OpCube:
				n = 3
			}
			for i := range n {
				pt := seg.Pts[i]
				if pt.X < box.Min.X {
					box.Min.X = pt.X
				}
				if pt.X > box.Max.X {
					box.Max.X = pt.X
				}
				if pt.Y < box.Min.Y {
					box.Min.Y = pt.Y
				}
				if pt.Y > box.Max.Y {
					box.Max.Y = pt.Y
				}
			}
		}
	}
	return box
}

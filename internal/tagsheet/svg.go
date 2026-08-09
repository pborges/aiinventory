package tagsheet

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// RenderSVG serializes sheet as an SVG document sized in millimeters (1
// SVG user unit = 1mm) with three layer groups, one per laser op, in cut
// order: black-filled text (Scan fill), blue-stroked text outline (Cut),
// red-stroked tag outline (Cut). LightBurn's SVG import maps distinct
// stroke/fill colors to distinct layers, so these colors aren't
// cosmetic — they're how the three-op laser workflow survives the import.
func RenderSVG(sheet Sheet) string {
	w, h := formatNum(sheet.WidthMm), formatNum(sheet.HeightMm)

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%smm" height="%smm" viewBox="0 0 %s %s">`+"\n", w, h, w, h)

	b.WriteString(`  <g id="text-fill" fill="#000000">` + "\n")
	for _, tag := range sheet.Tags {
		for _, p := range tag.Text {
			fmt.Fprintf(&b, `    <path d="%s"/>`+"\n", pathToSVGD(p))
		}
	}
	b.WriteString("  </g>\n")

	b.WriteString(`  <g id="text-outline" fill="none" stroke="#0000FF" stroke-width="0.1">` + "\n")
	for _, tag := range sheet.Tags {
		for _, p := range tag.Text {
			fmt.Fprintf(&b, `    <path d="%s"/>`+"\n", pathToSVGD(p))
		}
	}
	b.WriteString("  </g>\n")

	// A native <rect> can't control where its own path starts, and that
	// start point is where the laser's start/end seam (and its scorch
	// mark) ends up — so the cut boundary is built via roundedRectPath,
	// same as the LightBurn/Rayforge exporters, to keep all three
	// formats' cut seam at the same corner.
	b.WriteString(`  <g id="cut" fill="none" stroke="#FF0000" stroke-width="0.1">` + "\n")
	for _, tag := range sheet.Tags {
		fmt.Fprintf(&b, `    <path d="%s"/>`+"\n", pathToSVGD(roundedRectPath(tag.X, tag.Y, TagWidthMm, TagHeightMm, TagCornerMm)))
	}
	b.WriteString("  </g>\n")

	b.WriteString("</svg>\n")
	return b.String()
}

// pathToSVGD renders p as an SVG path "d" attribute, closing each contour
// with "Z" (a new OpMove starts the next contour, closing the previous
// one first) — used identically for both the fill and outline layers,
// which differ only in their enclosing <g>'s fill/stroke.
func pathToSVGD(p Path) string {
	var b strings.Builder
	for _, seg := range p {
		switch seg.Op {
		case OpMove:
			if b.Len() > 0 {
				b.WriteString("Z ")
			}
			fmt.Fprintf(&b, "M%s,%s ", formatNum(seg.Pts[0].X), formatNum(seg.Pts[0].Y))
		case OpLine:
			fmt.Fprintf(&b, "L%s,%s ", formatNum(seg.Pts[0].X), formatNum(seg.Pts[0].Y))
		case OpQuad:
			fmt.Fprintf(&b, "Q%s,%s %s,%s ",
				formatNum(seg.Pts[0].X), formatNum(seg.Pts[0].Y), formatNum(seg.Pts[1].X), formatNum(seg.Pts[1].Y))
		case OpCube:
			fmt.Fprintf(&b, "C%s,%s %s,%s %s,%s ",
				formatNum(seg.Pts[0].X), formatNum(seg.Pts[0].Y), formatNum(seg.Pts[1].X), formatNum(seg.Pts[1].Y),
				formatNum(seg.Pts[2].X), formatNum(seg.Pts[2].Y))
		}
	}
	if b.Len() > 0 {
		b.WriteString("Z")
	}
	return strings.TrimSpace(b.String())
}

// formatNum renders v rounded to 4 decimal places using the shortest
// decimal string that round-trips to that value — shared by svg.go and
// lbrn2.go so both writers emit identically-rounded coordinates.
func formatNum(v float64) string {
	v = math.Round(v*10000) / 10000
	return strconv.FormatFloat(v, 'f', -1, 64)
}

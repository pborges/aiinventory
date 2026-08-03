package tagsheet

import (
	"fmt"
	"strings"
)

// RenderLBRN2 serializes sheet as a native LightBurn project: three
// CutSettings (Scan "Text Fill", Cut "Text Outline", Cut "Cut", priorities
// 0/1/2) and one <Shape Type="Path"> per glyph contour (emitted twice, once
// per text CutIndex — a Scan raster fills the glyph, then a Cut retraces
// its outline) plus one rounded-rect boundary Shape per tag on the third
// CutIndex. Every Shape uses an identity XForm; all rotation/translation is
// already baked into the coordinates by tagsheet.Layout, and the only
// transform this writer applies itself is the y-flip LightBurn expects
// (its Y axis increases up, this package's increases down).
//
// The VertList/PrimList encoding below — the "c0x1"/"c1x1" placeholders
// for a vertex with no real bezier control on that side, one Shape per
// contour so letter counters render as holes, PrimList's closing prim
// referencing back to vertex 0 rather than duplicating it — was reverse
// engineered from the user's own working LaserTag.lbrn2 (LightBurn's
// condensed project format isn't otherwise documented).
func RenderLBRN2(sheet Sheet, cs CutSettings) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<LightBurnProject AppVersion="2.1.03" FormatVersion="1" MaterialHeight="0" MirrorX="False" MirrorY="False">` + "\n")
	b.WriteString(variableTextXML)
	b.WriteString(uiPrefsXML)
	b.WriteString(cutSettingXML(0, "Scan", "Text Fill", cs.RasterPowerPct, cs.RasterSpeedMmMin, cs.RasterAirAssist, 0))
	b.WriteString(cutSettingXML(1, "Cut", "Text Outline", cs.OutlinePowerPct, cs.OutlineSpeedMmMin, cs.OutlineAirAssist, 1))
	b.WriteString(cutSettingXML(2, "Cut", "Cut", cs.CutPowerPct, cs.CutSpeedMmMin, cs.CutAirAssist, 2))
	for _, tag := range sheet.Tags {
		writeTagShapes(&b, tag, sheet.HeightMm)
	}
	b.WriteString(`    <Notes ShowOnLoad="0" Notes=""/>` + "\n")
	b.WriteString("</LightBurnProject>\n")
	return b.String()
}

const variableTextXML = `    <VariableText>
        <Start Value="0"/>
        <End Value="999"/>
        <Current Value="0"/>
        <Increment Value="1"/>
        <AutoAdvance Value="0"/>
    </VariableText>
`

const uiPrefsXML = `    <UIPrefs>
        <Optimize_ByLayer Value="0"/>
        <Optimize_ByGroup Value="-1"/>
        <Optimize_ByPriority Value="1"/>
        <Optimize_WhichDirection Value="0"/>
        <Optimize_InnerToOuter Value="1"/>
        <Optimize_ByDirection Value="0"/>
        <Optimize_ReduceTravel Value="1"/>
        <Optimize_HideBacklash Value="0"/>
        <Optimize_ReduceDirChanges Value="0"/>
        <Optimize_ChooseCorners Value="0"/>
        <Optimize_AllowReverse Value="1"/>
        <Optimize_RemoveOverlaps Value="0"/>
        <Optimize_OptimalEntryPoint Value="0"/>
    </UIPrefs>
`

// cutSettingXML renders one CutSetting block. speedMmMin is converted to
// mm/sec — the unit the .lbrn2 file format actually stores, regardless of
// what unit LightBurn's UI happens to display — at the point of writing.
func cutSettingXML(index int, cutType, name string, maxPowerPct, speedMmMin float64, airAssist bool, priority int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "    <CutSetting type=%q>\n", cutType)
	fmt.Fprintf(&b, `        <index Value="%d"/>`+"\n", index)
	fmt.Fprintf(&b, `        <name Value=%q/>`+"\n", name)
	fmt.Fprintf(&b, `        <maxPower Value="%s"/>`+"\n", formatNum(maxPowerPct))
	fmt.Fprintf(&b, `        <speed Value="%s"/>`+"\n", formatNum(mmPerMinToMmPerSec(speedMmMin)))
	fmt.Fprintf(&b, `        <runBlower Value="%s"/>`+"\n", boolToLBRN2(airAssist))
	fmt.Fprintf(&b, `        <priority Value="%d"/>`+"\n", priority)
	b.WriteString("    </CutSetting>\n")
	return b.String()
}

func mmPerMinToMmPerSec(v float64) float64 { return v / 60 }

func boolToLBRN2(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

// writeTagShapes emits one tag's Shapes: every glyph contour twice (Scan
// fill CutIndex 0, then Cut outline CutIndex 1), then the tag's
// rounded-rect boundary once (Cut CutIndex 2).
func writeTagShapes(b *strings.Builder, tag TagLayout, sheetHeightMm float64) {
	for _, glyphPath := range tag.Text {
		for _, contour := range splitContours(glyphPath) {
			b.WriteString(contourShapeXML(contour, 0, sheetHeightMm))
			b.WriteString(contourShapeXML(contour, 1, sheetHeightMm))
		}
	}
	rect := roundedRectPath(tag.X, tag.Y, TagWidthMm, TagHeightMm, TagCornerMm)
	b.WriteString(contourShapeXML(rect, 2, sheetHeightMm))
}

// splitContours breaks a glyph's Path (which may describe several
// contours — an outer outline plus inner counters, e.g. the holes in "A"
// or "@") at each OpMove into one Path per contour.
func splitContours(p Path) []Path {
	var out []Path
	var cur Path
	for _, seg := range p {
		if seg.Op == OpMove && len(cur) > 0 {
			out = append(out, cur)
			cur = nil
		}
		cur = append(cur, seg)
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

// lbrn2Vertex is one contour vertex plus its optional exit control (c0 —
// the bezier control for the curve leaving this vertex) and entry control
// (c1 — for the curve arriving at this vertex). A control is only set
// (has*=true) when that side of the vertex is actually curved; a straight
// edge leaves both sides as placeholders.
type lbrn2Vertex struct {
	pt                Point
	hasExit, hasEntry bool
	exit, entry       Point
}

type lbrn2Prim struct {
	from, to int
	curve    bool
}

// contourVertsAndPrims converts one self-closing contour (c[0] is OpMove;
// c's last segment's endpoint already coincides with c[0]'s point — true
// for every glyph contour sfnt gives us, and for roundedRectPath by
// construction) into LightBurn's vertex/prim model. The closing segment
// doesn't get its own vertex — it's folded into a prim that references
// back to vertex 0, matching how LightBurn itself encodes a closed path.
func contourVertsAndPrims(c Path) ([]lbrn2Vertex, []lbrn2Prim) {
	n := len(c) - 1 // drawing segments, excluding the initial Move
	verts := make([]lbrn2Vertex, n)
	verts[0].pt = c[0].Pts[0]
	for i := 1; i < n; i++ {
		verts[i].pt = endpointOf(c[i])
	}

	prims := make([]lbrn2Prim, 0, n)
	for j := 1; j <= n; j++ {
		seg := c[j]
		fromIdx := j - 1
		toIdx := j % n // the closing segment (j==n) wraps back to vertex 0

		switch seg.Op {
		case OpLine:
			prims = append(prims, lbrn2Prim{from: fromIdx, to: toIdx, curve: false})
		case OpQuad:
			p0, q, p2 := verts[fromIdx].pt, seg.Pts[0], seg.Pts[1]
			exit := Point{X: p0.X + 2.0/3.0*(q.X-p0.X), Y: p0.Y + 2.0/3.0*(q.Y-p0.Y)}
			entry := Point{X: p2.X + 2.0/3.0*(q.X-p2.X), Y: p2.Y + 2.0/3.0*(q.Y-p2.Y)}
			verts[fromIdx].hasExit, verts[fromIdx].exit = true, exit
			verts[toIdx].hasEntry, verts[toIdx].entry = true, entry
			prims = append(prims, lbrn2Prim{from: fromIdx, to: toIdx, curve: true})
		case OpCube:
			verts[fromIdx].hasExit, verts[fromIdx].exit = true, seg.Pts[0]
			verts[toIdx].hasEntry, verts[toIdx].entry = true, seg.Pts[1]
			prims = append(prims, lbrn2Prim{from: fromIdx, to: toIdx, curve: true})
		}
	}
	return verts, prims
}

func endpointOf(seg Seg) Point {
	switch seg.Op {
	case OpQuad:
		return seg.Pts[1]
	case OpCube:
		return seg.Pts[2]
	default: // OpMove, OpLine
		return seg.Pts[0]
	}
}

// contourShapeXML renders one contour as a single <Shape Type="Path">,
// flipping every coordinate from this package's y-down mm space into
// LightBurn's y-up (yUp = sheetHeightMm - yDown).
func contourShapeXML(c Path, cutIndex int, sheetHeightMm float64) string {
	verts, prims := contourVertsAndPrims(c)

	var b strings.Builder
	fmt.Fprintf(&b, `    <Shape Type="Path" CutIndex="%d">`+"\n", cutIndex)
	b.WriteString("        <XForm>1 0 0 1 0 0</XForm>\n")

	b.WriteString("        <VertList>")
	for _, v := range verts {
		p := flipY(v.pt, sheetHeightMm)
		fmt.Fprintf(&b, "V%s %s", formatNum(p.X), formatNum(p.Y))
		if v.hasExit {
			e := flipY(v.exit, sheetHeightMm)
			fmt.Fprintf(&b, "c0x%sc0y%s", formatNum(e.X), formatNum(e.Y))
		} else {
			b.WriteString("c0x1")
		}
		if v.hasEntry {
			e := flipY(v.entry, sheetHeightMm)
			fmt.Fprintf(&b, "c1x%sc1y%s", formatNum(e.X), formatNum(e.Y))
		} else {
			b.WriteString("c1x1")
		}
	}
	b.WriteString("</VertList>\n")

	b.WriteString("        <PrimList>")
	for _, pr := range prims {
		op := "L"
		if pr.curve {
			op = "B"
		}
		fmt.Fprintf(&b, "%s%d %d", op, pr.from, pr.to)
	}
	b.WriteString("</PrimList>\n")

	b.WriteString("    </Shape>\n")
	return b.String()
}

func flipY(p Point, sheetHeightMm float64) Point {
	return Point{X: p.X, Y: sheetHeightMm - p.Y}
}

// roundedRectPath builds a self-closing 8-vertex rounded-rect contour (4
// straight edges, 4 cubic-bezier corners) as a Path — a synthetic contour
// in the same shape contourVertsAndPrims expects from a glyph, so the tag
// boundary shares its emission code with glyph outlines. Corner control
// points sit kappa*r along each edge's tangent from the corner's two
// tangent points, the standard circle-to-cubic-bezier approximation.
func roundedRectPath(x, y, w, h, r float64) Path {
	k := r * bezierK
	v0 := Point{X: x + r, Y: y}
	v1 := Point{X: x + w - r, Y: y}
	v2 := Point{X: x + w, Y: y + r}
	v3 := Point{X: x + w, Y: y + h - r}
	v4 := Point{X: x + w - r, Y: y + h}
	v5 := Point{X: x + r, Y: y + h}
	v6 := Point{X: x, Y: y + h - r}
	v7 := Point{X: x, Y: y + r}

	return Path{
		{Op: OpMove, Pts: [3]Point{v0}},
		{Op: OpLine, Pts: [3]Point{v1}},
		{Op: OpCube, Pts: [3]Point{{X: v1.X + k, Y: v1.Y}, {X: v2.X, Y: v2.Y - k}, v2}},
		{Op: OpLine, Pts: [3]Point{v3}},
		{Op: OpCube, Pts: [3]Point{{X: v3.X, Y: v3.Y + k}, {X: v4.X + k, Y: v4.Y}, v4}},
		{Op: OpLine, Pts: [3]Point{v5}},
		{Op: OpCube, Pts: [3]Point{{X: v5.X - k, Y: v5.Y}, {X: v6.X, Y: v6.Y + k}, v6}},
		{Op: OpLine, Pts: [3]Point{v7}},
		{Op: OpCube, Pts: [3]Point{{X: v7.X, Y: v7.Y - k}, {X: v0.X - k, Y: v0.Y}, v0}},
	}
}

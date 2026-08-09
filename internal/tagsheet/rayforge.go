package tagsheet

import "github.com/pborges/rayforged"

// RenderRayforge serializes sheet as a native Rayforge (rayforge.org)
// .ryp project file via github.com/pborges/rayforged — a library
// extracted from this exact code after reverse engineering Rayforge's
// undocumented project format (see that module's package doc comment
// for the full story). It produces three layers, one per cut
// operation (mirroring RenderSVG's three groups and RenderLBRN2's
// three CutSettings): a raster engrave of the filled text, a vector
// cut of the text's outline, and a vector cut of the tag's
// rounded-rect boundary. All three layers' WorkPieces share one
// SourceAsset — the same document RenderSVG produces.
func RenderRayforge(sheet Sheet, cs CutSettings) ([]byte, error) {
	asset := &rayforged.SourceAsset{
		Name:     "tag-sheet.svg",
		SVG:      []byte(RenderSVG(sheet)),
		WidthMm:  sheet.WidthMm,
		HeightMm: sheet.HeightMm,
	}

	var fillPaths, outlinePaths []rayforged.Path
	for _, tag := range sheet.Tags {
		for _, p := range tag.Text {
			fillPaths = append(fillPaths, toRayforgePath(p))
			outlinePaths = append(outlinePaths, toRayforgePath(p))
		}
	}
	cutPaths := make([]rayforged.Path, len(sheet.Tags))
	for i, tag := range sheet.Tags {
		cutPaths[i] = toRayforgePath(roundedRectPath(tag.X, tag.Y, TagWidthMm, TagHeightMm, TagCornerMm))
	}

	doc := &rayforged.Doc{HeightMm: sheet.HeightMm}

	raster := doc.AddLayer("Raster Text", "#00ccff")
	raster.Steps = []rayforged.Step{rayforged.EngraveStep{
		Name: "Raster Text Engrave", SpeedMmMin: cs.RasterSpeedMmMin, PowerPct: cs.RasterPowerPct, AirAssist: cs.RasterAirAssist,
	}}
	raster.WorkPieces = []rayforged.WorkPiece{{
		Name: "Raster Text", Source: asset, SVGLayerID: "text-fill", Geometry: fillPaths,
	}}

	outline := doc.AddLayer("Outline Text", "#ff6600")
	outline.Steps = []rayforged.Step{rayforged.ContourStep{
		Name: "Outline Text Cut", SpeedMmMin: cs.OutlineSpeedMmMin, PowerPct: cs.OutlinePowerPct, AirAssist: cs.OutlineAirAssist, CutSide: rayforged.CutSideCenterline,
	}}
	outline.WorkPieces = []rayforged.WorkPiece{{
		Name: "Outline Text", Source: asset, SVGLayerID: "text-outline", Geometry: outlinePaths,
	}}

	cut := doc.AddLayer("Cut Tag", "#33cc33")
	cut.Steps = []rayforged.Step{rayforged.ContourStep{
		Name: "Cut Tag Border", SpeedMmMin: cs.CutSpeedMmMin, PowerPct: cs.CutPowerPct, AirAssist: cs.CutAirAssist, CutSide: rayforged.CutSideOutside,
	}}
	cut.WorkPieces = []rayforged.WorkPiece{{
		Name: "Cut Tag", Source: asset, SVGLayerID: "cut", Geometry: cutPaths,
	}}

	return doc.Marshal()
}

// toRayforgePath converts this package's own Path/Seg representation
// (shared with svg.go and lbrn2.go) into github.com/pborges/rayforged's
// equivalent — the two are structurally identical (they share a
// common origin: the rayforge library's geometry types were extracted
// from this package's own), just distinct Go types across the module
// boundary.
func toRayforgePath(p Path) rayforged.Path {
	out := make(rayforged.Path, len(p))
	for i, seg := range p {
		var op rayforged.SegOp
		switch seg.Op {
		case OpMove:
			op = rayforged.OpMove
		case OpLine:
			op = rayforged.OpLine
		case OpQuad:
			op = rayforged.OpQuad
		case OpCube:
			op = rayforged.OpCubic
		}
		var pts [3]rayforged.Point
		for j, pt := range seg.Pts {
			pts[j] = rayforged.Point{X: pt.X, Y: pt.Y}
		}
		out[i] = rayforged.Segment{Op: op, Pts: pts}
	}
	return out
}

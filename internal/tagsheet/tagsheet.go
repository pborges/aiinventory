// Package tagsheet generates laser-cuttable sheets of pre-registered asset
// and location tags: a grid of rounded-rect tags with OCR-B text, exported
// as both SVG (for a quick preview/import) and LightBurn's native .lbrn2
// project format (for direct laser cutting). It replaces the manual
// FreeCAD/LightBurn workflow that previously produced these sheets by hand.
//
// The single source of truth is an abstract Sheet of Paths in millimeters,
// y-down, with every transform (rotation, translation, centering) already
// baked into the coordinates. svg.go and lbrn2.go each serialize that same
// geometry into their respective formats — svg.go directly, lbrn2.go with a
// y-flip (LightBurn is y-up) and a quadratic-to-cubic conversion (TrueType
// glyphs are quadratic; LightBurn paths are cubic).
package tagsheet

// Kind selects which tag geometry/text a Sheet is built for.
type Kind int

const (
	KindAsset Kind = iota
	KindLocation
)

// Point is a coordinate in millimeters, y-down (SVG/screen convention).
type Point struct {
	X, Y float64
}

// SegOp is a path segment's drawing operator, mirroring TrueType/SVG path
// commands minus ClosePath — contours are closed explicitly by writers
// (SVG emits "Z", lbrn2 closes its PrimList back to vertex 0) rather than
// carrying a synthetic closing segment through the shared geometry.
type SegOp int

const (
	OpMove SegOp = iota
	OpLine
	OpQuad // quadratic bezier: Pts[0] = control, Pts[1] = end
	OpCube // cubic bezier: Pts[0], Pts[1] = controls, Pts[2] = end
)

// Seg is one path segment. Which entries of Pts are meaningful depends on
// Op: Move/Line use Pts[0] as the destination; Quad uses Pts[0] (control)
// and Pts[1] (end); Cube uses Pts[0], Pts[1] (controls) and Pts[2] (end).
type Seg struct {
	Op  SegOp
	Pts [3]Point
}

// Path is a sequence of segments forming one or more contours. The first
// segment of each contour is always OpMove.
type Path []Seg

// TagLayout is one tag's placement on the sheet: its registered code, the
// tag's top-left origin in sheet-space mm, and the already-positioned glyph
// outlines (also in sheet-space mm) that render its code.
type TagLayout struct {
	Code string
	X, Y float64
	Text []Path
}

// Sheet is the fully laid-out output: sheet dimensions in mm plus every
// tag's geometry, ready to hand to RenderSVG or RenderLBRN2.
type Sheet struct {
	WidthMm, HeightMm float64
	Tags              []TagLayout
}

// Geometry constants shared by both tag kinds and every writer.
const (
	TagWidthMm  = 60.0
	TagHeightMm = 26.0
	TagCornerMm = 2.0

	AssetFontCapMm = 6.5 // OCR-B cap height for the 4-letter asset code

	// LocationFontCapMm is the target rendered ink height, in mm, for every
	// character of a location tag — including the leading "@", which is
	// scaled off its own measured ink height (ocrFont.atHeight) rather than
	// the letters' cap height so it reads as the same size instead of
	// visibly smaller.
	LocationFontCapMm = 15.0

	// assetTextLeftMarginMm is how far the rotated asset-tag text's left
	// edge (after rotation, its bounding box's min X) sits from the tag's
	// left edge — matched against the user's working LaserTag.lbrn2,
	// whose glyph band starts 1.61mm from the left edge on the same
	// 60x26mm tag.
	assetTextLeftMarginMm = 1.6

	// bezierK is the standard circle-to-cubic-bezier magic number (4/3 *
	// (sqrt(2)-1)), used to approximate a quarter-circle corner arc with
	// one cubic bezier for the tag's rounded-rect outline.
	bezierK = 0.5522847498
)

// CutSettings holds the LightBurn/Rayforge speed/power/air-assist for each
// of the sheet's two operations: the raster engrave of the text, and the
// vector cut of the tag's own rounded-rect boundary. Speeds are mm/min —
// LightBurn's default UI unit — converted to the mm/sec the .lbrn2 file
// format actually stores at RenderLBRN2 time; powers are percent (0-100].
// RasterLineIntervalMm is the raster's line-to-line pitch in mm (smaller is
// denser/darker coverage) — there's no cut-side equivalent since a vector
// cut has no scan lines.
type CutSettings struct {
	RasterSpeedMmMin, RasterPowerPct, RasterLineIntervalMm float64
	RasterAirAssist                                        bool
	CutSpeedMmMin, CutPowerPct                             float64
	CutAirAssist                                           bool
}

// DefaultCutSettings are reasonable starting points for a 20W diode laser
// cutting/engraving 60x26mm tags out of 3mm basswood — tune per
// material/machine in LightBurn/Rayforge after import. Air assist defaults
// on only for the through-cut, where it matters most for preventing
// charring and flame-up; the raster pass doesn't usually need it.
var DefaultCutSettings = CutSettings{
	RasterSpeedMmMin: 7000, RasterPowerPct: 32, RasterLineIntervalMm: 0.05, RasterAirAssist: false,
	CutSpeedMmMin: 200, CutPowerPct: 100, CutAirAssist: true,
}

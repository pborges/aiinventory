package tagsheet

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"math"

	"github.com/google/uuid"
)

// RenderRayforge serializes sheet as a native Rayforge (rayforge.org) .ryp
// project file: a zip archive containing one project.json, matching what
// Rayforge itself writes via its own Save Project (see
// doceditor/file_cmd.py's save_project_to_path in Rayforge's source —
// json.dumps(doc.to_dict()) zipped under the single member name
// "project.json"). This is an internal, undocumented, unversioned format
// with no public spec; the shape below was reverse engineered from
// Rayforge's own to_dict/from_dict methods (core/doc.py, layer.py,
// workflow.py, step.py, workpiece.py, source_asset.py,
// source_asset_segment.py) plus its compiled geometry/matrix engine
// (raygeo), not from any interchange-format documentation the way
// lbrn2.go's LightBurn format was. It produces three layers, one per cut
// operation (mirroring RenderSVG's three groups and RenderLBRN2's three
// CutSettings): a raster engrave of the filled text, a vector cut of the
// text's outline, and a vector cut of the tag's rounded-rect boundary.
// Each layer gets its own Workflow (one Step: EngraveStep or ContourStep,
// carrying that operation's speed/power/air-assist directly — no material
// "recipe" or specific machine head is required for those values to load)
// and one WorkPiece whose geometry is embedded directly as a
// SourceAssetSegment.pristine_geometry, rather than left for Rayforge to
// re-derive from parsing the embedded SVG on import. All three WorkPieces
// share one SourceAsset (the same document RenderSVG produces), each
// scoped to one of its "text-fill"/"text-outline"/"cut" groups via
// source_segment.layer_id — the same top-level <g id> grouping Rayforge's
// own SVG importer uses to split a multi-layer SVG (see
// image/svg/svg_vector.py's _layer_geometries_by_svg).
func RenderRayforge(sheet Sheet, cs CutSettings) ([]byte, error) {
	svgSource := RenderSVG(sheet)
	assetUID := uuid.NewString()

	var fillPaths, outlinePaths []Path
	for _, tag := range sheet.Tags {
		fillPaths = append(fillPaths, tag.Text...)
		outlinePaths = append(outlinePaths, tag.Text...)
	}
	cutPaths := make([]Path, len(sheet.Tags))
	for i, tag := range sheet.Tags {
		cutPaths[i] = roundedRectPath(tag.X, tag.Y, TagWidthMm, TagHeightMm, TagCornerMm)
	}

	doc := map[string]any{
		"uid":                uuid.NewString(),
		"type":               "doc",
		"active_layer_index": 0,
		"children": []any{
			rayforgeLayer(sheet, "Raster Text", "#00ccff", "text-fill", fillPaths, assetUID,
				engraveStepDict("Raster Text Engrave", cs.RasterSpeedMmMin, cs.RasterPowerPct, cs.RasterAirAssist)),
			rayforgeLayer(sheet, "Outline Text", "#ff6600", "text-outline", outlinePaths, assetUID,
				contourStepDict("Outline Text Cut", cs.OutlineSpeedMmMin, cs.OutlinePowerPct, cs.OutlineAirAssist, "CENTERLINE")),
			rayforgeLayer(sheet, "Cut Tag", "#33cc33", "cut", cutPaths, assetUID,
				contourStepDict("Cut Tag Border", cs.CutSpeedMmMin, cs.CutPowerPct, cs.CutAirAssist, "OUTSIDE")),
		},
		"assets": []any{
			map[string]any{
				"uid":              assetUID,
				"type":             "source",
				"name":             "tag-sheet.svg",
				"source_file":      "tag-sheet.svg",
				"original_data":    base64.StdEncoding.EncodeToString([]byte(svgSource)),
				"base_render_data": nil,
				"thumbnail_data":   nil,
				"renderer_name":    "SvgRenderer",
				"metadata":         map[string]any{},
				"width_px":         nil,
				"height_px":        nil,
				"width_mm":         sheet.WidthMm,
				"height_mm":        sheet.HeightMm,
				"hidden":           false,
			},
		},
	}

	jsonBytes, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create("project.json")
	if err != nil {
		return nil, err
	}
	if _, err := f.Write(jsonBytes); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

var identityMatrix = [3][3]float64{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}

// rayforgeLayer builds one Layer dict — a Workflow (wrapping the given
// step) plus a WorkPiece positioned at paths' bounding box, both parented
// under the layer, matching Layer.to_dict's {uid, type, name, matrix,
// visible, rotary_*, color, wcs, children:[workflow, workpiece]} shape.
func rayforgeLayer(sheet Sheet, name, color, layerID string, paths []Path, assetUID string, step map[string]any) map[string]any {
	commands, minX, minY, maxX, maxY := pathsToCommands(paths)
	bw, bh := maxX-minX, maxY-minY

	// World matrix: places the workpiece at its content's bounding box, in
	// Rayforge's Y-up document space — this package's sheet space (like
	// SVG) is Y-down, so the Y position flips against the page height.
	// Reverse engineered from NormalizationEngine.calculate_layout_item:
	// pos = (bx, page_h - (by+bh)); matrix = translate(pos) @ scale(bw,bh).
	worldMatrix := [3][3]float64{{bw, 0, minX}, {0, bh, sheet.HeightMm - maxY}, {0, 0, 1}}

	// Normalization matrix: maps the content's bounding box into the
	// workpiece's own 0..1 unit space, also Y-up. Also reverse engineered
	// from calculate_layout_item: translate(-bx,-by) @ scale(1/bw,1/bh) @
	// scale(1,-1) @ translate(0,1), composed into one 3x3 here.
	normMatrix := [3][3]float64{
		{1 / bw, 0, -minX / bw},
		{0, -1 / bh, 1 + minY/bh},
		{0, 0, 1},
	}

	workpiece := map[string]any{
		"uid":          uuid.NewString(),
		"type":         "workpiece",
		"name":         name,
		"matrix":       worldMatrix,
		"width_mm":     bw,
		"height_mm":    bh,
		"tabs":         []any{},
		"tabs_enabled": true,
		"source_segment": map[string]any{
			"source_asset_uid":     assetUID,
			"image_modifier_chain": []any{},
			"vectorization_spec": map[string]any{
				"type":              "PassthroughSpec",
				"active_layer_ids":  nil,
				"layer_import_mode": "new_layers",
				"layer_source":      "svg_layers",
				"color_attr":        "any",
				"trim_padding":      0.01,
				"ppi":               96.0,
			},
			"crop_window_px":    [4]float64{minX, minY, bw, bh},
			"cropped_width_mm":  bw,
			"cropped_height_mm": bh,
			"layer_id":          layerID,
			"pristine_geometry": map[string]any{
				"last_move_to":     [3]float64{0, 0, 0},
				"uniform_scalable": true,
				"commands":         commands,
			},
			"normalization_matrix": normMatrix,
		},
		"edited_boundaries":        nil,
		"geometry_provider_uid":    nil,
		"geometry_provider_params": map[string]any{},
		"source_asset_uid":         nil,
	}

	workflow := map[string]any{
		"uid":      uuid.NewString(),
		"type":     "workflow",
		"name":     name + " Workflow",
		"matrix":   identityMatrix,
		"children": []any{step},
	}

	return map[string]any{
		"uid":               uuid.NewString(),
		"type":              "layer",
		"name":              name,
		"matrix":            identityMatrix,
		"visible":           true,
		"rotary_enabled":    false,
		"rotary_diameter":   25.0,
		"rotary_module_uid": nil,
		"color":             color,
		"wcs":               nil,
		"children":          []any{workflow, workpiece},
	}
}

// baseStepFields returns the fields common to every Step subclass (from
// Step.to_dict) plus LaserStep's power/air-assist fields (from
// LaserStep.to_dict) — the ones both EngraveStep and ContourStep share.
// applied_recipe_uid and selected_head_uid are left null: a step carries
// its speed/power/air-assist directly and doesn't need a material recipe
// or a specific machine head bound to load with those values intact.
func baseStepFields(stepType, name, typelabel string, speedMmMin, powerPct float64, airAssist bool) map[string]any {
	return map[string]any{
		"uid":                              uuid.NewString(),
		"type":                             "step",
		"step_type":                        stepType,
		"name":                             name,
		"matrix":                           identityMatrix,
		"typelabel":                        typelabel,
		"visible":                          true,
		"selected_head_uid":                nil,
		"generated_workpiece_uid":          nil,
		"applied_recipe_uid":               nil,
		"per_workpiece_transformers_dicts": []any{},
		"per_step_transformers_dicts":      []any{},
		"pixels_per_mm":                    [2]int{50, 50},
		"cut_speed":                        speedMmMin,
		"max_cut_speed":                    10000,
		"travel_speed":                     5000,
		"max_travel_speed":                 10000,
		"coolant_method":                   "OFF",
		"children":                         []any{},
		"power":                            powerPct / 100,
		"max_power":                        1000,
		"air_assist":                       airAssist,
		"tab_power":                        0.0,
		"frequency":                        0,
		"pulse_width":                      0,
	}
}

// engraveStepDict builds an EngraveStep — the raster-fill operation —
// with defaults for everything beyond speed/power/air-assist (dithering,
// scan mode, etc.) matching what Rayforge's own "New Step" creates.
func engraveStepDict(name string, speedMmMin, powerPct float64, airAssist bool) map[string]any {
	d := baseStepFields("EngraveStep", name, "Engrave", speedMmMin, powerPct, airAssist)
	d["scan_angle"] = 0.0
	d["depth_mode"] = "POWER_MODULATION"
	d["invert"] = false
	d["auto_levels"] = true
	d["black_point"] = 0
	d["white_point"] = 255
	d["threshold"] = 128
	d["line_interval_mm"] = nil
	d["sample_interval_mm"] = nil
	d["dot_width_correction_mm"] = nil
	d["min_power_level"] = 0.0
	d["max_power_level"] = 1.0
	d["num_power_levels"] = 25
	d["offset_x_mm"] = 0.0
	d["offset_y_mm"] = 0.0
	d["scan_mode"] = "SEGMENTED"
	d["cross_hatch"] = false
	d["num_depth_levels"] = 5
	d["z_step_down"] = 0.0
	d["angle_increment"] = 0.0
	d["dither_algorithm"] = nil
	d["bidir_x_offset_mm"] = 0.0
	return d
}

// contourStepDict builds a ContourStep — a vector cut/trace operation —
// with cutSide ("CENTERLINE" for tracing the text outline in place,
// "OUTSIDE" for cutting the tag free of the sheet along its border).
func contourStepDict(name string, speedMmMin, powerPct float64, airAssist bool, cutSide string) map[string]any {
	d := baseStepFields("ContourStep", name, "Contour", speedMmMin, powerPct, airAssist)
	d["cut_side"] = cutSide
	d["cut_order"] = "INSIDE_OUTSIDE"
	d["remove_inner_paths"] = false
	d["offset_mm"] = 0.0
	d["overcut"] = 0.0
	d["override_threshold"] = false
	d["threshold"] = 0.5
	return d
}

// pathsToCommands flattens paths (already positioned in sheet-space mm,
// y-down) into Rayforge's Geometry command encoding: ["M"|"L", x, y, z]
// for a move/line, ["B", endx, endy, endz, c1x, c1y, c1z, c2x, c2y, c2z]
// for a cubic bezier (z is always 0 — this package is 2D). It also
// returns the bounding box of every point referenced, including bezier
// control points, so the box is always a safe (if occasionally slightly
// loose) superset of the drawn ink — crop_window_px, the normalization
// matrix, and the workpiece's world matrix all derive from this same box,
// so self-consistency matters far more than tightness.
// Every contour in paths already closes itself — its last segment's
// endpoint coincides with its opening Move's point (true for every glyph
// contour font.go produces and for roundedRectPath by construction, per
// lbrn2.go's splitContours comment) — so no synthetic closing command is
// emitted here.
func pathsToCommands(paths []Path) (commands [][]any, minX, minY, maxX, maxY float64) {
	minX, minY = math.Inf(1), math.Inf(1)
	maxX, maxY = math.Inf(-1), math.Inf(-1)
	include := func(p Point) {
		minX, maxX = math.Min(minX, p.X), math.Max(maxX, p.X)
		minY, maxY = math.Min(minY, p.Y), math.Max(maxY, p.Y)
	}

	var cur Point
	for _, path := range paths {
		for _, seg := range path {
			switch seg.Op {
			case OpMove:
				cur = seg.Pts[0]
				include(cur)
				commands = append(commands, []any{"M", cur.X, cur.Y, 0.0})
			case OpLine:
				cur = seg.Pts[0]
				include(cur)
				commands = append(commands, []any{"L", cur.X, cur.Y, 0.0})
			case OpQuad:
				q, end := seg.Pts[0], seg.Pts[1]
				c1 := Point{X: cur.X + 2.0/3.0*(q.X-cur.X), Y: cur.Y + 2.0/3.0*(q.Y-cur.Y)}
				c2 := Point{X: end.X + 2.0/3.0*(q.X-end.X), Y: end.Y + 2.0/3.0*(q.Y-end.Y)}
				include(q)
				include(end)
				commands = append(commands, []any{"B", end.X, end.Y, 0.0, c1.X, c1.Y, 0.0, c2.X, c2.Y, 0.0})
				cur = end
			case OpCube:
				c1, c2, end := seg.Pts[0], seg.Pts[1], seg.Pts[2]
				include(c1)
				include(c2)
				include(end)
				commands = append(commands, []any{"B", end.X, end.Y, 0.0, c1.X, c1.Y, 0.0, c2.X, c2.Y, 0.0})
				cur = end
			}
		}
	}
	return commands, minX, minY, maxX, maxY
}

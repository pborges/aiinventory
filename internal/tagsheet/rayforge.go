package tagsheet

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"maps"
	"math"

	"github.com/google/uuid"
)

// RenderRayforge serializes sheet as a native Rayforge (rayforge.org) .ryp
// project file: a zip archive containing one project.json, matching what
// Rayforge itself writes via its own Save Project (see
// doceditor/file_cmd.py's save_project_to_path in Rayforge's source —
// json.dumps(doc.to_dict()) zipped under the single member name
// "project.json"). This is an internal, undocumented, unversioned format
// with no public spec.
//
// The shape below targets the installed macOS app (v1.8.5 per its
// version.txt), NOT Rayforge's github.com/barebaric/rayforge "main"
// branch, which several rounds of testing (loading generated files in
// the real app, and inspecting a real project the app itself saved)
// showed disagreeing with the installed build on multiple field names
// and a couple of enum values — main is ahead of what's actually
// shipped. The installed app still writes a step's speed/power/mode
// settings as flat top-level fields (that's the primary path each step
// subclass's from_dict reads first) AND an opsproducer_dict (a
// vestigial pre-refactor payload — present in every real save, its
// own values don't need to agree with the top-level ones since nothing
// appears to read them back once the flat fields exist, but the key
// itself is a hard requirement: rayforge/core/step.py's from_dict
// does an unguarded data["opsproducer_dict"] before a subclass ever
// gets a say, so omitting it entirely crashes the load with
// "KeyError: 'opsproducer_dict'"). This shape — and the exact flat
// field names/values for EngraveStep, which don't all match either
// Rayforge's git history or its own class source — came from a real
// project.json the installed app wrote after importing a generated
// sheet and adjusting settings by hand.
//
// It produces three layers, one per cut operation (mirroring
// RenderSVG's three groups and RenderLBRN2's three CutSettings): a
// raster engrave of the filled text, a vector cut of the text's
// outline, and a vector cut of the tag's rounded-rect boundary. Each
// layer gets its own Workflow (one Step: EngraveStep or ContourStep,
// carrying that operation's speed/power/air-assist both as the step's
// own top-level fields and inside its opsproducer_dict.params — no
// material "recipe" or specific machine laser is required for those
// values to load, both are left null) and one WorkPiece whose geometry
// is embedded directly as a SourceAssetSegment.pristine_geometry,
// rather than left for Rayforge to re-derive from parsing the embedded
// SVG on import. All three WorkPieces share one SourceAsset (the same
// document RenderSVG produces).
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
// under the layer, matching the shape Layer.to_dict produces in the
// installed app.
func rayforgeLayer(sheet Sheet, name, color, svgLayerID string, paths []Path, assetUID string, step map[string]any) map[string]any {
	commands, minX, minY, maxX, maxY := pathsToCommands(paths)
	bw, bh := maxX-minX, maxY-minY

	// World matrix: places the workpiece at its content's bounding box, in
	// Rayforge's Y-up document space — this package's sheet space, like
	// SVG, is Y-down, so the Y position flips against the page height.
	worldMatrix := [3][3]float64{{bw, 0, minX}, {0, bh, sheet.HeightMm - maxY}, {0, 0, 1}}

	// Normalization matrix: maps the content's bounding box into the
	// workpiece's own 0..1 unit space, also Y-up.
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
				"type": "PassthroughSpec",
				// The <g id="..."> group in the shared SVG this workpiece's
				// geometry came from — only consulted if the operator later
				// re-vectorizes from the source asset; pristine_geometry
				// below is what actually loads.
				"active_layer_ids":  []string{svgLayerID},
				"layer_import_mode": "flatten",
				"trim_padding":      0.01,
				"ppi":               96.0,
			},
			"crop_window_px":    [4]float64{minX, minY, bw, bh},
			"cropped_width_mm":  bw,
			"cropped_height_mm": bh,
			"layer_id":          nil,
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
		"stock_item_uid":    nil,
		"children":          []any{workflow, workpiece},
	}
}

// baseStepFields returns the fields common to both step types: identity,
// visibility, the polymorphic-dispatch step_type/typelabel pair,
// selected_laser_uid/applied_recipe_uid left null (a step carries its
// speed/power/air-assist directly and doesn't need a specific machine
// laser or a material recipe bound to load with those values intact),
// speed/power/air-assist as flat fields, and an empty transformer-dict
// pair (Step.from_dict merges these with the step class's own defaults,
// so [] safely picks up whatever pipeline that step type normally runs).
//
// extra holds the step-type-specific fields (e.g. depth_mode, cut_side)
// at TWO levels at once: as flat top-level keys, matching exactly what
// that step class's own to_dict() writes (EngraveStep/ContourStep.
// to_dict() in laser_essentials/steps/{raster,contour}_step.py) — the
// primary path each subclass's from_dict reads first — and nested
// inside opsproducer_dict.params, the pre-refactor legacy path each
// from_dict falls back to if a top-level key is missing. Only the
// second was present in this generator's first attempt at this schema:
// it fixed the "KeyError: 'opsproducer_dict'" crash (that key is a hard
// requirement in the base Step.from_dict, independent of this
// fallback), but the raster step then showed "Variable" power instead
// of the requested "Constant" — evidence that whatever's actually
// compiled into the installed app (confirmed elsewhere to disagree
// with github.com/barebaric/rayforge "main" on other details too, e.g.
// raster_step.py's from_dict sits at a different line number than the
// crash traceback reported) prioritizes the top-level keys over the
// legacy fallback, at least for some fields. Writing both removes the
// ambiguity instead of betting on one path.
func baseStepFields(stepType, name, typelabel string, capabilities []string, speedMmMin, powerPct float64, airAssist bool, opsproducer map[string]any, extra map[string]any) map[string]any {
	d := map[string]any{
		"uid":                     uuid.NewString(),
		"type":                    "step",
		"step_type":               stepType,
		"name":                    name,
		"matrix":                  identityMatrix,
		"typelabel":               typelabel,
		"visible":                 true,
		"selected_laser_uid":      nil,
		"generated_workpiece_uid": nil,
		"applied_recipe_uid":      nil,
		"capabilities":            capabilities,
		"opsproducer_dict":        opsproducer,

		"per_workpiece_transformers_dicts": []any{},
		"per_step_transformers_dicts":      []any{},
		"pixels_per_mm":                    [2]int{50, 50},
		"power":                            powerPct / 100,
		"max_power":                        1.0,
		"cut_speed":                        speedMmMin,
		"max_cut_speed":                    10000,
		"travel_speed":                     5000,
		"max_travel_speed":                 10000,
		"air_assist":                       airAssist,
		"kerf_mm":                          0.0,
		"tab_power":                        0.0,
		"frequency":                        0,
		"pulse_width":                      0,
		"children":                         []any{},
	}
	maps.Copy(d, extra)
	return d
}

// engraveStepDict builds an EngraveStep — the raster-fill operation —
// backed by a Rasterizer opsproducer, with fields matching
// EngraveStep.__init__'s own defaults, including depth_mode:
// POWER_MODULATION ("Variable Power" in the UI) — confirmed correct by
// the app's owner; an earlier attempt switched this to CONSTANT_POWER
// on the theory that a flat black-fill vector glyph has no grayscale
// data to modulate against, but that reasoning was wrong for this
// setup and the actual problem was an unrelated machine max-speed
// setting. scan_mode is FULL_SWEEP rather than the class default
// SEGMENTED ("moves between content regions" — for sparse per-letter
// glyph ink, that's a lot of short start-stop jumps; "Full Sweep:
// scans full width with laser toggling" per its own tooltip in
// widgets/raster_page.py, simpler and more predictable motion for
// small text like this).
func engraveStepDict(name string, speedMmMin, powerPct float64, airAssist bool) map[string]any {
	producer := map[string]any{
		"type": "Rasterizer",
		"params": map[string]any{
			"scan_angle":         0.0,
			"depth_mode":         "POWER_MODULATION",
			"threshold":          128,
			"dither_algorithm":   "floyd_steinberg",
			"cross_hatch":        false,
			"speed":              speedMmMin,
			"min_power":          0.0,
			"max_power":          1.0,
			"num_depth_levels":   5,
			"num_power_levels":   25,
			"z_step_down":        0.0,
			"invert":             false,
			"line_interval_mm":   nil,
			"sample_interval_mm": nil,
			"black_point":        0,
			"white_point":        255,
			"auto_levels":        true,
			"angle_increment":    0.0,
		},
	}
	flat := map[string]any{
		"scan_angle":         0.0,
		"depth_mode":         "POWER_MODULATION",
		"invert":             false,
		"auto_levels":        true,
		"black_point":        0,
		"white_point":        255,
		"threshold":          128,
		"line_interval_mm":   nil,
		"sample_interval_mm": nil,
		"min_power":          0.0,
		"num_power_levels":   25,
		"offset_x_mm":        0.0,
		"offset_y_mm":        0.0,
		"scan_mode":          "FULL_SWEEP",
		"cross_hatch":        false,
		"num_depth_levels":   5,
		"z_step_down":        0.0,
		"angle_increment":    0.0,
		"dither_algorithm":   nil,
		"bidir_x_offset_mm":  0.0,
	}
	return baseStepFields("EngraveStep", name, "Engrave", []string{"ENGRAVE"}, speedMmMin, powerPct, airAssist, producer, flat)
}

// contourStepDict builds a ContourStep — a vector cut/trace operation —
// backed by a ContourProducer, with cutSide ("CENTERLINE" for tracing
// the text outline in place, "OUTSIDE" for cutting the tag free of the
// sheet along its border).
func contourStepDict(name string, speedMmMin, powerPct float64, airAssist bool, cutSide string) map[string]any {
	producer := map[string]any{
		"type": "ContourProducer",
		"params": map[string]any{
			"remove_inner_paths": false,
			"path_offset_mm":     0.0,
			"cut_side":           cutSide,
			"cut_order":          "INSIDE_OUTSIDE",
			"override_threshold": false,
			"threshold":          0.5,
		},
	}
	flat := map[string]any{
		"cut_side":           cutSide,
		"cut_order":          "INSIDE_OUTSIDE",
		"remove_inner_paths": false,
		"offset_mm":          0.0,
		"overcut":            0.0,
		"override_threshold": false,
		"threshold":          0.5,
	}
	return baseStepFields("ContourStep", name, "Contour", []string{"CUT", "SCORE"}, speedMmMin, powerPct, airAssist, producer, flat)
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

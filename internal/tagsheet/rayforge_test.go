package tagsheet

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"testing"
)

type rypDocT struct {
	Type     string      `json:"type"`
	Children []rypLayerT `json:"children"`
	Assets   []rypAssetT `json:"assets"`
}

type rypLayerT struct {
	Type     string            `json:"type"`
	Name     string            `json:"name"`
	Children []json.RawMessage `json:"children"`
}

type rypWorkflowT struct {
	Type     string     `json:"type"`
	Children []rypStepT `json:"children"`
}

type rypStepT struct {
	Type             string         `json:"type"`
	StepType         string         `json:"step_type"`
	CutSpeed         float64        `json:"cut_speed"`
	Power            float64        `json:"power"`
	AirAssist        bool           `json:"air_assist"`
	SelectedLaserUID *string        `json:"selected_laser_uid"`
	Capabilities     []string       `json:"capabilities"`
	OpsproducerDict  map[string]any `json:"opsproducer_dict"`
	// DepthMode/CutSide are read from these TOP-LEVEL keys by each step
	// subclass's from_dict before it ever looks inside opsproducer_dict —
	// verified by loading a generated file in the real app: an early
	// version of this generator wrote depth_mode only inside
	// opsproducer_dict.params and the app displayed "Variable" power
	// instead of the requested "Constant".
	DepthMode *string `json:"depth_mode"`
	CutSide   *string `json:"cut_side"`
}

type rypWorkPieceT struct {
	Type          string        `json:"type"`
	Matrix        [3][3]float64 `json:"matrix"`
	WidthMm       float64       `json:"width_mm"`
	HeightMm      float64       `json:"height_mm"`
	SourceSegment struct {
		SourceAssetUID    string     `json:"source_asset_uid"`
		CropWindowPx      [4]float64 `json:"crop_window_px"`
		VectorizationSpec struct {
			ActiveLayerIDs []string `json:"active_layer_ids"`
		} `json:"vectorization_spec"`
		PristineGeometry struct {
			Commands [][]any `json:"commands"`
		} `json:"pristine_geometry"`
	} `json:"source_segment"`
}

type rypAssetT struct {
	UID          string  `json:"uid"`
	Type         string  `json:"type"`
	OriginalData string  `json:"original_data"`
	WidthMm      float64 `json:"width_mm"`
	HeightMm     float64 `json:"height_mm"`
}

// unmarshalTypedChild decodes raw into either a Workflow or a WorkPiece
// based on its "type" field, mirroring how Rayforge's own Doc.from_dict
// dispatches polymorphic children.
func unmarshalTypedChild(t *testing.T, raw json.RawMessage) (workflow *rypWorkflowT, workpiece *rypWorkPieceT) {
	t.Helper()
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		t.Fatalf("could not probe child type: %v", err)
	}
	switch probe.Type {
	case "workflow":
		var wf rypWorkflowT
		if err := json.Unmarshal(raw, &wf); err != nil {
			t.Fatalf("could not decode workflow: %v", err)
		}
		return &wf, nil
	case "workpiece":
		var wp rypWorkPieceT
		if err := json.Unmarshal(raw, &wp); err != nil {
			t.Fatalf("could not decode workpiece: %v", err)
		}
		return nil, &wp
	default:
		t.Fatalf("unexpected layer child type %q", probe.Type)
		return nil, nil
	}
}

func TestRenderRayforge(t *testing.T) {
	sheet, err := Layout(KindLocation, []string{"@ABC"}, 1, 1, 4)
	if err != nil {
		t.Fatal(err)
	}
	zipBytes, err := RenderRayforge(sheet, DefaultCutSettings)
	if err != nil {
		t.Fatalf("RenderRayforge: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf(".ryp did not parse as a zip: %v", err)
	}
	if len(zr.File) != 1 || zr.File[0].Name != "project.json" {
		t.Fatalf("expected exactly one member named project.json, got %v", zr.File)
	}
	rc, err := zr.File[0].Open()
	if err != nil {
		t.Fatalf("opening project.json: %v", err)
	}
	defer rc.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(rc); err != nil {
		t.Fatalf("reading project.json: %v", err)
	}

	var doc rypDocT
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("project.json did not parse: %v", err)
	}

	if doc.Type != "doc" {
		t.Errorf("doc.Type = %q, want \"doc\"", doc.Type)
	}
	if len(doc.Children) != 3 {
		t.Fatalf("got %d layers, want 3", len(doc.Children))
	}
	if len(doc.Assets) != 1 || doc.Assets[0].Type != "source" {
		t.Fatalf("assets = %+v, want exactly one type=source asset", doc.Assets)
	}

	asset := doc.Assets[0]
	svgBytes, err := base64.StdEncoding.DecodeString(asset.OriginalData)
	if err != nil {
		t.Fatalf("asset original_data did not decode as base64: %v", err)
	}
	if got, want := string(svgBytes), RenderSVG(sheet); got != want {
		t.Errorf("asset original_data does not match RenderSVG(sheet)")
	}
	if asset.WidthMm != sheet.WidthMm || asset.HeightMm != sheet.HeightMm {
		t.Errorf("asset dims = %vx%v, want %vx%v", asset.WidthMm, asset.HeightMm, sheet.WidthMm, sheet.HeightMm)
	}

	wantLayers := []struct {
		name         string
		layerID      string
		stepType     string
		producerType string
		speed        float64
		power        float64
		air          bool
		depthMode    string // "" if not applicable to this step type
		cutSide      string // "" if not applicable to this step type
	}{
		{"Raster Text", "text-fill", "EngraveStep", "Rasterizer", DefaultCutSettings.RasterSpeedMmMin, DefaultCutSettings.RasterPowerPct / 100, DefaultCutSettings.RasterAirAssist, "POWER_MODULATION", ""},
		{"Outline Text", "text-outline", "ContourStep", "ContourProducer", DefaultCutSettings.OutlineSpeedMmMin, DefaultCutSettings.OutlinePowerPct / 100, DefaultCutSettings.OutlineAirAssist, "", "CENTERLINE"},
		{"Cut Tag", "cut", "ContourStep", "ContourProducer", DefaultCutSettings.CutSpeedMmMin, DefaultCutSettings.CutPowerPct / 100, DefaultCutSettings.CutAirAssist, "", "OUTSIDE"},
	}

	for i, layer := range doc.Children {
		want := wantLayers[i]
		if layer.Type != "layer" {
			t.Errorf("layer[%d].Type = %q, want \"layer\"", i, layer.Type)
		}
		if layer.Name != want.name {
			t.Errorf("layer[%d].Name = %q, want %q", i, layer.Name, want.name)
		}
		if len(layer.Children) != 2 {
			t.Fatalf("layer[%d] has %d children, want 2 (workflow, workpiece)", i, len(layer.Children))
		}

		workflow, workpiece := unmarshalTypedChild(t, layer.Children[0])
		if workflow == nil {
			t.Fatalf("layer[%d].children[0] is not a workflow", i)
		}
		_, wp := unmarshalTypedChild(t, layer.Children[1])
		if wp == nil {
			t.Fatalf("layer[%d].children[1] is not a workpiece", i)
		}
		workpiece = wp

		if len(workflow.Children) != 1 {
			t.Fatalf("layer[%d] workflow has %d steps, want 1", i, len(workflow.Children))
		}
		step := workflow.Children[0]
		if step.StepType != want.stepType {
			t.Errorf("layer[%d] step.step_type = %q, want %q", i, step.StepType, want.stepType)
		}
		if step.CutSpeed != want.speed {
			t.Errorf("layer[%d] step.cut_speed = %v, want %v", i, step.CutSpeed, want.speed)
		}
		if step.Power != want.power {
			t.Errorf("layer[%d] step.power = %v, want %v", i, step.Power, want.power)
		}
		if step.AirAssist != want.air {
			t.Errorf("layer[%d] step.air_assist = %v, want %v", i, step.AirAssist, want.air)
		}
		if step.SelectedLaserUID != nil {
			t.Errorf("layer[%d] step.selected_laser_uid = %v, want null (no machine-specific laser bound)", i, *step.SelectedLaserUID)
		}
		// opsproducer_dict is the field that broke the very first version of
		// this generator: Rayforge's installed-app schema requires it
		// (unconditional dict access in Step.from_dict), unlike the flat
		// fields its still-unreleased "main" branch schema uses instead.
		if step.OpsproducerDict == nil {
			t.Fatalf("layer[%d] step has no opsproducer_dict — this is REQUIRED by the installed app, its absence is exactly what produced \"KeyError: 'opsproducer_dict'\" the first time", i)
		}
		if got, _ := step.OpsproducerDict["type"].(string); got != want.producerType {
			t.Errorf("layer[%d] step.opsproducer_dict.type = %q, want %q", i, got, want.producerType)
		}
		if _, ok := step.OpsproducerDict["params"].(map[string]any); !ok {
			t.Errorf("layer[%d] step.opsproducer_dict.params is missing or not an object", i)
		}
		// depth_mode/cut_side must be set at the TOP level of the step dict,
		// not only inside opsproducer_dict.params — each step subclass's
		// from_dict reads the top-level key first and only falls back to
		// the legacy opsproducer_dict payload if it's absent.
		if want.depthMode != "" {
			if step.DepthMode == nil || *step.DepthMode != want.depthMode {
				t.Errorf("layer[%d] step.depth_mode (top-level) = %v, want %q", i, step.DepthMode, want.depthMode)
			}
		}
		if want.cutSide != "" {
			if step.CutSide == nil || *step.CutSide != want.cutSide {
				t.Errorf("layer[%d] step.cut_side (top-level) = %v, want %q", i, step.CutSide, want.cutSide)
			}
		}
		if len(step.Capabilities) == 0 {
			t.Errorf("layer[%d] step.capabilities is empty", i)
		}

		seg := workpiece.SourceSegment
		if seg.SourceAssetUID != asset.UID {
			t.Errorf("layer[%d] workpiece.source_segment.source_asset_uid = %q, want %q", i, seg.SourceAssetUID, asset.UID)
		}
		if activeIDs := seg.VectorizationSpec.ActiveLayerIDs; len(activeIDs) != 1 || activeIDs[0] != want.layerID {
			t.Errorf("layer[%d] workpiece.source_segment.vectorization_spec.active_layer_ids = %v, want [%q]", i, activeIDs, want.layerID)
		}
		if len(seg.PristineGeometry.Commands) == 0 {
			t.Errorf("layer[%d] workpiece has no geometry commands", i)
		}

		// The world matrix's scale (matrix[0][0], matrix[1][1]) must match
		// the workpiece's own width_mm/height_mm — Rayforge positions a
		// workpiece by scaling a unit square, so a mismatch here means the
		// shape will render at the wrong size.
		if workpiece.Matrix[0][0] != workpiece.WidthMm {
			t.Errorf("layer[%d] matrix[0][0] = %v, want width_mm %v", i, workpiece.Matrix[0][0], workpiece.WidthMm)
		}
		if workpiece.Matrix[1][1] != workpiece.HeightMm {
			t.Errorf("layer[%d] matrix[1][1] = %v, want height_mm %v", i, workpiece.Matrix[1][1], workpiece.HeightMm)
		}
		if seg.CropWindowPx[2] != workpiece.WidthMm || seg.CropWindowPx[3] != workpiece.HeightMm {
			t.Errorf("layer[%d] crop_window_px size = (%v,%v), want (%v,%v)", i, seg.CropWindowPx[2], seg.CropWindowPx[3], workpiece.WidthMm, workpiece.HeightMm)
		}

		// The workpiece's world position must place it within the sheet —
		// matrix[0][2]/matrix[1][2] are its (x,y) origin.
		const tol = 2.0 // bezier/arc control-point overshoot allowance
		x, y := workpiece.Matrix[0][2], workpiece.Matrix[1][2]
		if x < -tol || x+workpiece.WidthMm > sheet.WidthMm+tol {
			t.Errorf("layer[%d] workpiece x-range [%v,%v] falls outside sheet width %v", i, x, x+workpiece.WidthMm, sheet.WidthMm)
		}
		if y < -tol || y+workpiece.HeightMm > sheet.HeightMm+tol {
			t.Errorf("layer[%d] workpiece y-range [%v,%v] falls outside sheet height %v", i, y, y+workpiece.HeightMm, sheet.HeightMm)
		}
	}
}

package tagsheet

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/pborges/rayforge"
)

// The detailed .ryp JSON shape (opsproducer_dict, top-level vs nested
// fields, matrices, ...) is github.com/pborges/rayforge's own concern
// and covered by its test suite. This package only needs to verify
// its own integration: that CutSettings values reach the right steps,
// the three layers/paths are wired up correctly, and the path-type
// conversion at the module boundary (toRayforgePath) is correct.

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
	Type      string  `json:"type"`
	StepType  string  `json:"step_type"`
	CutSpeed  float64 `json:"cut_speed"`
	Power     float64 `json:"power"`
	AirAssist bool    `json:"air_assist"`
}

type rypWorkPieceT struct {
	Type          string        `json:"type"`
	Matrix        [3][3]float64 `json:"matrix"`
	WidthMm       float64       `json:"width_mm"`
	HeightMm      float64       `json:"height_mm"`
	SourceSegment struct {
		VectorizationSpec struct {
			ActiveLayerIDs []string `json:"active_layer_ids"`
		} `json:"vectorization_spec"`
	} `json:"source_segment"`
}

type rypAssetT struct {
	Type         string  `json:"type"`
	OriginalData string  `json:"original_data"`
	WidthMm      float64 `json:"width_mm"`
	HeightMm     float64 `json:"height_mm"`
}

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
		name     string
		layerID  string
		stepType string
		speed    float64
		power    float64
		air      bool
	}{
		{"Raster Text", "text-fill", "EngraveStep", DefaultCutSettings.RasterSpeedMmMin, DefaultCutSettings.RasterPowerPct / 100, DefaultCutSettings.RasterAirAssist},
		{"Outline Text", "text-outline", "ContourStep", DefaultCutSettings.OutlineSpeedMmMin, DefaultCutSettings.OutlinePowerPct / 100, DefaultCutSettings.OutlineAirAssist},
		{"Cut Tag", "cut", "ContourStep", DefaultCutSettings.CutSpeedMmMin, DefaultCutSettings.CutPowerPct / 100, DefaultCutSettings.CutAirAssist},
	}

	for i, layer := range doc.Children {
		want := wantLayers[i]
		if layer.Name != want.name {
			t.Errorf("layer[%d].Name = %q, want %q", i, layer.Name, want.name)
		}
		if len(layer.Children) != 2 {
			t.Fatalf("layer[%d] has %d children, want 2 (workflow, workpiece)", i, len(layer.Children))
		}

		workflow, _ := unmarshalTypedChild(t, layer.Children[0])
		if workflow == nil {
			t.Fatalf("layer[%d].children[0] is not a workflow", i)
		}
		_, workpiece := unmarshalTypedChild(t, layer.Children[1])
		if workpiece == nil {
			t.Fatalf("layer[%d].children[1] is not a workpiece", i)
		}
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

		if activeIDs := workpiece.SourceSegment.VectorizationSpec.ActiveLayerIDs; len(activeIDs) != 1 || activeIDs[0] != want.layerID {
			t.Errorf("layer[%d] workpiece active_layer_ids = %v, want [%q]", i, activeIDs, want.layerID)
		}
		if workpiece.Matrix[0][0] != workpiece.WidthMm || workpiece.Matrix[1][1] != workpiece.HeightMm {
			t.Errorf("layer[%d] matrix scale (%v,%v) doesn't match width/height_mm (%v,%v)", i,
				workpiece.Matrix[0][0], workpiece.Matrix[1][1], workpiece.WidthMm, workpiece.HeightMm)
		}
	}
}

func TestToRayforgePath(t *testing.T) {
	src := Path{
		{Op: OpMove, Pts: [3]Point{{X: 1, Y: 2}}},
		{Op: OpLine, Pts: [3]Point{{X: 3, Y: 4}}},
		{Op: OpQuad, Pts: [3]Point{{X: 5, Y: 6}, {X: 7, Y: 8}}},
		{Op: OpCube, Pts: [3]Point{{X: 9, Y: 10}, {X: 11, Y: 12}, {X: 13, Y: 14}}},
	}
	got := toRayforgePath(src)
	want := rayforge.Path{
		rayforge.Move(rayforge.Point{X: 1, Y: 2}),
		rayforge.Line(rayforge.Point{X: 3, Y: 4}),
		rayforge.Quad(rayforge.Point{X: 5, Y: 6}, rayforge.Point{X: 7, Y: 8}),
		rayforge.Cubic(rayforge.Point{X: 9, Y: 10}, rayforge.Point{X: 11, Y: 12}, rayforge.Point{X: 13, Y: 14}),
	}
	if len(got) != len(want) {
		t.Fatalf("got %d segments, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("segment[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

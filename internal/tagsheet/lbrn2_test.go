package tagsheet

import (
	"encoding/xml"
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

type lbrnDoc struct {
	XMLName     xml.Name         `xml:"LightBurnProject"`
	CutSettings []lbrnCutSetting `xml:"CutSetting"`
	Shapes      []lbrnShapeTag   `xml:"Shape"`
}

type lbrnCutSetting struct {
	Type      string         `xml:"type,attr"`
	Index     lbrnValueInt   `xml:"index"`
	MaxPower  lbrnValueFloat `xml:"maxPower"`
	Speed     lbrnValueFloat `xml:"speed"`
	RunBlower lbrnValueInt   `xml:"runBlower"`
	Priority  lbrnValueInt   `xml:"priority"`
}

type lbrnValueInt struct {
	Value int `xml:"Value,attr"`
}

type lbrnValueFloat struct {
	Value float64 `xml:"Value,attr"`
}

type lbrnShapeTag struct {
	Type     string `xml:"Type,attr"`
	CutIndex int    `xml:"CutIndex,attr"`
	XForm    string `xml:"XForm"`
	VertList string `xml:"VertList"`
	PrimList string `xml:"PrimList"`
}

var (
	vertTokenRe = regexp.MustCompile(`V-?[\d.]+ -?[\d.]+(?:c0x-?[\d.]+c0y-?[\d.]+|c0x1)(?:c1x-?[\d.]+c1y-?[\d.]+|c1x1)`)
	primTokenRe = regexp.MustCompile(`[LB]\d+ \d+`)
)

// decomposeFully tokenizes s with re and reports whether the concatenated
// tokens reconstruct s exactly — i.e. the whole string decomposes cleanly
// into tokens with nothing left over.
func decomposeFully(t *testing.T, re *regexp.Regexp, s string) []string {
	t.Helper()
	tokens := re.FindAllString(s, -1)
	if strings.Join(tokens, "") != s {
		t.Fatalf("string did not fully decompose into expected tokens: %q", s)
	}
	return tokens
}

func TestRenderLBRN2(t *testing.T) {
	sheet, err := Layout(KindLocation, []string{"@ABC"}, 1, 1, 4)
	if err != nil {
		t.Fatal(err)
	}
	out := RenderLBRN2(sheet, DefaultCutSettings)

	var doc lbrnDoc
	if err := xml.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("lbrn2 did not parse: %v", err)
	}

	if len(doc.CutSettings) != 3 {
		t.Fatalf("got %d CutSettings, want 3", len(doc.CutSettings))
	}
	wantTypes := []string{"Scan", "Cut", "Cut"}
	wantSpeedMmMin := []float64{DefaultCutSettings.RasterSpeedMmMin, DefaultCutSettings.OutlineSpeedMmMin, DefaultCutSettings.CutSpeedMmMin}
	wantPowerPct := []float64{DefaultCutSettings.RasterPowerPct, DefaultCutSettings.OutlinePowerPct, DefaultCutSettings.CutPowerPct}
	wantAirAssist := []bool{DefaultCutSettings.RasterAirAssist, DefaultCutSettings.OutlineAirAssist, DefaultCutSettings.CutAirAssist}
	for i, cs := range doc.CutSettings {
		if cs.Type != wantTypes[i] {
			t.Errorf("CutSettings[%d].Type = %q, want %q", i, cs.Type, wantTypes[i])
		}
		if cs.Index.Value != i {
			t.Errorf("CutSettings[%d].index = %d, want %d", i, cs.Index.Value, i)
		}
		if cs.Priority.Value != i {
			t.Errorf("CutSettings[%d].priority = %d, want %d", i, cs.Priority.Value, i)
		}
		if cs.MaxPower.Value != wantPowerPct[i] {
			t.Errorf("CutSettings[%d].maxPower = %v, want %v", i, cs.MaxPower.Value, wantPowerPct[i])
		}
		if wantSpeed := mmPerMinToMmPerSec(wantSpeedMmMin[i]); math.Abs(cs.Speed.Value-wantSpeed) > 1e-3 {
			t.Errorf("CutSettings[%d].speed = %v, want ≈%v (mm/sec, rounded to 4 decimals)", i, cs.Speed.Value, wantSpeed)
		}
		wantBlower := 0
		if wantAirAssist[i] {
			wantBlower = 1
		}
		if cs.RunBlower.Value != wantBlower {
			t.Errorf("CutSettings[%d].runBlower = %d, want %d", i, cs.RunBlower.Value, wantBlower)
		}
	}

	if len(doc.Shapes) == 0 {
		t.Fatal("no shapes rendered")
	}

	cutIndexCounts := map[int]int{}
	for _, sh := range doc.Shapes {
		cutIndexCounts[sh.CutIndex]++

		if sh.Type != "Path" {
			t.Errorf("shape Type = %q, want Path", sh.Type)
		}
		if sh.CutIndex < 0 || sh.CutIndex > 2 {
			t.Errorf("shape CutIndex = %d, want in [0,2]", sh.CutIndex)
		}
		if strings.TrimSpace(sh.XForm) != "1 0 0 1 0 0" {
			t.Errorf("shape XForm = %q, want the identity matrix", sh.XForm)
		}

		verts := decomposeFully(t, vertTokenRe, sh.VertList)
		if len(verts) == 0 {
			t.Error("shape has an empty VertList")
		}
		for _, v := range verts {
			m := vertTokenRe.FindStringSubmatch(v)
			x, errX := strconv.ParseFloat(strings.TrimPrefix(strings.SplitN(v, " ", 2)[0], "V"), 64)
			ySpaceSplit := strings.SplitN(v, " ", 2)[1]
			// y is followed immediately by "c0x..." or "c1x..." — cut it
			// at the first non-numeric rune after the leading sign/digits/dot.
			yEnd := 0
			for yEnd < len(ySpaceSplit) && (ySpaceSplit[yEnd] == '-' || ySpaceSplit[yEnd] == '.' || (ySpaceSplit[yEnd] >= '0' && ySpaceSplit[yEnd] <= '9')) {
				yEnd++
			}
			y, errY := strconv.ParseFloat(ySpaceSplit[:yEnd], 64)
			if errX != nil || errY != nil || m == nil {
				t.Errorf("could not parse vertex token %q", v)
				continue
			}
			const tol = 2.0 // bezier control-point overshoot allowance
			if x < -tol || x > sheet.WidthMm+tol || y < -tol || y > sheet.HeightMm+tol {
				t.Errorf("vertex (%v, %v) falls outside sheet bounds (0,0)-(%v,%v)", x, y, sheet.WidthMm, sheet.HeightMm)
			}
		}

		prims := decomposeFully(t, primTokenRe, sh.PrimList)
		if len(prims) != len(verts) {
			t.Errorf("got %d prims for %d verts, want equal counts (one outgoing prim per vertex)", len(prims), len(verts))
		}
		for _, p := range prims {
			if p[0] != 'L' && p[0] != 'B' {
				t.Errorf("prim token %q has an unexpected op", p)
			}
		}
		if last := prims[len(prims)-1]; !strings.HasSuffix(last, " 0") {
			t.Errorf("last prim %q does not close back to vertex 0", last)
		}
	}

	if cutIndexCounts[2] != len(sheet.Tags) {
		t.Errorf("got %d CutIndex=2 (tag boundary) shapes, want %d (one per tag)", cutIndexCounts[2], len(sheet.Tags))
	}
	if cutIndexCounts[0] != cutIndexCounts[1] {
		t.Errorf("CutIndex 0 (fill) and 1 (outline) shape counts differ: %d vs %d — glyph contours should be emitted on both", cutIndexCounts[0], cutIndexCounts[1])
	}
}

package tagsheet

import (
	"encoding/xml"
	"testing"
)

type svgDoc struct {
	XMLName xml.Name   `xml:"svg"`
	Width   string     `xml:"width,attr"`
	Height  string     `xml:"height,attr"`
	ViewBox string     `xml:"viewBox,attr"`
	Groups  []svgGroup `xml:"g"`
}

type svgGroup struct {
	ID     string       `xml:"id,attr"`
	Fill   string       `xml:"fill,attr"`
	Stroke string       `xml:"stroke,attr"`
	Paths  []svgPathTag `xml:"path"`
}

type svgPathTag struct {
	D string `xml:"d,attr"`
}

func TestRenderSVG(t *testing.T) {
	sheet, err := Layout(KindAsset, []string{"AAAA", "BBBB"}, 1, 2, 4)
	if err != nil {
		t.Fatal(err)
	}
	out := RenderSVG(sheet)

	var doc svgDoc
	if err := xml.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("SVG did not parse: %v", err)
	}

	if doc.Width != "132mm" {
		t.Errorf("width = %q, want 132mm", doc.Width)
	}
	if doc.Height != "34mm" {
		t.Errorf("height = %q, want 34mm", doc.Height)
	}
	if doc.ViewBox != "0 0 132 34" {
		t.Errorf("viewBox = %q, want '0 0 132 34'", doc.ViewBox)
	}

	if len(doc.Groups) != 2 {
		t.Fatalf("got %d <g> groups, want 2", len(doc.Groups))
	}
	fill, cut := doc.Groups[0], doc.Groups[1]

	if fill.ID != "text-fill" || fill.Fill != "#000000" {
		t.Errorf("text-fill group = %+v", fill)
	}
	if cut.ID != "cut" || cut.Stroke != "#FF0000" {
		t.Errorf("cut group = %+v", cut)
	}

	wantPaths := 0
	for _, tag := range sheet.Tags {
		wantPaths += len(tag.Text)
	}
	if len(fill.Paths) != wantPaths {
		t.Errorf("text-fill has %d <path>, want %d", len(fill.Paths), wantPaths)
	}
	for _, p := range fill.Paths {
		if p.D == "" {
			t.Error("path has an empty d attribute")
		}
	}

	if len(cut.Paths) != len(sheet.Tags) {
		t.Errorf("cut has %d <path>, want %d (one per tag)", len(cut.Paths), len(sheet.Tags))
	}
	// The cut boundary starts (and closes) on the bottom edge next to
	// the bottom-right corner, not the top-left, so the laser's
	// start/end seam lands on the least conspicuous corner of a
	// mounted tag.
	for i, tag := range sheet.Tags {
		wantStart := "M" + formatNum(tag.X+TagWidthMm-TagCornerMm) + "," + formatNum(tag.Y+TagHeightMm)
		if got := cut.Paths[i].D; len(got) < len(wantStart) || got[:len(wantStart)] != wantStart {
			t.Errorf("cut path[%d].d starts %q, want prefix %q", i, got, wantStart)
		}
	}
}

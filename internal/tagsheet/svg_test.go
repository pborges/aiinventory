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
	Rects  []svgRectTag `xml:"rect"`
}

type svgPathTag struct {
	D string `xml:"d,attr"`
}

type svgRectTag struct {
	X  string `xml:"x,attr"`
	Y  string `xml:"y,attr"`
	Rx string `xml:"rx,attr"`
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

	if doc.Width != "124mm" {
		t.Errorf("width = %q, want 124mm", doc.Width)
	}
	if doc.Height != "26mm" {
		t.Errorf("height = %q, want 26mm", doc.Height)
	}
	if doc.ViewBox != "0 0 124 26" {
		t.Errorf("viewBox = %q, want '0 0 124 26'", doc.ViewBox)
	}

	if len(doc.Groups) != 3 {
		t.Fatalf("got %d <g> groups, want 3", len(doc.Groups))
	}
	fill, outline, cut := doc.Groups[0], doc.Groups[1], doc.Groups[2]

	if fill.ID != "text-fill" || fill.Fill != "#000000" {
		t.Errorf("text-fill group = %+v", fill)
	}
	if outline.ID != "text-outline" || outline.Stroke != "#0000FF" {
		t.Errorf("text-outline group = %+v", outline)
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
	if len(outline.Paths) != wantPaths {
		t.Errorf("text-outline has %d <path>, want %d", len(outline.Paths), wantPaths)
	}
	for _, p := range fill.Paths {
		if p.D == "" {
			t.Error("path has an empty d attribute")
		}
	}

	if len(cut.Rects) != len(sheet.Tags) {
		t.Errorf("cut has %d <rect>, want %d (one per tag)", len(cut.Rects), len(sheet.Tags))
	}
	for i, r := range cut.Rects {
		if r.Rx != "2" {
			t.Errorf("rect[%d].rx = %q, want 2", i, r.Rx)
		}
	}
}

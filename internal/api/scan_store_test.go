package api

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestSaveScanWritesImageAndSidecar(t *testing.T) {
	dir := t.TempDir()
	s := &Server{scanStoreDir: dir}

	s.saveScan("locate", []byte("fake-jpeg-bytes"), []string{"@XYZ", "ZKEI", "GKEI"})

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d files, want 2 (image + sidecar)", len(entries))
	}

	idPattern := regexp.MustCompile(`^locate-([0-9a-f]{16})\.(jpg|txt)$`)
	var id string
	for _, e := range entries {
		m := idPattern.FindStringSubmatch(e.Name())
		if m == nil {
			t.Fatalf("unexpected file name %q", e.Name())
		}
		if id == "" {
			id = m[1]
		} else if id != m[1] {
			t.Errorf("image and sidecar have different ids: %q vs %q", id, m[1])
		}
	}

	img, err := os.ReadFile(filepath.Join(dir, "locate-"+id+".jpg"))
	if err != nil {
		t.Fatalf("read saved image: %v", err)
	}
	if string(img) != "fake-jpeg-bytes" {
		t.Errorf("saved image = %q, want %q", img, "fake-jpeg-bytes")
	}

	txt, err := os.ReadFile(filepath.Join(dir, "locate-"+id+".txt"))
	if err != nil {
		t.Fatalf("read saved sidecar: %v", err)
	}
	if want := "@XYZ\nZKEI\nGKEI\n"; string(txt) != want {
		t.Errorf("saved sidecar = %q, want %q", txt, want)
	}
}

func TestSaveScanSidecarEmptyWhenNothingFound(t *testing.T) {
	dir := t.TempDir()
	s := &Server{scanStoreDir: dir}

	s.saveScan("item", []byte("data"), nil)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".txt" {
			continue
		}
		txt, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read sidecar: %v", err)
		}
		if len(txt) != 0 {
			t.Errorf("sidecar = %q, want empty", txt)
		}
	}
}

func TestSaveScanNoopWhenUnconfigured(t *testing.T) {
	s := &Server{scanStoreDir: ""}
	// must not panic or attempt to write anywhere
	s.saveScan("locate", []byte("data"), []string{"@XYZ"})
}

package api

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// saveScan best-effort writes a resized scan image (already cropped/
// downsized/JPEG-encoded client-side — see web/src/lib/camera.ts) plus a
// text sidecar listing whatever tags/location code Gemini reported for it,
// to scanStoreDir for later inspection of OCR misreads/hallucinations.
// Files share one random id: "<prefix>-<id>.jpg" and "<prefix>-<id>.txt"
// (one tag/code per line; empty file if nothing was found). found is the
// raw model output — pass it in unfiltered by any deterministic validation,
// so the sidecar shows what Gemini actually claimed, not what survived it.
// A no-op if scanStoreDir wasn't configured (the -store flag — see
// internal/config). Failures are logged, not returned: this is a
// diagnostic side channel and must never affect the request it's attached to.
func (s *Server) saveScan(prefix string, image []byte, found []string) {
	if s.scanStoreDir == "" {
		return
	}
	id := randomScanID()

	imgPath := filepath.Join(s.scanStoreDir, fmt.Sprintf("%s-%s.jpg", prefix, id))
	if err := os.WriteFile(imgPath, image, 0o644); err != nil {
		log.Printf("scan store: failed to save %s: %v", imgPath, err)
	}

	txtPath := filepath.Join(s.scanStoreDir, fmt.Sprintf("%s-%s.txt", prefix, id))
	text := ""
	if len(found) > 0 {
		text = strings.Join(found, "\n") + "\n"
	}
	if err := os.WriteFile(txtPath, []byte(text), 0o644); err != nil {
		log.Printf("scan store: failed to save %s: %v", txtPath, err)
	}
}

func randomScanID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		log.Printf("scan store: random id generation failed: %v", err)
		return "unknown"
	}
	return hex.EncodeToString(buf)
}

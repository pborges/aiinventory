package api

import (
	"io"
	"net/http"
	"regexp"
	"strings"
)

// maxUploadTextBytes caps the registry .txt bulk-upload — plenty for a tag
// list, mirrors readUploadedImage's cap pattern in capture_handlers.go.
const maxUploadTextBytes = 1 << 20 // 1MB

// readUploadedTextFile reads a "file" multipart field as raw bytes — the
// registry bulk-upload's counterpart to readUploadedImage.
func readUploadedTextFile(r *http.Request) ([]byte, error) {
	if err := r.ParseMultipartForm(maxUploadTextBytes); err != nil {
		return nil, err
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return io.ReadAll(io.LimitReader(file, maxUploadTextBytes))
}

// parseTagLines splits data into non-blank, trimmed, uppercased lines and
// classifies each against pattern. Every bulk-upload line must pass this
// deterministic shape check before any of them get written — a garbled file
// fails loudly (naming the bad lines) rather than silently seeding the
// registry with junk.
func parseTagLines(data []byte, pattern *regexp.Regexp) (valid, invalid []string) {
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.ToUpper(strings.TrimSpace(line))
		if line == "" {
			continue
		}
		if pattern.MatchString(line) {
			valid = append(valid, line)
		} else {
			invalid = append(invalid, line)
		}
	}
	return valid, invalid
}

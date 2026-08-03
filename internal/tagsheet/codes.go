package tagsheet

import (
	"fmt"
	"io"
)

// GenerateCodes draws n unique, previously-unseen tag codes: prefix +
// letters uppercase letters, e.g. letters=4, prefix="" for asset tags
// ("ABCD") or letters=3, prefix="@" for location tags ("@ABC"). rnd is
// injectable so tests can use a deterministic source; production callers
// pass crypto/rand.Reader. exclude holds every code that must not be
// produced (the full set of already-registered/known tags) — codes is
// checked against exclude plus every code already generated within this
// call, so the result never collides with anything already in use.
func GenerateCodes(rnd io.Reader, n, letters int, prefix string, exclude map[string]struct{}) ([]string, error) {
	out := make([]string, 0, n)
	seen := make(map[string]struct{}, n)

	maxAttempts := 1000 + 200*n
	for attempt := 0; len(out) < n; attempt++ {
		if attempt >= maxAttempts {
			return nil, fmt.Errorf("tagsheet: code space exhausted after %d attempts generating %d of %d unique codes", attempt, len(out), n)
		}
		code, err := randomCode(rnd, letters, prefix)
		if err != nil {
			return nil, err
		}
		if _, dup := seen[code]; dup {
			continue
		}
		if _, known := exclude[code]; known {
			continue
		}
		seen[code] = struct{}{}
		out = append(out, code)
	}
	return out, nil
}

// randomCode draws one prefix+letters-uppercase-letter code using
// per-character rejection sampling: a byte in [0, 234) maps unbiased onto
// 26 letters (234 = 9*26, the largest multiple of 26 not exceeding 256),
// and bytes >= 234 are discarded and redrawn rather than reduced mod 26,
// which would otherwise skew the first few letters slightly more likely.
func randomCode(rnd io.Reader, letters int, prefix string) (string, error) {
	const rejectionCeiling = 234 // 9*26, largest multiple of 26 <= 256

	buf := make([]byte, letters)
	for i := 0; i < letters; {
		var b [1]byte
		if _, err := io.ReadFull(rnd, b[:]); err != nil {
			return "", fmt.Errorf("tagsheet: read random byte: %w", err)
		}
		if b[0] >= rejectionCeiling {
			continue
		}
		buf[i] = 'A' + b[0]%26
		i++
	}
	return prefix + string(buf), nil
}

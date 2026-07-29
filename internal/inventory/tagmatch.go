package inventory

import (
	"sort"
)

// HammingDistance returns the number of positions at which a and b differ.
// Both callers only ever compare already shape-validated 4-char tags, but
// unequal lengths degrade to "very different" (every extra character counts
// as a mismatch) rather than panicking.
func HammingDistance(a, b string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	dist := 0
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			dist++
		}
	}
	return dist + (len(a) - n) + (len(b) - n)
}

type TagMatchStatus string

const (
	TagStatusExact     TagMatchStatus = "exact"     // distance 0
	TagStatusCorrected TagMatchStatus = "corrected" // unique registered tag at distance 1
	TagStatusAmbiguous TagMatchStatus = "ambiguous" // tie at distance 1, or candidates only at distance 2
	TagStatusNoMatch   TagMatchStatus = "no_match"  // nothing within suggestCandidateDistance
)

// suggestCandidateDistance is the farthest a registered tag can be from a
// raw OCR read and still be offered as a candidate: distance 1 covers "one
// confused letter" (auto-applied when unique), distance 2 is surfaced as a
// weaker suggestion but never auto-applied.
const suggestCandidateDistance = 2

// maxCandidates caps how many suggestions ResolveTag returns.
const maxCandidates = 5

// TagMatch is one raw OCR read's classification against the tag registry.
type TagMatch struct {
	Raw    string
	Status TagMatchStatus
	// Resolved is set only for TagStatusExact/TagStatusCorrected — the tag
	// to actually use in place of Raw (equal to Raw for TagStatusExact).
	Resolved string
	// Candidates lists registered tags within suggestCandidateDistance of
	// Raw, closest first — what an ambiguous/no-match choice UI offers
	// besides Raw itself and free text. Empty for TagStatusNoMatch.
	Candidates []string
}

// ResolveTag classifies raw (an already shape-validated 4-char asset tag)
// against registered, the full current tag registry. Deliberately dumb and
// deterministic — no AI, no fuzzy-string library — because the point is a
// second, independent check that doesn't share Gemini's failure modes.
func ResolveTag(raw string, registered []string) TagMatch {
	for _, tag := range registered {
		if tag == raw {
			return TagMatch{Raw: raw, Status: TagStatusExact, Resolved: raw}
		}
	}

	var atDistance1, candidates []string
	for _, tag := range registered {
		if d := HammingDistance(raw, tag); d <= suggestCandidateDistance {
			candidates = append(candidates, tag)
			if d == 1 {
				atDistance1 = append(atDistance1, tag)
			}
		}
	}

	if len(atDistance1) == 1 {
		return TagMatch{Raw: raw, Status: TagStatusCorrected, Resolved: atDistance1[0]}
	}
	if len(candidates) == 0 {
		return TagMatch{Raw: raw, Status: TagStatusNoMatch}
	}

	sort.Slice(candidates, func(i, j int) bool {
		di, dj := HammingDistance(raw, candidates[i]), HammingDistance(raw, candidates[j])
		if di != dj {
			return di < dj
		}
		return candidates[i] < candidates[j]
	})
	if len(candidates) > maxCandidates {
		candidates = candidates[:maxCandidates]
	}
	return TagMatch{Raw: raw, Status: TagStatusAmbiguous, Candidates: candidates}
}

// ResolveTags resolves every raw OCR read in raws against registered — the
// full current tag registry, already fetched by the caller (one query,
// shared across every raw read, whether that's a handful of asset tags or
// the single location tag in a frame).
func ResolveTags(raws []string, registered []string) []TagMatch {
	out := make([]TagMatch, 0, len(raws))
	for _, raw := range raws {
		out = append(out, ResolveTag(raw, registered))
	}
	return out
}

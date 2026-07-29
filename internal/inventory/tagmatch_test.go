package inventory

import (
	"reflect"
	"testing"
)

func TestHammingDistance(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"ZKEI", "ZKEI", 0},
		{"QORB", "OORB", 1},
		{"AAAA", "AAAB", 1},
		{"AAAA", "BBBB", 4},
		{"AB", "ABCD", 2}, // unequal length: extra chars count as mismatches
	}
	for _, c := range cases {
		if got := HammingDistance(c.a, c.b); got != c.want {
			t.Errorf("HammingDistance(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestResolveTagExact(t *testing.T) {
	m := ResolveTag("OORB", []string{"OORB", "ZKEI"})
	if m.Status != TagStatusExact || m.Resolved != "OORB" {
		t.Fatalf("ResolveTag exact = %+v", m)
	}
}

func TestResolveTagUniqueDistance1Correction(t *testing.T) {
	// The QORB -> OORB example: registry has only one tag, one letter off.
	m := ResolveTag("QORB", []string{"OORB"})
	if m.Status != TagStatusCorrected || m.Resolved != "OORB" {
		t.Fatalf("ResolveTag corrected = %+v, want Status=corrected Resolved=OORB", m)
	}
}

func TestResolveTagAmbiguousTiedDistance1(t *testing.T) {
	// Two registered tags both one letter off from the raw read.
	m := ResolveTag("QORB", []string{"OORB", "QIRB"})
	if m.Status != TagStatusAmbiguous {
		t.Fatalf("ResolveTag tie = %+v, want Status=ambiguous", m)
	}
	want := []string{"OORB", "QIRB"}
	got := append([]string{}, m.Candidates...)
	sortStrings(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Candidates = %v, want %v", got, want)
	}
}

func TestResolveTagAmbiguousDistance2Only(t *testing.T) {
	m := ResolveTag("QORB", []string{"QAAB"}) // differs at positions 1,2 -> distance 2
	if m.Status != TagStatusAmbiguous {
		t.Fatalf("ResolveTag distance-2-only = %+v, want Status=ambiguous", m)
	}
	if len(m.Candidates) != 1 || m.Candidates[0] != "QAAB" {
		t.Fatalf("Candidates = %v, want [QAAB]", m.Candidates)
	}
}

func TestResolveTagNoMatch(t *testing.T) {
	m := ResolveTag("QORB", []string{"ZZZZ"}) // distance 4, outside suggestCandidateDistance
	if m.Status != TagStatusNoMatch {
		t.Fatalf("ResolveTag far = %+v, want Status=no_match", m)
	}
	if len(m.Candidates) != 0 {
		t.Fatalf("Candidates = %v, want empty", m.Candidates)
	}
}

func TestResolveTagEmptyRegistry(t *testing.T) {
	m := ResolveTag("QORB", nil)
	if m.Status != TagStatusNoMatch {
		t.Fatalf("ResolveTag empty registry = %+v, want Status=no_match", m)
	}
}

func TestResolveTagsBatch(t *testing.T) {
	registered := []string{"OORB", "ZKEI"}
	matches := ResolveTags([]string{"OORB", "QORB", "AAAA"}, registered)
	if len(matches) != 3 {
		t.Fatalf("len(matches) = %d, want 3", len(matches))
	}
	if matches[0].Status != TagStatusExact {
		t.Errorf("matches[0].Status = %v, want exact", matches[0].Status)
	}
	if matches[1].Status != TagStatusCorrected || matches[1].Resolved != "OORB" {
		t.Errorf("matches[1] = %+v, want corrected -> OORB", matches[1])
	}
	if matches[2].Status != TagStatusNoMatch {
		t.Errorf("matches[2].Status = %v, want no_match", matches[2].Status)
	}
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

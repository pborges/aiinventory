package tagsheet

import (
	"math/rand"
	"regexp"
	"strings"
	"testing"
)

func TestGenerateCodesShapeAndUniqueness(t *testing.T) {
	rnd := rand.New(rand.NewSource(1))
	codes, err := GenerateCodes(rnd, 50, 4, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != 50 {
		t.Fatalf("got %d codes, want 50", len(codes))
	}

	pattern := regexp.MustCompile(`^[A-Z]{4}$`)
	seen := make(map[string]bool, len(codes))
	for _, c := range codes {
		if !pattern.MatchString(c) {
			t.Errorf("code %q does not match %s", c, pattern)
		}
		if seen[c] {
			t.Errorf("duplicate code %q", c)
		}
		seen[c] = true
	}
}

func TestGenerateCodesLocationPrefix(t *testing.T) {
	rnd := rand.New(rand.NewSource(2))
	codes, err := GenerateCodes(rnd, 10, 3, "@", nil)
	if err != nil {
		t.Fatal(err)
	}
	pattern := regexp.MustCompile(`^@[A-Z]{3}$`)
	for _, c := range codes {
		if !pattern.MatchString(c) {
			t.Errorf("code %q does not match %s", c, pattern)
		}
	}
}

func TestGenerateCodesRespectsExclusion(t *testing.T) {
	rnd := rand.New(rand.NewSource(3))
	exclude := map[string]struct{}{"AAAA": {}, "BBBB": {}, "CCCC": {}}
	codes, err := GenerateCodes(rnd, 20, 4, "", exclude)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range codes {
		if _, excluded := exclude[c]; excluded {
			t.Errorf("code %q should have been excluded", c)
		}
	}
}

func TestGenerateCodesExhaustion(t *testing.T) {
	rnd := rand.New(rand.NewSource(4))
	// Only 26 possible 1-letter codes exist, so 27 unique ones can never
	// be produced — this must terminate with a clear error rather than
	// looping forever.
	_, err := GenerateCodes(rnd, 27, 1, "", nil)
	if err == nil {
		t.Fatal("expected an exhaustion error, got nil")
	}
	if !strings.Contains(err.Error(), "exhausted") {
		t.Errorf("error = %q, want it to mention exhaustion", err.Error())
	}
}

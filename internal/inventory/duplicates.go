package inventory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pborges/aiinventory/internal/gemini"
	"github.com/pborges/aiinventory/internal/store"
)

// Runner tracks whether a duplicate-finder run is currently active, purely
// in memory — deliberately not persisted (see README's Data model notes:
// a crash mid-run just loses that attempt, with nothing left stuck).
type Runner struct {
	mu        sync.Mutex
	running   bool
	startedAt time.Time
	startedBy int64
}

// TryStart claims the runner for userID, returning false if a run is
// already active.
func (r *Runner) TryStart(userID int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		return false
	}
	r.running = true
	r.startedAt = time.Now()
	r.startedBy = userID
	return true
}

func (r *Runner) Status() (running bool, startedAt time.Time, startedBy int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running, r.startedAt, r.startedBy
}

func (r *Runner) finish() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.running = false
}

type DuplicateDetectionStore interface {
	ListAssetTagDescriptions(ctx context.Context) ([]store.AssetTagDescription, error)
	RecordDuplicateRun(ctx context.Context, status string, startedBy int64, startedAt time.Time, groups []store.DuplicateGroupCandidate) error
}

// RunDetection performs one duplicate-finder run (README flow #5): query
// every item's asset tag + description, ask Gemini to flag likely
// duplicate groups, and persist the result — always calling r.finish() so
// the runner is available again regardless of outcome. Intended to run in
// a background goroutine kicked off by the API handler; pass a context
// independent of the triggering HTTP request (which gets cancelled when
// that response is written).
func RunDetection(ctx context.Context, s DuplicateDetectionStore, g gemini.Client, r *Runner, userID int64, model, prompt string) error {
	defer r.finish()
	startedAt := time.Now()

	items, err := s.ListAssetTagDescriptions(ctx)
	if err != nil {
		s.RecordDuplicateRun(ctx, "failed", userID, startedAt, nil)
		return fmt.Errorf("list items: %w", err)
	}

	geminiItems := make([]gemini.AssetTagDescription, len(items))
	for i, it := range items {
		geminiItems[i] = gemini.AssetTagDescription{AssetTag: it.AssetTag, Description: it.Description}
	}

	result, err := g.DetectDuplicates(ctx, model, prompt, geminiItems)
	if err != nil {
		s.RecordDuplicateRun(ctx, "failed", userID, startedAt, nil)
		return fmt.Errorf("gemini: %w", err)
	}

	candidates := make([]store.DuplicateGroupCandidate, len(result.Groups))
	for i, g := range result.Groups {
		candidates[i] = store.DuplicateGroupCandidate{AssetTags: g.AssetTags, Reasoning: g.Reasoning}
	}

	if err := s.RecordDuplicateRun(ctx, "completed", userID, startedAt, candidates); err != nil {
		return fmt.Errorf("record duplicate run: %w", err)
	}
	return nil
}

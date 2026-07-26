package inventory

import (
	"context"
	"sync"

	"github.com/pborges/aiinventory/internal/gemini"
)

type BatchItemStatus string

const (
	BatchItemPending    BatchItemStatus = "pending"
	BatchItemGenerating BatchItemStatus = "generating"
	BatchItemDone       BatchItemStatus = "done"
	BatchItemError      BatchItemStatus = "error"
)

// BatchItem is one item's progress within a DescriptionBatch.
type BatchItem struct {
	ItemID      int64
	AssetTag    string
	Hint        string
	Status      BatchItemStatus
	Description string
	Error       string
}

// DescriptionBatchRequest is one item the caller wants a description
// generated for, with its optional per-item hint.
type DescriptionBatchRequest struct {
	ItemID int64
	Hint   string
}

// DescriptionBatch tracks a bulk "Regenerate description" run entirely in
// memory — same rationale as duplicate finder's Runner (README's Data model
// notes): a crash mid-run just loses that attempt, nothing gets stuck. This
// is what makes the batch survive a page refresh: the frontend polls
// Snapshot() for progress instead of holding the result in a single HTTP
// response that dies if the client goes away.
type DescriptionBatch struct {
	mu      sync.Mutex
	running bool
	items   []BatchItem
}

// TryStart claims the batch and seeds it with one pending entry per
// request, returning false if a batch is already running.
func (b *DescriptionBatch) TryStart(requests []DescriptionBatchRequest, assetTags map[int64]string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.running {
		return false
	}
	b.running = true
	b.items = make([]BatchItem, len(requests))
	for i, req := range requests {
		b.items[i] = BatchItem{ItemID: req.ItemID, AssetTag: assetTags[req.ItemID], Hint: req.Hint, Status: BatchItemPending}
	}
	return true
}

// Snapshot returns whether a batch is active plus a copy of its current
// per-item progress, safe to serialize directly for the status-poll endpoint.
func (b *DescriptionBatch) Snapshot() (running bool, items []BatchItem) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]BatchItem, len(b.items))
	copy(out, b.items)
	return b.running, out
}

func (b *DescriptionBatch) setStatus(itemID int64, status BatchItemStatus, description, errMsg string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := range b.items {
		if b.items[i].ItemID == itemID {
			b.items[i].Status = status
			b.items[i].Description = description
			b.items[i].Error = errMsg
			return
		}
	}
}

func (b *DescriptionBatch) finish() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.running = false
}

// RunDescriptionBatch processes every item in the batch sequentially (one
// Gemini call at a time, deliberately not parallel — this is a background
// job, not a latency-sensitive request, and sequential keeps Gemini load
// predictable), updating each item's status as it goes so a concurrent
// poller sees live progress. Always calls b.finish() so the batch is
// available again regardless of how many items failed. Intended to run in
// a background goroutine on a context independent of the triggering HTTP
// request — see the duplicate finder's RunDetection for the same pattern
// and why (the request context is cancelled the moment the handler returns,
// which is exactly the bug this batch replaces: the old synchronous bulk
// endpoint died mid-loop whenever the client disconnected, e.g. on a page
// refresh, since it ran the whole thing inline).
func RunDescriptionBatch(ctx context.Context, s DescriptionStore, g gemini.Client, b *DescriptionBatch, userID int64, model, prompt string) {
	defer b.finish()

	_, items := b.Snapshot()
	for _, item := range items {
		b.setStatus(item.ItemID, BatchItemGenerating, "", "")
		desc, err := RegenerateDescription(ctx, s, g, userID, model, prompt, item.ItemID, item.Hint)
		if err != nil {
			b.setStatus(item.ItemID, BatchItemError, "", err.Error())
			continue
		}
		b.setStatus(item.ItemID, BatchItemDone, desc, "")
	}
}

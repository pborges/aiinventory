package inventory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pborges/aiinventory/internal/gemini"
	"github.com/pborges/aiinventory/internal/store"
)

type fakeDuplicateStore struct {
	items       []store.AssetTagDescription
	listErr     error
	recordCalls []recordedRun
}

type recordedRun struct {
	status string
	groups []store.DuplicateGroupCandidate
}

func (f *fakeDuplicateStore) ListAssetTagDescriptions(_ context.Context) ([]store.AssetTagDescription, error) {
	return f.items, f.listErr
}

func (f *fakeDuplicateStore) RecordDuplicateRun(_ context.Context, status string, _ int64, _ time.Time, groups []store.DuplicateGroupCandidate) error {
	f.recordCalls = append(f.recordCalls, recordedRun{status: status, groups: groups})
	return nil
}

func TestRunDetectionSuccess(t *testing.T) {
	s := &fakeDuplicateStore{items: []store.AssetTagDescription{
		{AssetTag: "ZKEI", Description: "a cordless drill, S/N 123"},
		{AssetTag: "GKEI", Description: "a cordless drill, S/N 123"},
	}}
	fake := &gemini.Fake{DuplicateDetectionResult: gemini.DuplicateDetectionResult{
		Groups: []gemini.DuplicateGroupCandidate{{AssetTags: []string{"ZKEI", "GKEI"}, Reasoning: "matching S/N"}},
	}}
	runner := &Runner{}
	if !runner.TryStart(1) {
		t.Fatal("TryStart should succeed on a fresh runner")
	}

	if err := RunDetection(context.Background(), s, fake, runner, 1, "gemini-2.5-flash", "prompt"); err != nil {
		t.Fatalf("RunDetection: %v", err)
	}

	running, _, _ := runner.Status()
	if running {
		t.Error("runner should no longer be running after RunDetection returns")
	}
	if len(s.recordCalls) != 1 || s.recordCalls[0].status != "completed" {
		t.Fatalf("recordCalls = %+v", s.recordCalls)
	}
	if len(s.recordCalls[0].groups) != 1 || s.recordCalls[0].groups[0].Reasoning != "matching S/N" {
		t.Fatalf("recorded groups = %+v", s.recordCalls[0].groups)
	}
}

func TestRunDetectionGeminiFailureRecordsFailedRunAndFreesRunner(t *testing.T) {
	s := &fakeDuplicateStore{}
	fake := &gemini.Fake{DuplicateDetectionErr: errors.New("gemini unavailable")}
	runner := &Runner{}
	runner.TryStart(1)

	if err := RunDetection(context.Background(), s, fake, runner, 1, "model", "prompt"); err == nil {
		t.Fatal("expected an error when Gemini fails")
	}

	running, _, _ := runner.Status()
	if running {
		t.Error("runner must be freed even when the run fails")
	}
	if len(s.recordCalls) != 1 || s.recordCalls[0].status != "failed" {
		t.Fatalf("recordCalls = %+v, want one failed run", s.recordCalls)
	}
}

func TestRunnerRejectsConcurrentStart(t *testing.T) {
	r := &Runner{}
	if !r.TryStart(1) {
		t.Fatal("first TryStart should succeed")
	}
	if r.TryStart(2) {
		t.Fatal("second TryStart should fail while a run is active")
	}
	r.finish()
	if !r.TryStart(3) {
		t.Fatal("TryStart should succeed again after finish")
	}
}

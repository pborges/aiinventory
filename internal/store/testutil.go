package store

import (
	"context"
	"path/filepath"
	"testing"
)

// NewTestStore opens a fresh, migrated Store backed by a temp-file database
// that's cleaned up automatically when the test ends.
func NewTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

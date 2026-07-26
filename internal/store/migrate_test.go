package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestMigrate(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	s, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	// re-opening (fresh connection, same file) must be a no-op on the
	// already-applied migration, not an error.
	s2, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer s2.Close()

	tables := []string{"users", "locations", "items", "images", "settings", "activity", "duplicate_runs", "duplicate_groups", "duplicate_group_items", "items_fts", "images_fts"}
	for _, tbl := range tables {
		var name string
		err := s.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type IN ('table','view') AND name = ?`, tbl).Scan(&name)
		if err != nil {
			t.Errorf("table/vtab %q not found: %v", tbl, err)
		}
	}

	// FTS5 triggers must actually fire: inserting an item should make it
	// findable via items_fts.
	_, err = s.db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash) VALUES (1, 'alice', 'x')`)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO items (asset_tag, description) VALUES ('ZKEI', 'a cordless drill')`)
	if err != nil {
		t.Fatalf("insert item: %v", err)
	}

	var matched string
	err = s.db.QueryRowContext(ctx, `SELECT asset_tag FROM items_fts WHERE items_fts MATCH 'drill'`).Scan(&matched)
	if err != nil {
		t.Fatalf("fts5 match query: %v", err)
	}
	if matched != "ZKEI" {
		t.Errorf("fts5 match = %q, want ZKEI", matched)
	}
}

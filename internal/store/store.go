// Package store is aiinventory's data access layer: plain database/sql over
// modernc.org/sqlite (pure Go, no cgo), no ORM.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type rowScanner interface {
	Scan(dest ...any) error
}

// Open opens (creating if necessary) the SQLite database at path, applies
// pragmas suited to a single-process embedded app, and runs any pending
// migrations.
func Open(ctx context.Context, path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?%s", path, url.Values{
		"_pragma": []string{
			"foreign_keys(1)",
			"journal_mode(WAL)",
			"busy_timeout(5000)",
		},
	}.Encode())

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// modernc.org/sqlite's driver doesn't support concurrent writers well
	// across multiple *connections* to the same DB; a single connection
	// keeps writes serialized and avoids SQLITE_BUSY under WAL for this
	// single-process app.
	db.SetMaxOpenConns(1)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// withTx runs fn inside a transaction, committing on success and rolling
// back on any returned error (including a panic, which is re-raised after rollback).
func (s *Store) withTx(ctx context.Context, fn func(tx *sql.Tx) error) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

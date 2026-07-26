package store

import (
	"context"
	"embed"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrate applies any pending migrations via goose, using our own already-open
// *sql.DB (modernc.org/sqlite — pure Go, no cgo). goose only needs a dialect
// string to generate its version-tracking SQL correctly; it doesn't care what
// driver actually opened the connection.
func (s *Store) migrate(ctx context.Context) error {
	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("migrations fs: %w", err)
	}

	provider, err := goose.NewProvider(goose.DialectSQLite3, s.db, sub)
	if err != nil {
		return fmt.Errorf("goose provider: %w", err)
	}

	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}

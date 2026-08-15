package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/pborges/aiinventory/internal/domain"
)

// RegisterLocationTag idempotently adds tag to the location-tag registry —
// the self-healing path called whenever a location tag is actually
// accepted/written (an exact match, an auto-correction, or a manual
// operator override), regardless of source. A no-op if already registered.
func (s *Store) RegisterLocationTag(ctx context.Context, tag string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO registered_location_tags (location_tag) VALUES (?)
		ON CONFLICT(location_tag) DO NOTHING`, tag)
	if err != nil {
		return fmt.Errorf("register location tag %q: %w", tag, err)
	}
	return nil
}

// registerLocationTagTx is RegisterLocationTag's tx-scoped twin, for callers
// (like ApplyReconciliation) that need to fold registration into an
// existing transaction instead of opening a new one.
func registerLocationTagTx(ctx context.Context, tx *sql.Tx, tag string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO registered_location_tags (location_tag) VALUES (?)
		ON CONFLICT(location_tag) DO NOTHING`, tag)
	if err != nil {
		return fmt.Errorf("register location tag %q: %w", tag, err)
	}
	return nil
}

// BulkRegisterLocationTags idempotently adds every tag in tags to the
// registry in one transaction. Purely additive: already-registered tags
// (whether from a previous call or a duplicate within this same call) are
// left untouched and counted as skipped, never removed.
func (s *Store) BulkRegisterLocationTags(ctx context.Context, tags []string) (added, skipped int, err error) {
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO registered_location_tags (location_tag) VALUES (?)
			ON CONFLICT(location_tag) DO NOTHING`)
		if err != nil {
			return fmt.Errorf("prepare bulk register: %w", err)
		}
		defer stmt.Close()

		for _, tag := range tags {
			res, err := stmt.ExecContext(ctx, tag)
			if err != nil {
				return fmt.Errorf("register location tag %q: %w", tag, err)
			}
			n, err := res.RowsAffected()
			if err != nil {
				return err
			}
			if n > 0 {
				added++
			} else {
				skipped++
			}
		}
		return nil
	})
	return added, skipped, err
}

// ListRegisteredLocationTags returns every registered location tag, sorted
// — the whole-registry read that location-tag resolution
// (internal/inventory.ResolveTags) compares OCR reads against.
func (s *Store) ListRegisteredLocationTags(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT location_tag FROM registered_location_tags ORDER BY location_tag`)
	if err != nil {
		return nil, fmt.Errorf("list registered location tags: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, fmt.Errorf("scan registered location tag: %w", err)
		}
		out = append(out, tag)
	}
	return out, rows.Err()
}

// CountRegisteredLocationTags backs the sanity-check count shown in the
// Settings registry section (confirms a bulk import actually landed).
func (s *Store) CountRegisteredLocationTags(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM registered_location_tags`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count registered location tags: %w", err)
	}
	return count, nil
}

// GetRegisteredLocationTagByTag looks up one registry entry by its tag text
// — used to echo back the full row (id + created_at) after a create, since
// RegisterLocationTag itself is fire-and-forget idempotent and doesn't
// return one.
func (s *Store) GetRegisteredLocationTagByTag(ctx context.Context, tag string) (domain.RegisteredLocationTag, error) {
	var row domain.RegisteredLocationTag
	var createdAt string
	err := s.db.QueryRowContext(ctx, `SELECT id, location_tag, created_at FROM registered_location_tags WHERE location_tag = ?`, tag).
		Scan(&row.ID, &row.LocationTag, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.RegisteredLocationTag{}, ErrNotFound
	}
	if err != nil {
		return domain.RegisteredLocationTag{}, fmt.Errorf("get registered location tag: %w", err)
	}
	row.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
	return row, nil
}

// ListRegisteredLocationTagRows returns every registry entry with its id,
// created_at, and whether it's currently assigned to a location — the
// Settings registry section's list view, as opposed to
// ListRegisteredLocationTags' tag-only shape that tag resolution actually
// needs.
func (s *Store) ListRegisteredLocationTagRows(ctx context.Context) ([]domain.RegisteredLocationTag, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT rlt.id, rlt.location_tag, rlt.created_at,
		       EXISTS(SELECT 1 FROM locations l WHERE l.location_tag = rlt.location_tag)
		FROM registered_location_tags rlt
		ORDER BY rlt.location_tag`)
	if err != nil {
		return nil, fmt.Errorf("list registered location tag rows: %w", err)
	}
	defer rows.Close()

	var out []domain.RegisteredLocationTag
	for rows.Next() {
		var row domain.RegisteredLocationTag
		var createdAt string
		if err := rows.Scan(&row.ID, &row.LocationTag, &createdAt, &row.Assigned); err != nil {
			return nil, fmt.Errorf("scan registered location tag row: %w", err)
		}
		row.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
		out = append(out, row)
	}
	return out, rows.Err()
}

// DeleteRegisteredLocationTag removes one entry from the location-tag
// registry — Settings' registry CRUD only supports create/bulk-create/
// list/delete, no edit.
func (s *Store) DeleteRegisteredLocationTag(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM registered_location_tags WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete registered location tag: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

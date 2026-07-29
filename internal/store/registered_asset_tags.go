package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/pborges/aiinventory/internal/domain"
)

// RegisterAssetTag idempotently adds tag to the asset-tag registry — the
// self-healing path called whenever a tag is actually accepted/written (an
// exact match, an auto-correction, or a manual operator override),
// regardless of source. A no-op if already registered.
func (s *Store) RegisterAssetTag(ctx context.Context, tag string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO registered_asset_tags (asset_tag) VALUES (?)
		ON CONFLICT(asset_tag) DO NOTHING`, tag)
	if err != nil {
		return fmt.Errorf("register asset tag %q: %w", tag, err)
	}
	return nil
}

// registerAssetTagTx is RegisterAssetTag's tx-scoped twin, for callers
// (like ApplyReconciliation) that need to fold registration into an
// existing transaction instead of opening a new one.
func registerAssetTagTx(ctx context.Context, tx *sql.Tx, tag string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO registered_asset_tags (asset_tag) VALUES (?)
		ON CONFLICT(asset_tag) DO NOTHING`, tag)
	if err != nil {
		return fmt.Errorf("register asset tag %q: %w", tag, err)
	}
	return nil
}

// BulkRegisterAssetTags idempotently adds every tag in tags to the registry
// in one transaction — the write side of the label-printer script's bulk
// import. Purely additive: already-registered tags (whether from a
// previous call or a duplicate within this same call) are left untouched
// and counted as skipped, never removed.
func (s *Store) BulkRegisterAssetTags(ctx context.Context, tags []string) (added, skipped int, err error) {
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO registered_asset_tags (asset_tag) VALUES (?)
			ON CONFLICT(asset_tag) DO NOTHING`)
		if err != nil {
			return fmt.Errorf("prepare bulk register: %w", err)
		}
		defer stmt.Close()

		for _, tag := range tags {
			res, err := stmt.ExecContext(ctx, tag)
			if err != nil {
				return fmt.Errorf("register asset tag %q: %w", tag, err)
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

// ListRegisteredAssetTags returns every registered asset tag, sorted — the
// whole-registry read that tag resolution (internal/inventory.ResolveTags)
// compares OCR reads against.
func (s *Store) ListRegisteredAssetTags(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT asset_tag FROM registered_asset_tags ORDER BY asset_tag`)
	if err != nil {
		return nil, fmt.Errorf("list registered asset tags: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, fmt.Errorf("scan registered asset tag: %w", err)
		}
		out = append(out, tag)
	}
	return out, rows.Err()
}

// CountRegisteredAssetTags backs the sanity-check count shown in the
// Settings registry section (confirms a bulk import actually landed).
func (s *Store) CountRegisteredAssetTags(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM registered_asset_tags`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count registered asset tags: %w", err)
	}
	return count, nil
}

// GetRegisteredAssetTagByTag looks up one registry entry by its tag text —
// used to echo back the full row (id + created_at) after a create, since
// RegisterAssetTag itself is fire-and-forget idempotent and doesn't return one.
func (s *Store) GetRegisteredAssetTagByTag(ctx context.Context, tag string) (domain.RegisteredAssetTag, error) {
	var row domain.RegisteredAssetTag
	var createdAt string
	err := s.db.QueryRowContext(ctx, `SELECT id, asset_tag, created_at FROM registered_asset_tags WHERE asset_tag = ?`, tag).
		Scan(&row.ID, &row.Tag, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.RegisteredAssetTag{}, ErrNotFound
	}
	if err != nil {
		return domain.RegisteredAssetTag{}, fmt.Errorf("get registered asset tag: %w", err)
	}
	row.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
	return row, nil
}

// ListRegisteredAssetTagRows returns every registry entry with its id and
// created_at — the Settings registry section's list view, as opposed to
// ListRegisteredAssetTags' tag-only shape that tag resolution actually
// needs.
func (s *Store) ListRegisteredAssetTagRows(ctx context.Context) ([]domain.RegisteredAssetTag, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, asset_tag, created_at FROM registered_asset_tags ORDER BY asset_tag`)
	if err != nil {
		return nil, fmt.Errorf("list registered asset tag rows: %w", err)
	}
	defer rows.Close()

	var out []domain.RegisteredAssetTag
	for rows.Next() {
		var row domain.RegisteredAssetTag
		var createdAt string
		if err := rows.Scan(&row.ID, &row.Tag, &createdAt); err != nil {
			return nil, fmt.Errorf("scan registered asset tag row: %w", err)
		}
		row.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
		out = append(out, row)
	}
	return out, rows.Err()
}

// DeleteRegisteredAssetTag removes one entry from the asset-tag registry —
// Settings' registry CRUD only supports create/bulk-create/list/delete, no
// edit.
func (s *Store) DeleteRegisteredAssetTag(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM registered_asset_tags WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete registered asset tag: %w", err)
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

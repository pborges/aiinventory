package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/pborges/aiinventory/internal/domain"
)

func (s *Store) GetLocationByCode(ctx context.Context, code string) (domain.Location, error) {
	return s.scanLocation(s.db.QueryRowContext(ctx, `
		SELECT id, code, created_at, created_by FROM locations WHERE code = ?`, code))
}

func (s *Store) GetLocationByID(ctx context.Context, id int64) (domain.Location, error) {
	return s.scanLocation(s.db.QueryRowContext(ctx, `
		SELECT id, code, created_at, created_by FROM locations WHERE id = ?`, id))
}

// GetOrCreateLocation returns the location for code, creating it (attributed
// to userID) if this is the first time it's been seen — mirroring how a new
// asset tag auto-creates an item.
func (s *Store) GetOrCreateLocation(ctx context.Context, code string, userID int64) (domain.Location, error) {
	loc, err := s.GetLocationByCode(ctx, code)
	if err == nil {
		return loc, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return domain.Location{}, err
	}

	res, err := s.db.ExecContext(ctx, `INSERT INTO locations (code, created_by) VALUES (?, ?)`, code, userID)
	if err != nil {
		if isUniqueConstraintErr(err) {
			// lost a create race; the row exists now, just fetch it
			return s.GetLocationByCode(ctx, code)
		}
		return domain.Location{}, fmt.Errorf("create location: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return domain.Location{}, fmt.Errorf("create location: %w", err)
	}
	return s.GetLocationByID(ctx, id)
}

func (s *Store) scanLocation(row *sql.Row) (domain.Location, error) {
	var loc domain.Location
	var createdAt string
	if err := row.Scan(&loc.ID, &loc.Code, &createdAt, &loc.CreatedBy); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Location{}, ErrNotFound
		}
		return domain.Location{}, fmt.Errorf("scan location: %w", err)
	}
	loc.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
	return loc, nil
}

// ListLocations powers the location view's sidebar (README flow #4).
func (s *Store) ListLocations(ctx context.Context) ([]domain.Location, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, code, created_at, created_by FROM locations ORDER BY code`)
	if err != nil {
		return nil, fmt.Errorf("list locations: %w", err)
	}
	defer rows.Close()

	var out []domain.Location
	for rows.Next() {
		var loc domain.Location
		var createdAt string
		if err := rows.Scan(&loc.ID, &loc.Code, &createdAt, &loc.CreatedBy); err != nil {
			return nil, fmt.Errorf("scan location: %w", err)
		}
		loc.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
		out = append(out, loc)
	}
	return out, rows.Err()
}

// ListItemsByLocation returns the items currently linked to locationID.
func (s *Store) ListItemsByLocation(ctx context.Context, locationID int64) ([]domain.Item, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, asset_tag, description, location_id, created_at, updated_at
		FROM items WHERE location_id = ? ORDER BY asset_tag`, locationID)
	if err != nil {
		return nil, fmt.Errorf("list items by location: %w", err)
	}
	defer rows.Close()

	var out []domain.Item
	for rows.Next() {
		var it domain.Item
		var description sql.NullString
		var locID sql.NullInt64
		var createdAt, updatedAt string
		if err := rows.Scan(&it.ID, &it.AssetTag, &description, &locID, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan item: %w", err)
		}
		it.Description = description.String
		if locID.Valid {
			it.LocationID = &locID.Int64
		}
		it.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
		it.UpdatedAt, _ = time.Parse(time.DateTime, updatedAt)
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *Store) SetItemLocation(ctx context.Context, itemID int64, locationID *int64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE items SET location_id = ?, updated_at = datetime('now') WHERE id = ?`, locationID, itemID)
	if err != nil {
		return fmt.Errorf("set item location: %w", err)
	}
	return nil
}

// ApplyReconciliation atomically applies an already-computed diff (see
// internal/inventory.ComputeReconciliation): updates every affected item's
// location and writes one activity entry per change plus a summary entry
// for the location itself. All-or-nothing — a partial failure rolls back
// rather than leaving some items moved and others not.
func (s *Store) ApplyReconciliation(ctx context.Context, userID int64, diff domain.ReconcileDiff) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		var locationID int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM locations WHERE code = ?`, diff.LocationCode).Scan(&locationID)
		if errors.Is(err, sql.ErrNoRows) {
			res, err := tx.ExecContext(ctx, `INSERT INTO locations (code, created_by) VALUES (?, ?)`, diff.LocationCode, userID)
			if err != nil {
				return fmt.Errorf("create location: %w", err)
			}
			locationID, err = res.LastInsertId()
			if err != nil {
				return err
			}
		} else if err != nil {
			return fmt.Errorf("look up location: %w", err)
		}

		relink := func(tag string, detail string) error {
			var itemID int64
			if err := tx.QueryRowContext(ctx, `SELECT id FROM items WHERE asset_tag = ?`, tag).Scan(&itemID); err != nil {
				return fmt.Errorf("look up item %s: %w", tag, err)
			}
			if _, err := tx.ExecContext(ctx, `UPDATE items SET location_id = ?, updated_at = datetime('now') WHERE id = ?`, locationID, itemID); err != nil {
				return fmt.Errorf("relink item %s: %w", tag, err)
			}
			_, err := tx.ExecContext(ctx, `
				INSERT INTO activity (user_id, action, item_id, location_id, detail) VALUES (?, ?, ?, ?, ?)`,
				userID, string(domain.ActivityItemMoved), itemID, locationID, detail)
			return err
		}

		for _, tag := range diff.Added {
			if err := relink(tag, fmt.Sprintf("added to %s", diff.LocationCode)); err != nil {
				return err
			}
		}
		for _, m := range diff.Moved {
			detail := fmt.Sprintf("moved to %s", diff.LocationCode)
			if m.FromLocation != "" {
				detail = fmt.Sprintf("moved to %s (was %s)", diff.LocationCode, m.FromLocation)
			}
			if err := relink(m.AssetTag, detail); err != nil {
				return err
			}
		}
		for _, tag := range diff.Removed {
			var itemID int64
			if err := tx.QueryRowContext(ctx, `SELECT id FROM items WHERE asset_tag = ?`, tag).Scan(&itemID); err != nil {
				return fmt.Errorf("look up item %s: %w", tag, err)
			}
			if _, err := tx.ExecContext(ctx, `UPDATE items SET location_id = NULL, updated_at = datetime('now') WHERE id = ?`, itemID); err != nil {
				return fmt.Errorf("unlink item %s: %w", tag, err)
			}
			detail := fmt.Sprintf("removed from %s", diff.LocationCode)
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO activity (user_id, action, item_id, location_id, detail) VALUES (?, ?, ?, ?, ?)`,
				userID, string(domain.ActivityItemRemovedFromLocation), itemID, locationID, detail); err != nil {
				return err
			}
		}

		summary := fmt.Sprintf("+%d ~%d -%d", len(diff.Added), len(diff.Moved), len(diff.Removed))
		_, err = tx.ExecContext(ctx, `
			INSERT INTO activity (user_id, action, location_id, detail) VALUES (?, ?, ?, ?)`,
			userID, string(domain.ActivityLocationReconciled), locationID, summary)
		return err
	})
}

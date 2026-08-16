package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/pborges/aiinventory/internal/domain"
)

func (s *Store) UpdateItemDescriptionWithActivity(ctx context.Context, userID, itemID int64, description string, action domain.ActivityAction) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE items SET description = ?, updated_at = datetime('now') WHERE id = ?`, description, itemID)
		if err != nil {
			return fmt.Errorf("update item description: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		return logActivityTx(ctx, tx, userID, action, &itemID, nil, "")
	})
}

func (s *Store) DeleteImageWithActivity(ctx context.Context, userID, itemID, imageID int64) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM images WHERE id = ? AND item_id = ?`, imageID, itemID)
		if err != nil {
			return fmt.Errorf("delete image: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		return logActivityTx(ctx, tx, userID, domain.ActivityImageDeleted, &itemID, nil, "")
	})
}

func (s *Store) DeleteItemWithActivity(ctx context.Context, userID, itemID int64) error {
	return s.DeleteItemsWithActivity(ctx, userID, []int64{itemID})
}

// DeleteItemsWithActivity applies a bulk delete all-or-nothing. Activity is
// inserted before each delete so ON DELETE SET NULL retains the audit row and
// its asset-tag detail after the item disappears.
func (s *Store) DeleteItemsWithActivity(ctx context.Context, userID int64, itemIDs []int64) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		for _, itemID := range itemIDs {
			var tag string
			if err := tx.QueryRowContext(ctx, `SELECT asset_tag FROM items WHERE id = ?`, itemID).Scan(&tag); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return ErrNotFound
				}
				return err
			}
			if err := logActivityTx(ctx, tx, userID, domain.ActivityItemDeleted, &itemID, nil, tag); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM items WHERE id = ?`, itemID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) MoveItemToLocationWithActivity(ctx context.Context, userID, itemID, locationID int64) (domain.Item, error) {
	var item domain.Item
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		var oldLocationID sql.NullInt64
		if err := tx.QueryRowContext(ctx, `SELECT location_id FROM items WHERE id = ?`, itemID).Scan(&oldLocationID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		var newTag string
		if err := tx.QueryRowContext(ctx, `SELECT location_tag FROM locations WHERE id = ?`, locationID).Scan(&newTag); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		detail := fmt.Sprintf("moved to %s", newTag)
		if oldLocationID.Valid {
			var oldTag string
			if err := tx.QueryRowContext(ctx, `SELECT location_tag FROM locations WHERE id = ?`, oldLocationID.Int64).Scan(&oldTag); err == nil {
				detail = fmt.Sprintf("moved to %s (was %s)", newTag, oldTag)
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE items SET location_id = ?, updated_at = datetime('now') WHERE id = ?`, locationID, itemID); err != nil {
			return err
		}
		if err := logActivityTx(ctx, tx, userID, domain.ActivityItemMoved, &itemID, &locationID, detail); err != nil {
			return err
		}
		var err error
		item, err = scanItemRow(tx.QueryRowContext(ctx, `SELECT id, asset_tag, description, location_id, created_at, updated_at FROM items WHERE id = ?`, itemID))
		return err
	})
	return item, err
}

func (s *Store) SetItemLabelsWithActivity(ctx context.Context, userID, itemID int64, labelIDs []int64) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM items WHERE id = ?`, itemID).Scan(&exists); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if err := replaceItemLabelsTx(ctx, tx, itemID, labelIDs); err != nil {
			return err
		}
		return logActivityTx(ctx, tx, userID, domain.ActivityItemLabelsUpdated, &itemID, nil, "")
	})
}

func (s *Store) SetLocationLabelsWithActivity(ctx context.Context, userID, locationID int64, labelIDs []int64) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM locations WHERE id = ?`, locationID).Scan(&exists); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if err := replaceLocationLabelsTx(ctx, tx, locationID, labelIDs); err != nil {
			return err
		}
		return logActivityTx(ctx, tx, userID, domain.ActivityLocationLabelsUpdated, nil, &locationID, "")
	})
}

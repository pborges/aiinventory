package store

// These raw mutation helpers exist only for focused store tests and fixture
// setup. Production callers must use the transactional *WithActivity APIs.

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pborges/aiinventory/internal/domain"
)

func (s *Store) LogActivity(ctx context.Context, userID int64, action domain.ActivityAction, itemID, locationID *int64, detail string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO activity (user_id, action, item_id, location_id, detail) VALUES (?, ?, ?, ?, ?)`, userID, string(action), itemID, locationID, detail)
	return err
}

func (s *Store) UpdateItemDescription(ctx context.Context, id int64, description string) error {
	return execTestMutation(s.db.ExecContext(ctx, `UPDATE items SET description = ?, updated_at = datetime('now') WHERE id = ?`, description, id))
}

func (s *Store) DeleteItem(ctx context.Context, id int64) error {
	return execTestMutation(s.db.ExecContext(ctx, `DELETE FROM items WHERE id = ?`, id))
}

func (s *Store) DeleteImage(ctx context.Context, itemID, imageID int64) error {
	return execTestMutation(s.db.ExecContext(ctx, `DELETE FROM images WHERE id = ? AND item_id = ?`, imageID, itemID))
}

func (s *Store) SetItemLocation(ctx context.Context, itemID int64, locationID *int64) error {
	return execTestMutation(s.db.ExecContext(ctx, `UPDATE items SET location_id = ?, updated_at = datetime('now') WHERE id = ?`, locationID, itemID))
}

func (s *Store) SetItemLabels(ctx context.Context, itemID int64, labelIDs []int64) error {
	return s.withTx(ctx, func(tx *sql.Tx) error { return replaceItemLabelsTx(ctx, tx, itemID, labelIDs) })
}

func (s *Store) SetLocationLabels(ctx context.Context, locationID int64, labelIDs []int64) error {
	return s.withTx(ctx, func(tx *sql.Tx) error { return replaceLocationLabelsTx(ctx, tx, locationID, labelIDs) })
}

func execTestMutation(result sql.Result, err error) error {
	if err != nil {
		return fmt.Errorf("test mutation: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return ErrNotFound
	}
	return nil
}

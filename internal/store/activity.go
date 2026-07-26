package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/pborges/aiinventory/internal/domain"
)

func (s *Store) LogActivity(ctx context.Context, userID int64, action domain.ActivityAction, itemID, locationID *int64, detail string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO activity (user_id, action, item_id, location_id, detail) VALUES (?, ?, ?, ?, ?)`,
		userID, string(action), itemID, locationID, detail)
	if err != nil {
		return fmt.Errorf("log activity: %w", err)
	}
	return nil
}

// ListActivityForItem powers the item detail view's activity log panel.
func (s *Store) ListActivityForItem(ctx context.Context, itemID int64) ([]domain.Activity, error) {
	return s.listActivity(ctx, `
		SELECT activity.id, activity.user_id, users.username, activity.action, activity.item_id, activity.location_id, activity.detail, activity.created_at
		FROM activity JOIN users ON users.id = activity.user_id
		WHERE activity.item_id = ? ORDER BY activity.created_at DESC, activity.id DESC`, itemID)
}

// ListActivityForLocation powers the location view's footer activity log.
func (s *Store) ListActivityForLocation(ctx context.Context, locationID int64) ([]domain.Activity, error) {
	return s.listActivity(ctx, `
		SELECT activity.id, activity.user_id, users.username, activity.action, activity.item_id, activity.location_id, activity.detail, activity.created_at
		FROM activity JOIN users ON users.id = activity.user_id
		WHERE activity.location_id = ? ORDER BY activity.created_at DESC, activity.id DESC`, locationID)
}

func (s *Store) listActivity(ctx context.Context, query string, arg int64) ([]domain.Activity, error) {
	rows, err := s.db.QueryContext(ctx, query, arg)
	if err != nil {
		return nil, fmt.Errorf("list activity: %w", err)
	}
	defer rows.Close()

	var out []domain.Activity
	for rows.Next() {
		var a domain.Activity
		var itemID, locationID sql.NullInt64
		var detail sql.NullString
		var createdAt string
		if err := rows.Scan(&a.ID, &a.UserID, &a.Username, &a.Action, &itemID, &locationID, &detail, &createdAt); err != nil {
			return nil, fmt.Errorf("scan activity: %w", err)
		}
		if itemID.Valid {
			a.ItemID = &itemID.Int64
		}
		if locationID.Valid {
			a.LocationID = &locationID.Int64
		}
		a.Detail = detail.String
		a.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
		out = append(out, a)
	}
	return out, rows.Err()
}

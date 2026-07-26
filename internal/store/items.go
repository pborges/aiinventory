package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/pborges/aiinventory/internal/domain"
)

func (s *Store) CreateItem(ctx context.Context, assetTag string) (domain.Item, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO items (asset_tag) VALUES (?)`, assetTag)
	if err != nil {
		if isUniqueConstraintErr(err) {
			return domain.Item{}, ErrAssetTagTaken
		}
		return domain.Item{}, fmt.Errorf("create item: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return domain.Item{}, fmt.Errorf("create item: %w", err)
	}
	return s.GetItemByID(ctx, id)
}

func (s *Store) GetItemByID(ctx context.Context, id int64) (domain.Item, error) {
	return s.scanItem(s.db.QueryRowContext(ctx, `
		SELECT id, asset_tag, description, location_id, created_at, updated_at FROM items WHERE id = ?`, id))
}

func (s *Store) GetItemByAssetTag(ctx context.Context, assetTag string) (domain.Item, error) {
	return s.scanItem(s.db.QueryRowContext(ctx, `
		SELECT id, asset_tag, description, location_id, created_at, updated_at FROM items WHERE asset_tag = ?`, assetTag))
}

func (s *Store) scanItem(row *sql.Row) (domain.Item, error) {
	var it domain.Item
	var description sql.NullString
	var locationID sql.NullInt64
	var createdAt, updatedAt string
	if err := row.Scan(&it.ID, &it.AssetTag, &description, &locationID, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Item{}, ErrNotFound
		}
		return domain.Item{}, fmt.Errorf("scan item: %w", err)
	}
	it.Description = description.String
	if locationID.Valid {
		it.LocationID = &locationID.Int64
	}
	it.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
	it.UpdatedAt, _ = time.Parse(time.DateTime, updatedAt)
	return it, nil
}

func (s *Store) UpdateItemDescription(ctx context.Context, id int64, description string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE items SET description = ?, updated_at = datetime('now') WHERE id = ?`, description, id)
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
	return nil
}

func (s *Store) DeleteItem(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM items WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete item: %w", err)
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

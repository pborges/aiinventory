package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/pborges/aiinventory/internal/domain"
)

// ApplyCapture is the atomic write side of one accepted camera capture. The
// required captureID makes the operation idempotent: replaying the same
// request returns the original item without inserting or logging again.
func (s *Store) ApplyCapture(ctx context.Context, userID int64, captureID, assetTag string, data []byte, contentType, description string, setItemDescription bool) (item domain.Item, itemWasNew bool, err error) {
	if captureID == "" {
		return domain.Item{}, false, fmt.Errorf("capture id is required")
	}
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		var existingItemID int64
		var created int
		err := tx.QueryRowContext(ctx, `SELECT item_id, capture_created_item FROM images WHERE capture_id = ?`, captureID).Scan(&existingItemID, &created)
		if err == nil {
			item, err = scanItemRow(tx.QueryRowContext(ctx, `
				SELECT id, asset_tag, description, location_id, created_at, updated_at FROM items WHERE id = ?`, existingItemID))
			itemWasNew = created != 0
			return err
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("look up capture id: %w", err)
		}

		existing, err := scanItemRow(tx.QueryRowContext(ctx, `
			SELECT id, asset_tag, description, location_id, created_at, updated_at FROM items WHERE asset_tag = ?`, assetTag))
		if errors.Is(err, ErrNotFound) {
			res, err := tx.ExecContext(ctx, `INSERT INTO items (asset_tag) VALUES (?)`, assetTag)
			if err != nil {
				return fmt.Errorf("create item: %w", err)
			}
			item.ID, err = res.LastInsertId()
			if err != nil {
				return err
			}
			item.AssetTag = assetTag
			itemWasNew = true
		} else if err != nil {
			return fmt.Errorf("look up item: %w", err)
		} else {
			item = existing
		}

		if err := registerAssetTagTx(ctx, tx, assetTag); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO images (item_id, data, content_type, description, sort_order, created_by, capture_id, capture_created_item)
			SELECT ?, ?, ?, ?, COALESCE(MAX(sort_order) + 1, 0), ?, ?, ?
			FROM images WHERE item_id = ?`, item.ID, data, contentType, description, userID, captureID, boolToInt(itemWasNew), item.ID)
		if err != nil {
			return fmt.Errorf("add image: %w", err)
		}
		if setItemDescription && description != "" {
			if _, err := tx.ExecContext(ctx, `UPDATE items SET description = ?, updated_at = datetime('now') WHERE id = ?`, description, item.ID); err != nil {
				return fmt.Errorf("update item description: %w", err)
			}
			item.Description = description
		}
		action := domain.ActivityImageAdded
		if itemWasNew {
			action = domain.ActivityItemCreated
		}
		if err := logActivityTx(ctx, tx, userID, action, &item.ID, nil, ""); err != nil {
			return err
		}

		return nil
	})
	return item, itemWasNew, err
}

// AddImage appends a new image to itemID, automatically placing it after
// all existing images (sort_order = current max + 1, or 0 if it's the
// first). The item_id/sort_order computation and insert happen in one
// statement so this is race-free even under concurrent capture requests.
func (s *Store) AddImage(ctx context.Context, itemID int64, data []byte, contentType, description string, createdBy int64) (domain.Image, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO images (item_id, data, content_type, description, sort_order, created_by)
		SELECT ?, ?, ?, ?, COALESCE(MAX(sort_order) + 1, 0), ?
		FROM images WHERE item_id = ?`,
		itemID, data, contentType, description, createdBy, itemID)
	if err != nil {
		return domain.Image{}, fmt.Errorf("add image: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return domain.Image{}, fmt.Errorf("add image: %w", err)
	}
	return s.GetImageByID(ctx, id)
}

func (s *Store) GetImageByID(ctx context.Context, id int64) (domain.Image, error) {
	return s.scanImage(s.db.QueryRowContext(ctx, `
		SELECT id, item_id, data, content_type, description, sort_order, created_at, created_by
		FROM images WHERE id = ?`, id))
}

func (s *Store) scanImage(row *sql.Row) (domain.Image, error) {
	return scanImageRow(row)
}

func scanImageRow(row rowScanner) (domain.Image, error) {
	var img domain.Image
	var description sql.NullString
	var createdAt string
	if err := row.Scan(&img.ID, &img.ItemID, &img.Data, &img.ContentType, &description, &img.SortOrder, &createdAt, &img.CreatedBy); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Image{}, ErrNotFound
		}
		return domain.Image{}, fmt.Errorf("scan image: %w", err)
	}
	img.Description = description.String
	img.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
	return img, nil
}

// ListImagesByItem returns an item's images (including blob bytes) ordered
// by sort_order (lowest first = primary image).
func (s *Store) ListImagesByItem(ctx context.Context, itemID int64) ([]domain.Image, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, item_id, data, content_type, description, sort_order, created_at, created_by
		FROM images WHERE item_id = ? ORDER BY sort_order`, itemID)
	if err != nil {
		return nil, fmt.Errorf("list images: %w", err)
	}
	defer rows.Close()

	var images []domain.Image
	for rows.Next() {
		img, err := scanImageRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan image: %w", err)
		}
		images = append(images, img)
	}
	return images, rows.Err()
}

// ListImageMetaByItem is like ListImagesByItem but omits the (potentially
// large) blob bytes — for callers that only need id/description/order,
// such as description regeneration or the item detail view.
func (s *Store) ListImageMetaByItem(ctx context.Context, itemID int64) ([]domain.Image, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, item_id, content_type, description, sort_order, created_at, created_by
		FROM images WHERE item_id = ? ORDER BY sort_order`, itemID)
	if err != nil {
		return nil, fmt.Errorf("list image meta: %w", err)
	}
	defer rows.Close()

	var images []domain.Image
	for rows.Next() {
		var img domain.Image
		var description sql.NullString
		var createdAt string
		if err := rows.Scan(&img.ID, &img.ItemID, &img.ContentType, &description, &img.SortOrder, &createdAt, &img.CreatedBy); err != nil {
			return nil, fmt.Errorf("scan image meta: %w", err)
		}
		img.Description = description.String
		img.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
		images = append(images, img)
	}
	return images, rows.Err()
}

// ReorderImages sets sort_order to match the given order of imageIDs (all
// of which must belong to itemID) — the item detail view's drag-to-reorder
// carousel, where the first image becomes the primary image.
func (s *Store) ReorderImages(ctx context.Context, itemID int64, imageIDs []int64) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		for i, id := range imageIDs {
			res, err := tx.ExecContext(ctx, `UPDATE images SET sort_order = ? WHERE id = ? AND item_id = ?`, i, id, itemID)
			if err != nil {
				return fmt.Errorf("reorder image %d: %w", id, err)
			}
			n, err := res.RowsAffected()
			if err != nil {
				return err
			}
			if n == 0 {
				return fmt.Errorf("image %d does not belong to item %d", id, itemID)
			}
		}
		return nil
	})
}

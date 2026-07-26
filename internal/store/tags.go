package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pborges/aiinventory/internal/domain"
)

var ErrTagNameTaken = errors.New("tag name taken")

func (s *Store) CreateTag(ctx context.Context, name, color string) (domain.Tag, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO tags (name, color) VALUES (?, ?)`, name, color)
	if err != nil {
		if isUniqueConstraintErr(err) {
			return domain.Tag{}, ErrTagNameTaken
		}
		return domain.Tag{}, fmt.Errorf("create tag: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return domain.Tag{}, fmt.Errorf("create tag: %w", err)
	}
	return s.GetTagByID(ctx, id)
}

func (s *Store) GetTagByID(ctx context.Context, id int64) (domain.Tag, error) {
	return s.scanTag(s.db.QueryRowContext(ctx, `SELECT id, name, color, created_at FROM tags WHERE id = ?`, id))
}

func (s *Store) scanTag(row *sql.Row) (domain.Tag, error) {
	var t domain.Tag
	var createdAt string
	if err := row.Scan(&t.ID, &t.Name, &t.Color, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Tag{}, ErrNotFound
		}
		return domain.Tag{}, fmt.Errorf("scan tag: %w", err)
	}
	t.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
	return t, nil
}

// ListTags returns every tag, ordered by name — powers Settings' tag
// management list and the tag-cloud pickers in Search/ItemDetail.
func (s *Store) ListTags(ctx context.Context) ([]domain.Tag, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, color, created_at FROM tags ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	defer rows.Close()
	return scanTags(rows)
}

func scanTags(rows *sql.Rows) ([]domain.Tag, error) {
	var out []domain.Tag
	for rows.Next() {
		var t domain.Tag
		var createdAt string
		if err := rows.Scan(&t.ID, &t.Name, &t.Color, &createdAt); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		t.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) UpdateTag(ctx context.Context, id int64, name, color string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE tags SET name = ?, color = ? WHERE id = ?`, name, color, id)
	if err != nil {
		if isUniqueConstraintErr(err) {
			return ErrTagNameTaken
		}
		return fmt.Errorf("update tag: %w", err)
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

// DeleteTag removes a tag entirely; ON DELETE CASCADE on item_tags detaches
// it from every item it was applied to.
func (s *Store) DeleteTag(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM tags WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete tag: %w", err)
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

// SetItemTags replaces itemID's full set of tags with tagIDs (delete-then-
// insert in one transaction), mirroring ReorderImages' "send the whole
// desired state" semantics (see internal/store/images.go).
func (s *Store) SetItemTags(ctx context.Context, itemID int64, tagIDs []int64) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM item_tags WHERE item_id = ?`, itemID); err != nil {
			return fmt.Errorf("clear item tags: %w", err)
		}
		for _, tagID := range tagIDs {
			if _, err := tx.ExecContext(ctx, `INSERT INTO item_tags (item_id, tag_id) VALUES (?, ?)`, itemID, tagID); err != nil {
				return fmt.Errorf("set item tag %d: %w", tagID, err)
			}
		}
		return nil
	})
}

// ListTagsByItem returns one item's tags, ordered by name.
func (s *Store) ListTagsByItem(ctx context.Context, itemID int64) ([]domain.Tag, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT tags.id, tags.name, tags.color, tags.created_at
		FROM tags
		JOIN item_tags ON item_tags.tag_id = tags.id
		WHERE item_tags.item_id = ?
		ORDER BY tags.name`, itemID)
	if err != nil {
		return nil, fmt.Errorf("list tags by item: %w", err)
	}
	defer rows.Close()
	return scanTags(rows)
}

// ListTagsForItems batch-loads tags for every item in itemIDs in one query —
// used by search results so rendering a page of cards doesn't run one query
// per item.
func (s *Store) ListTagsForItems(ctx context.Context, itemIDs []int64) (map[int64][]domain.Tag, error) {
	out := make(map[int64][]domain.Tag, len(itemIDs))
	if len(itemIDs) == 0 {
		return out, nil
	}

	placeholders := make([]string, len(itemIDs))
	args := make([]any, len(itemIDs))
	for i, id := range itemIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT item_tags.item_id, tags.id, tags.name, tags.color, tags.created_at
		FROM item_tags
		JOIN tags ON tags.id = item_tags.tag_id
		WHERE item_tags.item_id IN (`+strings.Join(placeholders, ",")+`)
		ORDER BY tags.name`, args...)
	if err != nil {
		return nil, fmt.Errorf("list tags for items: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var itemID int64
		var t domain.Tag
		var createdAt string
		if err := rows.Scan(&itemID, &t.ID, &t.Name, &t.Color, &createdAt); err != nil {
			return nil, fmt.Errorf("scan item tag: %w", err)
		}
		t.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
		out[itemID] = append(out[itemID], t)
	}
	return out, rows.Err()
}

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

var ErrLocationTagNameTaken = errors.New("location tag name taken")

func (s *Store) CreateLocationTag(ctx context.Context, name, color string) (domain.Tag, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO location_tags (name, color) VALUES (?, ?)`, name, color)
	if err != nil {
		if isUniqueConstraintErr(err) {
			return domain.Tag{}, ErrLocationTagNameTaken
		}
		return domain.Tag{}, fmt.Errorf("create location tag: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return domain.Tag{}, fmt.Errorf("create location tag: %w", err)
	}
	return s.GetLocationTagByID(ctx, id)
}

func (s *Store) GetLocationTagByID(ctx context.Context, id int64) (domain.Tag, error) {
	return s.scanLocationTag(s.db.QueryRowContext(ctx, `SELECT id, name, color, created_at FROM location_tags WHERE id = ?`, id))
}

func (s *Store) scanLocationTag(row *sql.Row) (domain.Tag, error) {
	var t domain.Tag
	var createdAt string
	if err := row.Scan(&t.ID, &t.Name, &t.Color, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Tag{}, ErrNotFound
		}
		return domain.Tag{}, fmt.Errorf("scan location tag: %w", err)
	}
	t.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
	return t, nil
}

// ListLocationTags returns every location tag, ordered by name — powers
// Settings' location-tag management list and the locations view's sidebar
// filter tag cloud.
func (s *Store) ListLocationTags(ctx context.Context) ([]domain.Tag, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, color, created_at FROM location_tags ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list location tags: %w", err)
	}
	defer rows.Close()
	return scanLocationTags(rows)
}

func scanLocationTags(rows *sql.Rows) ([]domain.Tag, error) {
	var out []domain.Tag
	for rows.Next() {
		var t domain.Tag
		var createdAt string
		if err := rows.Scan(&t.ID, &t.Name, &t.Color, &createdAt); err != nil {
			return nil, fmt.Errorf("scan location tag: %w", err)
		}
		t.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) UpdateLocationTag(ctx context.Context, id int64, name, color string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE location_tags SET name = ?, color = ? WHERE id = ?`, name, color, id)
	if err != nil {
		if isUniqueConstraintErr(err) {
			return ErrLocationTagNameTaken
		}
		return fmt.Errorf("update location tag: %w", err)
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

// DeleteLocationTag removes a location tag entirely; ON DELETE CASCADE on
// location_tag_links detaches it from every location it was applied to.
func (s *Store) DeleteLocationTag(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM location_tags WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete location tag: %w", err)
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

// SetLocationTags replaces locationID's full set of tags with tagIDs
// (delete-then-insert in one transaction), mirroring SetItemTags.
func (s *Store) SetLocationTags(ctx context.Context, locationID int64, tagIDs []int64) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM location_tag_links WHERE location_id = ?`, locationID); err != nil {
			return fmt.Errorf("clear location tags: %w", err)
		}
		for _, tagID := range tagIDs {
			if _, err := tx.ExecContext(ctx, `INSERT INTO location_tag_links (location_id, tag_id) VALUES (?, ?)`, locationID, tagID); err != nil {
				return fmt.Errorf("set location tag %d: %w", tagID, err)
			}
		}
		return nil
	})
}

// ListTagsByLocation returns one location's tags, ordered by name.
func (s *Store) ListTagsByLocation(ctx context.Context, locationID int64) ([]domain.Tag, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT location_tags.id, location_tags.name, location_tags.color, location_tags.created_at
		FROM location_tags
		JOIN location_tag_links ON location_tag_links.tag_id = location_tags.id
		WHERE location_tag_links.location_id = ?
		ORDER BY location_tags.name`, locationID)
	if err != nil {
		return nil, fmt.Errorf("list tags by location: %w", err)
	}
	defer rows.Close()
	return scanLocationTags(rows)
}

// ListTagsForLocations batch-loads tags for every location in locationIDs in
// one query — used by the locations sidebar so rendering the list doesn't
// run one query per location.
func (s *Store) ListTagsForLocations(ctx context.Context, locationIDs []int64) (map[int64][]domain.Tag, error) {
	out := make(map[int64][]domain.Tag, len(locationIDs))
	if len(locationIDs) == 0 {
		return out, nil
	}

	placeholders := make([]string, len(locationIDs))
	args := make([]any, len(locationIDs))
	for i, id := range locationIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT location_tag_links.location_id, location_tags.id, location_tags.name, location_tags.color, location_tags.created_at
		FROM location_tag_links
		JOIN location_tags ON location_tags.id = location_tag_links.tag_id
		WHERE location_tag_links.location_id IN (`+strings.Join(placeholders, ",")+`)
		ORDER BY location_tags.name`, args...)
	if err != nil {
		return nil, fmt.Errorf("list tags for locations: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var locationID int64
		var t domain.Tag
		var createdAt string
		if err := rows.Scan(&locationID, &t.ID, &t.Name, &t.Color, &createdAt); err != nil {
			return nil, fmt.Errorf("scan location tag: %w", err)
		}
		t.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
		out[locationID] = append(out[locationID], t)
	}
	return out, rows.Err()
}

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

var ErrLocationLabelNameTaken = errors.New("location label name taken")

func (s *Store) CreateLocationLabel(ctx context.Context, name, color string) (domain.Label, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO location_labels (name, color) VALUES (?, ?)`, name, color)
	if err != nil {
		if isUniqueConstraintErr(err) {
			return domain.Label{}, ErrLocationLabelNameTaken
		}
		return domain.Label{}, fmt.Errorf("create location label: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return domain.Label{}, fmt.Errorf("create location label: %w", err)
	}
	return s.GetLocationLabelByID(ctx, id)
}

func (s *Store) GetLocationLabelByID(ctx context.Context, id int64) (domain.Label, error) {
	return s.scanLocationLabel(s.db.QueryRowContext(ctx, `SELECT id, name, color, created_at FROM location_labels WHERE id = ?`, id))
}

func (s *Store) scanLocationLabel(row *sql.Row) (domain.Label, error) {
	var l domain.Label
	var createdAt string
	if err := row.Scan(&l.ID, &l.Name, &l.Color, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Label{}, ErrNotFound
		}
		return domain.Label{}, fmt.Errorf("scan location label: %w", err)
	}
	l.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
	return l, nil
}

// ListLocationLabels returns every location label, ordered by name — powers
// Settings' location-label management list and the locations view's sidebar
// filter label cloud.
func (s *Store) ListLocationLabels(ctx context.Context) ([]domain.Label, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, color, created_at FROM location_labels ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list location labels: %w", err)
	}
	defer rows.Close()
	return scanLocationLabels(rows)
}

func scanLocationLabels(rows *sql.Rows) ([]domain.Label, error) {
	var out []domain.Label
	for rows.Next() {
		var l domain.Label
		var createdAt string
		if err := rows.Scan(&l.ID, &l.Name, &l.Color, &createdAt); err != nil {
			return nil, fmt.Errorf("scan location label: %w", err)
		}
		l.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) UpdateLocationLabel(ctx context.Context, id int64, name, color string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE location_labels SET name = ?, color = ? WHERE id = ?`, name, color, id)
	if err != nil {
		if isUniqueConstraintErr(err) {
			return ErrLocationLabelNameTaken
		}
		return fmt.Errorf("update location label: %w", err)
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

// DeleteLocationLabel removes a location label entirely; ON DELETE CASCADE
// on location_label_links detaches it from every location it was applied to.
func (s *Store) DeleteLocationLabel(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM location_labels WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete location label: %w", err)
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

func replaceLocationLabelsTx(ctx context.Context, tx *sql.Tx, locationID int64, labelIDs []int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM location_label_links WHERE location_id = ?`, locationID); err != nil {
		return fmt.Errorf("clear location labels: %w", err)
	}
	for _, labelID := range labelIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO location_label_links (location_id, label_id) VALUES (?, ?)`, locationID, labelID); err != nil {
			return fmt.Errorf("set location label %d: %w", labelID, err)
		}
	}
	return nil
}

// ListLabelsByLocation returns one location's labels, ordered by name.
func (s *Store) ListLabelsByLocation(ctx context.Context, locationID int64) ([]domain.Label, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT location_labels.id, location_labels.name, location_labels.color, location_labels.created_at
		FROM location_labels
		JOIN location_label_links ON location_label_links.label_id = location_labels.id
		WHERE location_label_links.location_id = ?
		ORDER BY location_labels.name`, locationID)
	if err != nil {
		return nil, fmt.Errorf("list labels by location: %w", err)
	}
	defer rows.Close()
	return scanLocationLabels(rows)
}

// ListLabelsForLocations batch-loads labels for every location in
// locationIDs in one query — used by the locations sidebar so rendering the
// list doesn't run one query per location.
func (s *Store) ListLabelsForLocations(ctx context.Context, locationIDs []int64) (map[int64][]domain.Label, error) {
	out := make(map[int64][]domain.Label, len(locationIDs))
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
		SELECT location_label_links.location_id, location_labels.id, location_labels.name, location_labels.color, location_labels.created_at
		FROM location_label_links
		JOIN location_labels ON location_labels.id = location_label_links.label_id
		WHERE location_label_links.location_id IN (`+strings.Join(placeholders, ",")+`)
		ORDER BY location_labels.name`, args...)
	if err != nil {
		return nil, fmt.Errorf("list labels for locations: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var locationID int64
		var l domain.Label
		var createdAt string
		if err := rows.Scan(&locationID, &l.ID, &l.Name, &l.Color, &createdAt); err != nil {
			return nil, fmt.Errorf("scan location label: %w", err)
		}
		l.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
		out[locationID] = append(out[locationID], l)
	}
	return out, rows.Err()
}

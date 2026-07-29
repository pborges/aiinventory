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

var ErrLabelNameTaken = errors.New("label name taken")

func (s *Store) CreateLabel(ctx context.Context, name, color string) (domain.Label, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO labels (name, color) VALUES (?, ?)`, name, color)
	if err != nil {
		if isUniqueConstraintErr(err) {
			return domain.Label{}, ErrLabelNameTaken
		}
		return domain.Label{}, fmt.Errorf("create label: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return domain.Label{}, fmt.Errorf("create label: %w", err)
	}
	return s.GetLabelByID(ctx, id)
}

func (s *Store) GetLabelByID(ctx context.Context, id int64) (domain.Label, error) {
	return s.scanLabel(s.db.QueryRowContext(ctx, `SELECT id, name, color, created_at FROM labels WHERE id = ?`, id))
}

func (s *Store) scanLabel(row *sql.Row) (domain.Label, error) {
	var l domain.Label
	var createdAt string
	if err := row.Scan(&l.ID, &l.Name, &l.Color, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Label{}, ErrNotFound
		}
		return domain.Label{}, fmt.Errorf("scan label: %w", err)
	}
	l.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
	return l, nil
}

// ListLabels returns every label, ordered by name — powers Settings' label
// management list and the label-cloud pickers in Search/ItemDetail.
func (s *Store) ListLabels(ctx context.Context) ([]domain.Label, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, color, created_at FROM labels ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list labels: %w", err)
	}
	defer rows.Close()
	return scanLabels(rows)
}

func scanLabels(rows *sql.Rows) ([]domain.Label, error) {
	var out []domain.Label
	for rows.Next() {
		var l domain.Label
		var createdAt string
		if err := rows.Scan(&l.ID, &l.Name, &l.Color, &createdAt); err != nil {
			return nil, fmt.Errorf("scan label: %w", err)
		}
		l.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) UpdateLabel(ctx context.Context, id int64, name, color string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE labels SET name = ?, color = ? WHERE id = ?`, name, color, id)
	if err != nil {
		if isUniqueConstraintErr(err) {
			return ErrLabelNameTaken
		}
		return fmt.Errorf("update label: %w", err)
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

// DeleteLabel removes a label entirely; ON DELETE CASCADE on item_labels
// detaches it from every item it was applied to.
func (s *Store) DeleteLabel(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM labels WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete label: %w", err)
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

// SetItemLabels replaces itemID's full set of labels with labelIDs (delete-
// then-insert in one transaction), mirroring ReorderImages' "send the whole
// desired state" semantics (see internal/store/images.go).
func (s *Store) SetItemLabels(ctx context.Context, itemID int64, labelIDs []int64) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM item_labels WHERE item_id = ?`, itemID); err != nil {
			return fmt.Errorf("clear item labels: %w", err)
		}
		for _, labelID := range labelIDs {
			if _, err := tx.ExecContext(ctx, `INSERT INTO item_labels (item_id, label_id) VALUES (?, ?)`, itemID, labelID); err != nil {
				return fmt.Errorf("set item label %d: %w", labelID, err)
			}
		}
		return nil
	})
}

// ListLabelsByItem returns one item's labels, ordered by name.
func (s *Store) ListLabelsByItem(ctx context.Context, itemID int64) ([]domain.Label, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT labels.id, labels.name, labels.color, labels.created_at
		FROM labels
		JOIN item_labels ON item_labels.label_id = labels.id
		WHERE item_labels.item_id = ?
		ORDER BY labels.name`, itemID)
	if err != nil {
		return nil, fmt.Errorf("list labels by item: %w", err)
	}
	defer rows.Close()
	return scanLabels(rows)
}

// ListLabelsForItems batch-loads labels for every item in itemIDs in one
// query — used by search results so rendering a page of cards doesn't run
// one query per item.
func (s *Store) ListLabelsForItems(ctx context.Context, itemIDs []int64) (map[int64][]domain.Label, error) {
	out := make(map[int64][]domain.Label, len(itemIDs))
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
		SELECT item_labels.item_id, labels.id, labels.name, labels.color, labels.created_at
		FROM item_labels
		JOIN labels ON labels.id = item_labels.label_id
		WHERE item_labels.item_id IN (`+strings.Join(placeholders, ",")+`)
		ORDER BY labels.name`, args...)
	if err != nil {
		return nil, fmt.Errorf("list labels for items: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var itemID int64
		var l domain.Label
		var createdAt string
		if err := rows.Scan(&itemID, &l.ID, &l.Name, &l.Color, &createdAt); err != nil {
			return nil, fmt.Errorf("scan item label: %w", err)
		}
		l.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
		out[itemID] = append(out[itemID], l)
	}
	return out, rows.Err()
}

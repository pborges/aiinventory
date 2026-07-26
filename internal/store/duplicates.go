package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/pborges/aiinventory/internal/domain"
)

type AssetTagDescription struct {
	AssetTag    string
	Description string
}

// ListAssetTagDescriptions is the input to the duplicate finder: every
// item's asset tag and consolidated description, formatted for Gemini.
func (s *Store) ListAssetTagDescriptions(ctx context.Context) ([]AssetTagDescription, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT asset_tag, COALESCE(description, '') FROM items ORDER BY asset_tag`)
	if err != nil {
		return nil, fmt.Errorf("list asset tag descriptions: %w", err)
	}
	defer rows.Close()

	var out []AssetTagDescription
	for rows.Next() {
		var d AssetTagDescription
		if err := rows.Scan(&d.AssetTag, &d.Description); err != nil {
			return nil, fmt.Errorf("scan asset tag description: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

type DuplicateGroupCandidate struct {
	AssetTags []string
	Reasoning string
}

// RecordDuplicateRun persists a finished duplicate-finder run (status is
// 'completed' or 'failed') and any candidate groups it found, in one
// transaction. Called only once a run finishes — "is a run active" is
// tracked in-memory by inventory.Runner, never here (see README's Data
// model section for why).
func (s *Store) RecordDuplicateRun(ctx context.Context, status string, startedBy int64, startedAt time.Time, groups []DuplicateGroupCandidate) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO duplicate_runs (status, started_by, started_at) VALUES (?, ?, ?)`,
			status, startedBy, startedAt.UTC().Format(time.DateTime))
		if err != nil {
			return fmt.Errorf("insert duplicate run: %w", err)
		}
		runID, err := res.LastInsertId()
		if err != nil {
			return err
		}

		for _, g := range groups {
			gres, err := tx.ExecContext(ctx, `INSERT INTO duplicate_groups (run_id, reasoning) VALUES (?, ?)`, runID, g.Reasoning)
			if err != nil {
				return fmt.Errorf("insert duplicate group: %w", err)
			}
			groupID, err := gres.LastInsertId()
			if err != nil {
				return err
			}

			memberCount := 0
			for _, tag := range g.AssetTags {
				var itemID int64
				err := tx.QueryRowContext(ctx, `SELECT id FROM items WHERE asset_tag = ?`, tag).Scan(&itemID)
				if errors.Is(err, sql.ErrNoRows) {
					continue // Gemini referenced a tag that no longer/never existed; skip it
				}
				if err != nil {
					return fmt.Errorf("look up item %s: %w", tag, err)
				}
				if _, err := tx.ExecContext(ctx, `INSERT INTO duplicate_group_items (group_id, item_id) VALUES (?, ?)`, groupID, itemID); err != nil {
					return fmt.Errorf("insert duplicate group item: %w", err)
				}
				memberCount++
			}
			// a "duplicate" needs at least 2 real members; drop degenerate groups
			if memberCount < 2 {
				if _, err := tx.ExecContext(ctx, `DELETE FROM duplicate_groups WHERE id = ?`, groupID); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// DuplicateGroupMember identifies one item within a candidate group — both
// the item ID (needed to submit a merge) and its asset tag (for display).
type DuplicateGroupMember struct {
	ItemID   int64
	AssetTag string
}

type DuplicateGroup struct {
	ID        int64
	Reasoning string
	Items     []DuplicateGroupMember
	CreatedAt time.Time
}

// ListPendingDuplicateGroups powers the duplicate finder's report view.
func (s *Store) ListPendingDuplicateGroups(ctx context.Context) ([]DuplicateGroup, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT dg.id, dg.reasoning, dg.created_at, items.id, items.asset_tag
		FROM duplicate_groups dg
		JOIN duplicate_group_items dgi ON dgi.group_id = dg.id
		JOIN items ON items.id = dgi.item_id
		WHERE dg.status = 'pending'
		ORDER BY dg.created_at DESC, dg.id, items.asset_tag`)
	if err != nil {
		return nil, fmt.Errorf("list pending duplicate groups: %w", err)
	}
	defer rows.Close()

	byID := make(map[int64]*DuplicateGroup)
	var order []int64
	for rows.Next() {
		var id int64
		var reasoning sql.NullString
		var createdAt string
		var itemID int64
		var tag string
		if err := rows.Scan(&id, &reasoning, &createdAt, &itemID, &tag); err != nil {
			return nil, fmt.Errorf("scan duplicate group: %w", err)
		}
		g, ok := byID[id]
		if !ok {
			g = &DuplicateGroup{ID: id, Reasoning: reasoning.String}
			g.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
			byID[id] = g
			order = append(order, id)
		}
		g.Items = append(g.Items, DuplicateGroupMember{ItemID: itemID, AssetTag: tag})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]DuplicateGroup, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out, nil
}

// DismissDuplicateGroup marks a pending group as "not a duplicate" — no
// data changes besides the group's own status.
func (s *Store) DismissDuplicateGroup(ctx context.Context, groupID, userID int64) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE duplicate_groups SET status = 'dismissed', resolved_by = ?, resolved_at = datetime('now')
		WHERE id = ? AND status = 'pending'`, userID, groupID)
	if err != nil {
		return fmt.Errorf("dismiss duplicate group: %w", err)
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

var ErrNotGroupMember = errors.New("survivor is not a member of this duplicate group")
var ErrGroupNotPending = errors.New("duplicate group is not pending")

// MergeDuplicateGroup implements the duplicate finder's "Merge" resolution:
// every other member's images are reassigned to survivorItemID (appended
// after its existing images, sort_order kept contiguous), the losers are
// hard-deleted (freeing their asset tags, same as any other delete), and
// the survivor's location is set if locationID is non-nil. Deleting the
// losers cascades to duplicate_group_items (ON DELETE CASCADE), which is
// exactly what lets us detect and auto-dismiss any *other* pending group
// left with fewer than 2 members — the "duplicate-group staleness"
// question from the README's design notes — without tracking membership
// by hand.
func (s *Store) MergeDuplicateGroup(ctx context.Context, userID, groupID, survivorItemID int64, locationID *int64) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		var status string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM duplicate_groups WHERE id = ?`, groupID).Scan(&status); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if status != "pending" {
			return ErrGroupNotPending
		}

		memberIDs, err := queryInt64Column(ctx, tx, `SELECT item_id FROM duplicate_group_items WHERE group_id = ?`, groupID)
		if err != nil {
			return err
		}
		if !slices.Contains(memberIDs, survivorItemID) {
			return ErrNotGroupMember
		}

		var survivorTag string
		if err := tx.QueryRowContext(ctx, `SELECT asset_tag FROM items WHERE id = ?`, survivorItemID).Scan(&survivorTag); err != nil {
			return fmt.Errorf("look up survivor: %w", err)
		}

		var nextSortOrder int
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort_order) + 1, 0) FROM images WHERE item_id = ?`, survivorItemID).Scan(&nextSortOrder); err != nil {
			return err
		}

		var loserTags []string
		for _, loserID := range memberIDs {
			if loserID == survivorItemID {
				continue
			}

			imageIDs, err := queryInt64Column(ctx, tx, `SELECT id FROM images WHERE item_id = ? ORDER BY sort_order`, loserID)
			if err != nil {
				return err
			}
			for _, imgID := range imageIDs {
				if _, err := tx.ExecContext(ctx, `UPDATE images SET item_id = ?, sort_order = ? WHERE id = ?`, survivorItemID, nextSortOrder, imgID); err != nil {
					return fmt.Errorf("reassign image %d: %w", imgID, err)
				}
				nextSortOrder++
			}

			var loserTag string
			if err := tx.QueryRowContext(ctx, `SELECT asset_tag FROM items WHERE id = ?`, loserID).Scan(&loserTag); err != nil {
				return err
			}
			loserTags = append(loserTags, loserTag)

			if _, err := tx.ExecContext(ctx, `DELETE FROM items WHERE id = ?`, loserID); err != nil {
				return fmt.Errorf("delete loser item %d: %w", loserID, err)
			}
		}

		if locationID != nil {
			if _, err := tx.ExecContext(ctx, `UPDATE items SET location_id = ?, updated_at = datetime('now') WHERE id = ?`, *locationID, survivorItemID); err != nil {
				return fmt.Errorf("set survivor location: %w", err)
			}
		}

		detail := fmt.Sprintf("merged %v into %s", loserTags, survivorTag)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO activity (user_id, action, item_id, location_id, detail) VALUES (?, ?, ?, ?, ?)`,
			userID, string(domain.ActivityItemsMerged), survivorItemID, locationID, detail); err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE duplicate_groups SET status = 'resolved', resolved_item_id = ?, resolved_location_id = ?, resolved_by = ?, resolved_at = datetime('now')
			WHERE id = ?`, survivorItemID, locationID, userID, groupID); err != nil {
			return fmt.Errorf("resolve duplicate group: %w", err)
		}

		// auto-dismiss any other pending group now left with fewer than 2
		// members (one of its items was just merged away)
		staleIDs, err := queryInt64Column(ctx, tx, `
			SELECT dg.id FROM duplicate_groups dg
			WHERE dg.status = 'pending' AND dg.id != ?
			AND (SELECT COUNT(*) FROM duplicate_group_items WHERE group_id = dg.id) < 2`, groupID)
		if err != nil {
			return err
		}
		for _, staleID := range staleIDs {
			if _, err := tx.ExecContext(ctx, `
				UPDATE duplicate_groups SET status = 'dismissed', resolved_by = ?, resolved_at = datetime('now')
				WHERE id = ?`, userID, staleID); err != nil {
				return fmt.Errorf("auto-dismiss stale group %d: %w", staleID, err)
			}
		}

		return nil
	})
}

func queryInt64Column(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

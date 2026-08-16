package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/pborges/aiinventory/internal/domain"
)

type SearchParams struct {
	Query            string // free-text FTS query; "" means no text search, filters only
	NoDescription    bool
	NoLocation       bool
	NoPhoto          bool
	LocationID       *int64
	LabelIDs         []int64 // non-empty: item must have at least one of these labels (OR)
	LocationLabelIDs []int64 // non-empty: item's location must have at least one of these labels (OR)
}

// ItemSummary is one search result: enough to render a card (asset tag,
// description, location tag, primary image, labels) without shipping full
// image bytes — the frontend fetches those separately via GET /api/images/{id}.
type ItemSummary struct {
	ID                  int64
	AssetTag            string
	Description         string
	LocationTag         string // "" if unlinked
	LocationDescription string // "" if unlinked or the location has no description set
	PrimaryImageID      *int64
	Labels              []domain.Label
}

const itemSummaryColumns = `items.id, items.asset_tag, items.description, locations.location_tag, locations.description,
	       (SELECT images.id FROM images WHERE images.item_id = items.id ORDER BY images.sort_order LIMIT 1)`

const itemSummarySelect = `
	SELECT ` + itemSummaryColumns + `
	FROM items
	LEFT JOIN locations ON locations.id = items.location_id`

// SearchItems implements the Search view (README flow #3): a free-text
// query hits both items_fts and images_fts (see the README's "Full-text
// search" section for the rationale), unioned by item so a hit on either an
// item's consolidated description or a not-yet-consolidated per-image note
// surfaces the same item. Combinable with the no-description/no-location/
// no-photo/specific-location filters.
func (s *Store) SearchItems(ctx context.Context, params SearchParams) ([]ItemSummary, error) {
	var filterArgs []any
	filterClause := buildFilterClause(params, &filterArgs)
	ftsQuery := literalFTSQuery(params.Query)

	if ftsQuery == "" {
		rows, err := s.db.QueryContext(ctx, itemSummarySelect+" WHERE 1=1"+filterClause+" ORDER BY items.asset_tag", filterArgs...)
		if err != nil {
			return nil, fmt.Errorf("search items: %w", err)
		}
		defer rows.Close()
		results, err := scanItemSummaries(rows)
		if err != nil {
			return nil, err
		}
		return attachLabels(ctx, s, results)
	}

	// item-level hits, ranked by relevance
	itemArgs := append([]any{ftsQuery}, filterArgs...)
	itemRows, err := s.db.QueryContext(ctx, itemSummarySelect+`
		JOIN items_fts ON items_fts.rowid = items.id
		WHERE items_fts MATCH ?`+filterClause+`
		ORDER BY bm25(items_fts)`, itemArgs...)
	if err != nil {
		return nil, fmt.Errorf("search items (items_fts): %w", err)
	}
	itemHits, err := scanItemSummaries(itemRows)
	itemRows.Close()
	if err != nil {
		return nil, err
	}

	seen := make(map[int64]bool, len(itemHits))
	for _, it := range itemHits {
		seen[it.ID] = true
	}

	// image-level-only hits (per-image notes not yet folded into the item description)
	imageArgs := append([]any{ftsQuery}, filterArgs...)
	imageRows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT `+itemSummaryColumns+`
		FROM items
		LEFT JOIN locations ON locations.id = items.location_id
		JOIN images ON images.item_id = items.id
		JOIN images_fts ON images_fts.rowid = images.id
		WHERE images_fts MATCH ?`+filterClause, imageArgs...)
	if err != nil {
		return nil, fmt.Errorf("search items (images_fts): %w", err)
	}
	defer imageRows.Close()
	imageHits, err := scanItemSummaries(imageRows)
	if err != nil {
		return nil, err
	}

	results := itemHits
	for _, it := range imageHits {
		if !seen[it.ID] {
			results = append(results, it)
			seen[it.ID] = true
		}
	}
	return attachLabels(ctx, s, results)
}

// literalFTSQuery turns the UI's free text into an FTS5 expression without
// exposing FTS operators to the user. Quoting each whitespace-delimited term
// preserves useful punctuation such as XR-500 and S/N while ANDing terms in
// the same way users expect from an inventory search box.
func literalFTSQuery(input string) string {
	fields := strings.Fields(input)
	quoted := make([]string, 0, len(fields))
	for _, field := range fields {
		quoted = append(quoted, `"`+strings.ReplaceAll(field, `"`, `""`)+`"`)
	}
	return strings.Join(quoted, " AND ")
}

// attachLabels batch-loads labels for every result in one query and
// attaches them, rather than querying per-item.
func attachLabels(ctx context.Context, s *Store, results []ItemSummary) ([]ItemSummary, error) {
	if len(results) == 0 {
		return results, nil
	}
	ids := make([]int64, len(results))
	for i, r := range results {
		ids[i] = r.ID
	}
	labelsByItem, err := s.ListLabelsForItems(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range results {
		results[i].Labels = labelsByItem[results[i].ID]
	}
	return results, nil
}

func buildFilterClause(params SearchParams, args *[]any) string {
	var conds []string
	if params.NoDescription {
		conds = append(conds, "(items.description IS NULL OR items.description = '')")
	}
	if params.NoLocation {
		conds = append(conds, "items.location_id IS NULL")
	}
	if params.NoPhoto {
		conds = append(conds, "NOT EXISTS (SELECT 1 FROM images WHERE images.item_id = items.id)")
	}
	if params.LocationID != nil {
		conds = append(conds, "items.location_id = ?")
		*args = append(*args, *params.LocationID)
	}
	if len(params.LabelIDs) > 0 {
		placeholders := make([]string, len(params.LabelIDs))
		for i, labelID := range params.LabelIDs {
			placeholders[i] = "?"
			*args = append(*args, labelID)
		}
		conds = append(conds, "EXISTS (SELECT 1 FROM item_labels WHERE item_labels.item_id = items.id AND item_labels.label_id IN ("+strings.Join(placeholders, ",")+"))")
	}
	if len(params.LocationLabelIDs) > 0 {
		placeholders := make([]string, len(params.LocationLabelIDs))
		for i, labelID := range params.LocationLabelIDs {
			placeholders[i] = "?"
			*args = append(*args, labelID)
		}
		conds = append(conds, "EXISTS (SELECT 1 FROM location_label_links WHERE location_label_links.location_id = items.location_id AND location_label_links.label_id IN ("+strings.Join(placeholders, ",")+"))")
	}
	if len(conds) == 0 {
		return ""
	}
	return " AND " + strings.Join(conds, " AND ")
}

func scanItemSummaries(rows *sql.Rows) ([]ItemSummary, error) {
	var out []ItemSummary
	for rows.Next() {
		var it ItemSummary
		var description, locationTag, locationDescription sql.NullString
		var primaryImageID sql.NullInt64
		if err := rows.Scan(&it.ID, &it.AssetTag, &description, &locationTag, &locationDescription, &primaryImageID); err != nil {
			return nil, fmt.Errorf("scan item summary: %w", err)
		}
		it.Description = description.String
		it.LocationTag = locationTag.String
		it.LocationDescription = locationDescription.String
		if primaryImageID.Valid {
			it.PrimaryImageID = &primaryImageID.Int64
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

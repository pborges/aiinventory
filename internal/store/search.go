package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type SearchParams struct {
	Query         string // free-text FTS query; "" means no text search, filters only
	NoDescription bool
	NoLocation    bool
	LocationID    *int64
}

// ItemSummary is one search result: enough to render a card (asset tag,
// description, location code, primary image) without shipping full image
// bytes — the frontend fetches those separately via GET /api/images/{id}.
type ItemSummary struct {
	ID             int64
	AssetTag       string
	Description    string
	LocationCode   string // "" if unlinked
	PrimaryImageID *int64
}

const itemSummaryColumns = `items.id, items.asset_tag, items.description, locations.code,
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
// specific-location filters.
func (s *Store) SearchItems(ctx context.Context, params SearchParams) ([]ItemSummary, error) {
	var filterArgs []any
	filterClause := buildFilterClause(params, &filterArgs)

	if params.Query == "" {
		rows, err := s.db.QueryContext(ctx, itemSummarySelect+" WHERE 1=1"+filterClause+" ORDER BY items.asset_tag", filterArgs...)
		if err != nil {
			return nil, fmt.Errorf("search items: %w", err)
		}
		defer rows.Close()
		return scanItemSummaries(rows)
	}

	// item-level hits, ranked by relevance
	itemArgs := append([]any{params.Query}, filterArgs...)
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
	imageArgs := append([]any{params.Query}, filterArgs...)
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
	if params.LocationID != nil {
		conds = append(conds, "items.location_id = ?")
		*args = append(*args, *params.LocationID)
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
		var description, locationCode sql.NullString
		var primaryImageID sql.NullInt64
		if err := rows.Scan(&it.ID, &it.AssetTag, &description, &locationCode, &primaryImageID); err != nil {
			return nil, fmt.Errorf("scan item summary: %w", err)
		}
		it.Description = description.String
		it.LocationCode = locationCode.String
		if primaryImageID.Valid {
			it.PrimaryImageID = &primaryImageID.Int64
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

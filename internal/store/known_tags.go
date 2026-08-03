package store

import (
	"context"
	"fmt"
)

// ListAllKnownAssetTags returns every asset tag the system already knows
// about — both explicitly registered tags and tags that only exist because
// an item was created with them — sorted. This is the exclusion set the
// tag-sheet generator (internal/tagsheet) draws against so a freshly
// generated code can never collide with one already in use.
func (s *Store) ListAllKnownAssetTags(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT asset_tag FROM registered_asset_tags
		UNION
		SELECT asset_tag FROM items
		ORDER BY 1`)
	if err != nil {
		return nil, fmt.Errorf("list all known asset tags: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, fmt.Errorf("scan known asset tag: %w", err)
		}
		out = append(out, tag)
	}
	return out, rows.Err()
}

// ListAllKnownLocationTags is ListAllKnownAssetTags' location-tag twin:
// every registered location tag plus every location_tag that already has a
// locations row, sorted.
func (s *Store) ListAllKnownLocationTags(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT location_tag FROM registered_location_tags
		UNION
		SELECT location_tag FROM locations
		ORDER BY 1`)
	if err != nil {
		return nil, fmt.Errorf("list all known location tags: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, fmt.Errorf("scan known location tag: %w", err)
		}
		out = append(out, tag)
	}
	return out, rows.Err()
}

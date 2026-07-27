-- +goose Up
CREATE TABLE location_tags (
    id         INTEGER PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,
    color      TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE location_tag_links (
    location_id INTEGER NOT NULL REFERENCES locations(id) ON DELETE CASCADE,
    tag_id      INTEGER NOT NULL REFERENCES location_tags(id) ON DELETE CASCADE,
    PRIMARY KEY (location_id, tag_id)
);

CREATE INDEX idx_location_tag_links_tag ON location_tag_links(tag_id);

-- +goose Down
DROP INDEX IF EXISTS idx_location_tag_links_tag;
DROP TABLE IF EXISTS location_tag_links;
DROP TABLE IF EXISTS location_tags;

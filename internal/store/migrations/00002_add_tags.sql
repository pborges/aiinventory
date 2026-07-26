-- +goose Up
CREATE TABLE tags (
    id         INTEGER PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,
    color      TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE item_tags (
    item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    tag_id  INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (item_id, tag_id)
);

CREATE INDEX idx_item_tags_tag ON item_tags(tag_id);

-- +goose Down
DROP INDEX IF EXISTS idx_item_tags_tag;
DROP TABLE IF EXISTS item_tags;
DROP TABLE IF EXISTS tags;

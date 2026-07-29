-- +goose Up
CREATE TABLE registered_tags (
    id         INTEGER PRIMARY KEY,
    tag        TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Every asset tag that already exists as an item is, by definition, a real
-- printed tag — register it so upgrading doesn't demand a manual confirm on
-- any pre-existing item's next scan.
INSERT INTO registered_tags (tag)
SELECT DISTINCT asset_tag FROM items;

-- +goose Down
DROP TABLE IF EXISTS registered_tags;

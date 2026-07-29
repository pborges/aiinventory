-- +goose Up
CREATE TABLE registered_location_tags (
    id           INTEGER PRIMARY KEY,
    location_tag TEXT NOT NULL UNIQUE,
    created_at   TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Every location tag that already exists is, by definition, a real printed
-- tag — register it so upgrading doesn't demand a manual confirm on any
-- pre-existing location's next scan.
INSERT INTO registered_location_tags (location_tag)
SELECT DISTINCT location_tag FROM locations;

-- +goose Down
DROP TABLE IF EXISTS registered_location_tags;

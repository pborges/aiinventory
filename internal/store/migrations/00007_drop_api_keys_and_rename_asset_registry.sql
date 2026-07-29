-- +goose Up
DROP TABLE IF EXISTS api_keys;

ALTER TABLE registered_tags RENAME TO registered_asset_tags;
ALTER TABLE registered_asset_tags RENAME COLUMN tag TO asset_tag;

-- +goose Down
ALTER TABLE registered_asset_tags RENAME COLUMN asset_tag TO tag;
ALTER TABLE registered_asset_tags RENAME TO registered_tags;

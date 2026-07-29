-- +goose Up
ALTER TABLE locations RENAME COLUMN code TO location_tag;

-- +goose Down
ALTER TABLE locations RENAME COLUMN location_tag TO code;

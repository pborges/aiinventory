-- +goose Up
ALTER TABLE locations ADD COLUMN description TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE locations DROP COLUMN description;

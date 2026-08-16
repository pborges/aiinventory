-- +goose Up
ALTER TABLE images ADD COLUMN capture_id TEXT;
ALTER TABLE images ADD COLUMN capture_created_item INTEGER NOT NULL DEFAULT 0;
CREATE UNIQUE INDEX idx_images_capture_id ON images(capture_id) WHERE capture_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_images_capture_id;
ALTER TABLE images DROP COLUMN capture_created_item;
ALTER TABLE images DROP COLUMN capture_id;

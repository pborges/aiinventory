-- +goose Up
ALTER TABLE tags RENAME TO labels;
ALTER TABLE item_tags RENAME TO item_labels;
ALTER TABLE item_labels RENAME COLUMN tag_id TO label_id;

DROP INDEX idx_item_tags_tag;
CREATE INDEX idx_item_labels_label ON item_labels(label_id);

-- +goose Down
DROP INDEX idx_item_labels_label;
CREATE INDEX idx_item_tags_tag ON item_labels(label_id);

ALTER TABLE item_labels RENAME COLUMN label_id TO tag_id;
ALTER TABLE item_labels RENAME TO item_tags;
ALTER TABLE labels RENAME TO tags;

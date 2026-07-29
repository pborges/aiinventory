-- +goose Up
ALTER TABLE location_tags RENAME TO location_labels;
ALTER TABLE location_tag_links RENAME TO location_label_links;
ALTER TABLE location_label_links RENAME COLUMN tag_id TO label_id;

DROP INDEX idx_location_tag_links_tag;
CREATE INDEX idx_location_label_links_label ON location_label_links(label_id);

-- +goose Down
DROP INDEX idx_location_label_links_label;
CREATE INDEX idx_location_tag_links_tag ON location_label_links(label_id);

ALTER TABLE location_label_links RENAME COLUMN label_id TO tag_id;
ALTER TABLE location_label_links RENAME TO location_tag_links;
ALTER TABLE location_labels RENAME TO location_tags;

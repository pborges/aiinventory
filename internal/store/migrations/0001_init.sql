CREATE TABLE users (
    id            INTEGER PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    enabled       INTEGER NOT NULL DEFAULT 1,
    created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE locations (
    id         INTEGER PRIMARY KEY,
    code       TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    created_by INTEGER NOT NULL REFERENCES users(id)
);

CREATE TABLE items (
    id          INTEGER PRIMARY KEY,
    asset_tag   TEXT NOT NULL UNIQUE,
    description TEXT,
    location_id INTEGER REFERENCES locations(id) ON DELETE SET NULL,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE images (
    id           INTEGER PRIMARY KEY,
    item_id      INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    data         BLOB NOT NULL,
    content_type TEXT NOT NULL,
    description  TEXT,
    sort_order   INTEGER NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    created_by   INTEGER NOT NULL REFERENCES users(id)
);

-- generic key/value app settings: gemini_model, session_secret,
-- prompt.tag_capture, prompt.location_reconciliation,
-- prompt.description_regeneration, prompt.duplicate_detection
CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE activity (
    id          INTEGER PRIMARY KEY,
    user_id     INTEGER NOT NULL REFERENCES users(id),
    action      TEXT NOT NULL,
    item_id     INTEGER REFERENCES items(id) ON DELETE SET NULL,
    location_id INTEGER REFERENCES locations(id) ON DELETE SET NULL,
    detail      TEXT,
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

-- history of finished duplicate-finder runs only; "is a run active" is tracked
-- in-memory by the server process, never persisted here (see internal/inventory.Runner)
CREATE TABLE duplicate_runs (
    id           INTEGER PRIMARY KEY,
    status       TEXT NOT NULL,
    started_by   INTEGER NOT NULL REFERENCES users(id),
    started_at   TEXT NOT NULL,
    completed_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE duplicate_groups (
    id                    INTEGER PRIMARY KEY,
    run_id                INTEGER NOT NULL REFERENCES duplicate_runs(id) ON DELETE CASCADE,
    status                TEXT NOT NULL DEFAULT 'pending',
    reasoning             TEXT,
    resolved_item_id      INTEGER REFERENCES items(id) ON DELETE SET NULL,
    resolved_location_id  INTEGER REFERENCES locations(id) ON DELETE SET NULL,
    resolved_by           INTEGER REFERENCES users(id),
    resolved_at           TEXT,
    created_at            TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE duplicate_group_items (
    group_id INTEGER NOT NULL REFERENCES duplicate_groups(id) ON DELETE CASCADE,
    item_id  INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    PRIMARY KEY (group_id, item_id)
);

CREATE INDEX idx_items_location          ON items(location_id);
CREATE INDEX idx_items_no_desc           ON items(id) WHERE description IS NULL;
CREATE INDEX idx_images_item             ON images(item_id, sort_order);
CREATE INDEX idx_activity_item           ON activity(item_id);
CREATE INDEX idx_activity_location       ON activity(location_id);
CREATE INDEX idx_duplicate_groups_pending ON duplicate_groups(status) WHERE status = 'pending';
CREATE INDEX idx_duplicate_groups_run    ON duplicate_groups(run_id);

-- full-text search: external content tables kept in sync via triggers, see README's
-- "Full-text search" section for query patterns (bm25 ranking, union of both tables).
CREATE VIRTUAL TABLE items_fts USING fts5(
    asset_tag, description,
    content='items', content_rowid='id'
);

CREATE TRIGGER items_ai AFTER INSERT ON items BEGIN
    INSERT INTO items_fts(rowid, asset_tag, description) VALUES (new.id, new.asset_tag, new.description);
END;
CREATE TRIGGER items_ad AFTER DELETE ON items BEGIN
    INSERT INTO items_fts(items_fts, rowid, asset_tag, description) VALUES ('delete', old.id, old.asset_tag, old.description);
END;
CREATE TRIGGER items_au AFTER UPDATE ON items BEGIN
    INSERT INTO items_fts(items_fts, rowid, asset_tag, description) VALUES ('delete', old.id, old.asset_tag, old.description);
    INSERT INTO items_fts(rowid, asset_tag, description) VALUES (new.id, new.asset_tag, new.description);
END;

CREATE VIRTUAL TABLE images_fts USING fts5(
    description,
    content='images', content_rowid='id'
);

CREATE TRIGGER images_ai AFTER INSERT ON images BEGIN
    INSERT INTO images_fts(rowid, description) VALUES (new.id, new.description);
END;
CREATE TRIGGER images_ad AFTER DELETE ON images BEGIN
    INSERT INTO images_fts(images_fts, rowid, description) VALUES ('delete', old.id, old.description);
END;
CREATE TRIGGER images_au AFTER UPDATE ON images BEGIN
    INSERT INTO images_fts(images_fts, rowid, description) VALUES ('delete', old.id, old.description);
    INSERT INTO images_fts(rowid, description) VALUES (new.id, new.description);
END;

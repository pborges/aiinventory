# aiinventory

An AI-assisted, camera-first inventory system. Point your phone at an item, snap a photo, and the app figures out what it is, reads the asset tag off a printed label, and files it away — no manual data entry.

> **Status:** all core flows below are implemented end-to-end (capture, reconciliation, search, location view, duplicate finder, item detail, settings, auth, TLS).

## Why

Traditional inventory tools make you type everything. This one leans on a vision model (Gemini) to do the tedious parts: identifying the item, transcribing serial/part numbers, and reading printed tags straight out of the camera frame. The human's job is mostly capture-and-confirm.

## Core flows

### 1. Camera capture — tagging an item

The camera view hugs the top of the header — a square viewport sized to leave room below it for results — with the capture button pinned to the true bottom-right corner of the phone's viewport (thumb-friendly, one-handed use, regardless of how tall the camera square itself is). It's a **preview-then-commit** flow, mirroring [location reconciliation](#2-camera-capture--location-reconciliation) below: nothing is written to the database until the user explicitly accepts.

1. **Capture** — tapping the button freezes the viewfinder on the shot just taken (the live `<video>` stream stays running underneath; the frozen frame is just laid on top) and the button becomes a spinner while the frame is analyzed.
2. **Analyze (preview)** — the frame is resized/compressed client-side to a bounded max dimension *before* it goes anywhere, then uploaded to a preview endpoint that asks Gemini to look for:
   - **An asset tag** — a 4-character, uppercase-alpha-only code printed as black text on a white label (e.g. `ZKEI`).
   - **The item itself** — Gemini attempts to identify what the item is, and transcribes any visible serial numbers, part numbers, or other identifying text. Descriptions deliberately never mention quantity or how items are arranged (a bin holding a dozen identical connectors is still described as one connector, not "several loosely packed connectors") — the asset tag identifies an item *type*, not a specific physical count.
   - Analyzing never writes anything by itself.
3. **Review** — if a tag was found, a result card shows the tag, whether accepting will add a new item or a new photo to an existing one, and the short per-image description Gemini read off the photo. Two buttons replace the capture button: **Cancel** (✕, discards the photo, no server write, camera returns live) and **Accept** (✓, commits it).
4. **Accept (apply)** — accepting re-uploads the same photo to an apply endpoint along with the reviewed tag/description, which does the actual write (create-or-append) and is trusted to echo back what the client showed rather than re-calling Gemini. On success the view clears completely and returns straight to a live, ready-to-shoot camera — no lingering confirmation text to dismiss before the next shot. On failure, the frozen frame and an error message stay up until acknowledged.
5. **No tag found** — if the frame contains no asset tag (see [location reconciliation](#2-camera-capture--location-reconciliation) for what happens next), the camera shows a "nothing found" message and a single button to clear and try again.

| Frame contains | Result on Accept |
|---|---|
| An asset tag not yet in the system | A new item is created and the photo is associated with that tag |
| An asset tag that already exists | The photo is added to that item's existing image set |
| No asset tag | Falls through to the location-reconciliation check below; if that also finds nothing, capture is rejected and the user is prompted to retake |

Each image gets a **short, per-image description** (what Gemini read off that specific photo — serials, part numbers, notable text). This is *not* the item's description. It's raw material: the item's consolidated description is synthesized later from all of its image-level notes (see [Search & bulk actions](#3-search--bulk-actions)).

### 2. Camera capture — location reconciliation

A **location code** is `@` followed by 3 uppercase-alpha-only characters (e.g. `@XYZ`) and marks a storage location — a bin, shelf, or box, in a chaotic-storage model similar to Amazon's warehouses.

A frame containing a location code is treated differently: Gemini reads the location code plus every asset tag visible in the same frame, and the app computes a **reconciliation diff** against the location's current contents:

- Asset tags in the frame but not currently linked to this location → **added**
- Asset tags currently linked to this location but absent from the frame → **removed**
- Asset tags in the frame that are currently linked to a *different* location → **moved** to this location

The user is shown a git-diff-style summary and must explicitly approve or cancel it before anything is written:

```diff
Reconciling @XYZ
+ ZKEI added
~ GKEI moved (was @QRS)
- XDKW removed
```

Same preview-then-commit shape as tag capture: the diff is computed from a preview call and nothing is linked or unlinked until Approve is pressed. Approving clears the camera straight back to a live, ready-to-shoot state (same as a successful tag-capture Accept); Cancel discards the diff with no write and does the same.

Nothing about an item's *description* changes during reconciliation — only its location link.

### 3. Search & bulk actions

Search is a primary view on **both** mobile and desktop — it's the fast path to "where is X / what's in this bin" — not just a power-user screen. A free-text query box runs a [full-text search](#full-text-search) over item descriptions *and* per-image notes (so a search for a serial number can find an item even before its description has been consolidated), ranked by relevance. Filters can be combined with that query:

- Items missing a description
- Items with no location
- A specific location (also reachable by clicking a location badge from the item detail view)

On mobile, search leans toward *finding*, not maintaining: type-ahead results rendered as image-and-description cards (primary photo + consolidated description, so you can visually confirm "yes, that's the drill I'm looking for" at a glance) rather than a dense data table. Tapping a result opens the item detail view. Bulk select/maintenance actions exist but are a secondary, desktop-first concern.

On desktop, results are selectable (with select-all), and bulk actions apply to the current selection:

- **Delete** selected items
- **Regenerate description** — opens a **live-progress modal** listing every selected item (with its current thumbnail) and kicks off a description-regeneration batch on the server. The batch runs detached from the request that started it (a background goroutine, not tied to the HTTP request's lifetime) as an in-memory, mutex-guarded singleton — the same "one job at a time, tracked in the server process, not the database" pattern as the [duplicate finder](#5-duplicate-finder-desktop) — so it keeps running and reporting progress even if the modal is closed or the page is refreshed. The modal polls a status endpoint roughly once a second and updates each row's status (pending/generating/done/error) and description as results come in. Each row also has its own optional **hint** text box (e.g. "blue enclosure") and an individual **Regenerate** button to redo just that one item's description on demand, independent of the batch.

Gemini reviews all per-image descriptions attached to an item's photos and consolidates them into one concise item description, explicitly preserving any serial/part numbers found and never inventing a quantity or count.

### 4. Location view (desktop)

A specialized version of the search page, scoped to browsing and organizing *by location* rather than by item — mostly a desktop experience.

- A **left sidebar** lists all locations; selecting one filters the main area to items currently linked to it (this is the same underlying filter as "specific location" in search).
- The main area shows richer item cards than plain search results — **live image carousels and full descriptions** per item, not just a single thumbnail.
- Item cards can be **dragged onto a different location** in the sidebar to move them — a manual, desktop alternative to relocating an item via the camera reconciliation flow. A drag-move writes the same kind of `item_moved` activity entry that reconciliation does.
- When filtered to a single location, a **footer-like panel** shows that location's activity log (reconciliations, items moved in/out, etc.).

### 5. Duplicate finder (desktop)

An on-demand background process, launched from the desktop view, that scans the whole inventory for items that look like the same physical thing tagged more than once.

- **Launch:** a button kicks off a run on the server. Only one run may be active at a time — enforced in-memory (a mutex-guarded singleton in the server process, not a database row), so the UI shows a persistent "running" indicator while it's in flight and won't let a second run start until it finishes. Nothing about "is a run active" is persisted — if the server crashes or restarts mid-run, the in-flight run is simply gone and a new one can be started immediately, with no stuck state to detect or clean up.
- **Detection:** the server queries every asset tag with its consolidated description, formats them into a single prompt, and sends it to Gemini asking it to flag groups of items that appear to be duplicates of each other.
- **Persistence:** once a run *finishes*, it's recorded, and its results aren't ephemeral — each candidate group Gemini returns is written to the database as a pending duplicate group, so the report survives across sessions and can be worked through over time, not just immediately after the run. (Only completed/failed runs are ever written — there's no persisted "in progress" state, per the launch behavior above.)
- **Report / resolution:** the user works through pending groups one at a time, resolving each by either:
  - **Not a duplicate** — dismisses the group; no data changes.
  - **Merge** — the user picks which asset tag survives and which location the merged item ends up at. All images from the other item(s) in the group are reassigned to the surviving item, and the losing item(s) are hard-deleted — freeing their asset tags for reuse, same as any other delete.
- Both resolutions write **activity log** entries.

### 6. Item detail / edit view

Primarily a desktop layout, but functional on mobile. Contains:

- An image **carousel** of every photo captured for the item. Images are **drag-and-drop reorderable**; the first image in order is the item's primary image (no separate "select primary" step). Each photo can be **deleted** individually from the carousel.
- The consolidated **item description** below the carousel, with a **Generate description** button that runs the same single-item description regeneration used by the [search view's bulk action](#3-search--bulk-actions) against just this item.
- A **shadowbox/lightbox** showing the *local* (per-image) description for whichever photo is focused in the carousel
- The item's current **location**, shown prominently and clickable — clicking it jumps to the search view pre-filtered to that location
- An **activity log** panel for the item (created, images added, moved between locations, description regenerated, merged, etc.) — the same kind of log shown in the [location view](#4-location-view-desktop), scoped to this item instead
- A **Delete item** action (hard delete, same as the search view's bulk delete) — frees its asset tag for reuse

### 7. Settings (desktop)

An administrative page, split into two areas:

**Gemini configuration**
- A dropdown to pick which Gemini model the app uses for every request type (tag capture, location reconciliation, description regeneration, duplicate detection).
- Each of those request types has its own **prompt override**: a text area where a custom prompt can be typed in to replace the app's built-in one for that request. Directly under the text area, a small link opens the app's **default prompt for that type in a shadowbox** — so you can see exactly what you're overriding (or copy it as a starting point) without leaving the page. **If the text area is left empty, the built-in default prompt is used** — overrides are opt-in per request type, not required.

**User management**
- List, create, and enable/disable accounts (via the `enabled` flag — see [Auth](#auth)).
- No admin/non-admin distinction yet: any logged-in, enabled user can manage other users' accounts, matching the flat permission model used everywhere else in the app.

## Auth

Simple username/password accounts stored in the database — no external identity provider. Every mutating action (item creation, image ingestion, location reconciliation, moves, deletes, description regeneration) is tagged with the acting username for accountability. Accounts can be disabled (`enabled` flag) without deleting them, so a deactivated user's name stays intact on historical activity. There's no role/permission tiering planned yet; any enabled, logged-in user can perform any action.

## Data model (draft — refining)

Proposed SQLite schema. This is a starting point, not final — flag anything that looks wrong or missing.

```sql
CREATE TABLE users (
    id            INTEGER PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    enabled       INTEGER NOT NULL DEFAULT 1,   -- 0/1; disabled users can't log in, but stay intact for activity attribution
    created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE locations (
    id         INTEGER PRIMARY KEY,
    code       TEXT NOT NULL UNIQUE,   -- "@" + 3 uppercase-alpha chars, e.g. "@XYZ"
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    created_by INTEGER NOT NULL REFERENCES users(id)
);

CREATE TABLE items (
    id          INTEGER PRIMARY KEY,
    asset_tag   TEXT NOT NULL UNIQUE,   -- 4-char uppercase alpha, e.g. "ZKEI" — freed for reuse on hard delete
    description TEXT,                   -- consolidated description (nullable until generated)
    location_id INTEGER REFERENCES locations(id) ON DELETE SET NULL,   -- at most one location per item
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE images (
    id           INTEGER PRIMARY KEY,
    item_id      INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    data         BLOB NOT NULL,        -- optimized image bytes — same ones sent to Gemini for analysis, no separate full-res original kept
    content_type TEXT NOT NULL,        -- e.g. "image/jpeg"
    description  TEXT,                 -- per-image notes: what Gemini read off THIS photo
    sort_order   INTEGER NOT NULL,     -- drag-and-drop order within the item; lowest = primary image
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    created_by   INTEGER NOT NULL REFERENCES users(id)
);

-- generic key/value app settings: Gemini model choice, per-request-type prompt overrides,
-- and the auto-generated session secret (only used if SESSION_SECRET isn't set via env).
-- absent key or empty value for a prompt.* key means "use the built-in default".
--   well-known keys: 'gemini_model' | 'session_secret' |
--     'prompt.tag_capture' | 'prompt.location_reconciliation' |
--     'prompt.description_regeneration' | 'prompt.duplicate_detection'
CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- one row per reconciliation/move/create/delete/merge/etc, for the "tagged by user" audit trail
CREATE TABLE activity (
    id          INTEGER PRIMARY KEY,
    user_id     INTEGER NOT NULL REFERENCES users(id),
    action      TEXT NOT NULL,     -- 'item_created' | 'image_added' | 'item_moved' | 'item_removed_from_location' | 'location_reconciled' | 'item_deleted' | 'description_regenerated' | 'duplicate_group_dismissed' | 'items_merged'
    item_id     INTEGER REFERENCES items(id) ON DELETE SET NULL,
    location_id INTEGER REFERENCES locations(id) ON DELETE SET NULL,
    detail      TEXT,              -- freeform context, e.g. "moved from @QRS" / "merged GKEI, XDKW into ZKEI"
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

-- history of finished "duplicate finder" runs. Rows are written ONLY once a run finishes —
-- "is a run currently active" is tracked in-memory by the server process (a mutex-guarded
-- singleton), not here, so a crash mid-run never leaves a stuck row blocking future runs.
CREATE TABLE duplicate_runs (
    id           INTEGER PRIMARY KEY,
    status       TEXT NOT NULL,   -- 'completed' | 'failed'
    started_by   INTEGER NOT NULL REFERENCES users(id),
    started_at   TEXT NOT NULL,
    completed_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- one candidate group of possibly-duplicate items per run; persists until resolved or dismissed
CREATE TABLE duplicate_groups (
    id            INTEGER PRIMARY KEY,
    run_id        INTEGER NOT NULL REFERENCES duplicate_runs(id) ON DELETE CASCADE,
    status        TEXT NOT NULL DEFAULT 'pending',   -- 'pending' | 'resolved' | 'dismissed'
    reasoning     TEXT,                              -- Gemini's stated reasoning for flagging this group
    resolved_item_id     INTEGER REFERENCES items(id) ON DELETE SET NULL,     -- asset tag chosen to keep, once merged
    resolved_location_id INTEGER REFERENCES locations(id) ON DELETE SET NULL, -- location chosen for the merged item
    resolved_by   INTEGER REFERENCES users(id),
    resolved_at   TEXT,
    created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

-- which items Gemini flagged as belonging to a given candidate group
CREATE TABLE duplicate_group_items (
    group_id INTEGER NOT NULL REFERENCES duplicate_groups(id) ON DELETE CASCADE,
    item_id  INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    PRIMARY KEY (group_id, item_id)
);

CREATE INDEX idx_items_location      ON items(location_id);
CREATE INDEX idx_items_no_desc       ON items(id) WHERE description IS NULL;
CREATE INDEX idx_images_item         ON images(item_id, sort_order);
CREATE INDEX idx_activity_item       ON activity(item_id);
CREATE INDEX idx_activity_location   ON activity(location_id);
CREATE INDEX idx_duplicate_groups_pending ON duplicate_groups(status) WHERE status = 'pending';
```

Resolved from earlier drafts:

- **Primary image:** dropped `items.primary_image_id` in favor of `images.sort_order` — the lowest-ordered image for an item is its primary image, and the carousel is drag-and-drop reorderable. No circular FK, no separate "select primary" affordance.
- **Delete:** confirmed hard-delete (`ON DELETE CASCADE` removes an item's images with it) — deliberate, so freed asset tags can be reused. No undo/trash.
- **Locations:** confirmed one location per item at a time.
- **Duplicate-run concurrency:** moved out of the database — "is a run active" lives in an in-memory, mutex-guarded singleton in the server process, not a persisted `'running'` status. A crash mid-run just loses that attempt; nothing gets stuck. `duplicate_runs` rows are written only once a run finishes.

Open questions worth settling before implementation:

- **Does `activity` need a generic `target` (polymorphic) column instead of separate `item_id`/`location_id`?** Fine as-is for two target types; would need rethinking if a third target type shows up.
- **Duplicate-group staleness:** if an item belongs to more than one *pending* group (e.g. Gemini flags {A,B} and {B,C} in the same or different runs) and one group gets resolved/merged first, what happens to the other pending group referencing the now-deleted item? Leaning toward auto-dismissing any other pending group that references a deleted item, with an activity note — needs confirming.
- **Merge and descriptions:** when items are merged, does the surviving item's description get auto-regenerated from the combined image set (reusing the "Regenerate description" flow), or left as-is until the user triggers that separately?

### Full-text search

Search runs on SQLite's **FTS5** extension. The [pure-Go `modernc.org/sqlite` driver](https://gitlab.com/cznic/sqlite) (no cgo — see [Tech stack](#tech-stack)) compiles it in by default across every platform it ships (confirmed via its build flags: `-DSQLITE_ENABLE_FTS5`), so this needs no extra extension-loading step or build tag.

Two **external content** FTS5 tables index the two places searchable text lives — `items.description` and `images.description` — without duplicating the text on disk. Each is kept current via standard SQLite triggers on its source table:

```sql
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
-- same AFTER INSERT/UPDATE/DELETE trigger pattern as above, mirrored for `images`
```

A search query hits both and the app unions the results by `item_id`, so a hit on either an item's consolidated description or a not-yet-consolidated per-image note surfaces the same item:

```sql
SELECT items.*, bm25(items_fts) AS rank FROM items
JOIN items_fts ON items_fts.rowid = items.id
WHERE items_fts MATCH ?;

SELECT DISTINCT items.* FROM items
JOIN images ON images.item_id = items.id
JOIN images_fts ON images_fts.rowid = images.id
WHERE images_fts MATCH ?;
```

`bm25()` gives relevance ranking on the item-level query for free. Default tokenizer (`unicode61`) is fine for this content; `tokenize='porter unicode61'` is available later if stemming (e.g. "drill" matching "drills") turns out to matter.

## Tech stack

- **Backend:** Go, serving a JSON API and the built frontend as static assets
- **Frontend:** Preact + TypeScript, built with `pnpm`
- **Database:** embedded SQLite via [`modernc.org/sqlite`](https://gitlab.com/cznic/sqlite), a pure-Go driver — deliberately **no cgo**, so builds/cross-compiles stay simple (no C toolchain needed) and there's no external DB server to run. [Full-text search](#full-text-search) uses SQLite's FTS5 extension, which this driver compiles in by default.
- **Images:** resized/compressed client-side to a bounded max dimension at capture time, before upload — the same optimized bytes are sent to Gemini for analysis and stored as blobs directly in SQLite (no separate object storage, no full-resolution originals). This keeps phone uploads small and keeps Gemini's per-request image tokens (and therefore cost/latency) down, while staying sharp enough for it to read a 4-character tag reliably.
- **AI:** Google Gemini (vision) for tag/description/OCR extraction from captured frames — model and per-request-type prompts are configurable at runtime via [Settings](#7-settings-desktop)
- **TLS:** optional self-signed certificate, generated and served at startup via an env var — convenient for camera access over HTTPS on a local network (iOS requires a secure context for camera capture on non-localhost origins)

## UI: mobile vs. desktop

**Header/nav:** a single shared header on every view — a 📦 brand icon (links home, to search) followed by finger-friendly icon buttons for each section (📷 capture, 🔍 search, 🗺️ locations, 🧬 duplicates, ⚙️ settings, 🚪 sign out). No text hyperlinks, so the same header works as a thumb-friendly mobile toolbar and a desktop nav bar. **Search is the app's home page** (`/`) — the fastest path to "where is X," and the one flow that matters equally on both breakpoints.

- **Mobile (iPhone-optimized):** two primary, full-viewport, no-scroll surfaces — **camera capture** (ingest) and **search** (lookup, and the default landing view). Reconciliation diffs and capture feedback appear as overlays rather than pushing the camera off-screen. Mobile search favors image-and-description result cards for fast visual confirmation over dense tables or bulk maintenance.
- **Desktop:** a denser, power-user layout — search adds filters, select-all, and bulk actions (delete, regenerate description); the location view adds a location sidebar, drag-to-move item cards, and a per-location activity footer; the duplicate finder runs a server-side scan and surfaces a resolvable report; the item edit view adds drag-to-reorder images and a per-item activity log. Built with the assumption of a mouse, keyboard, and a larger viewport — this is where items get maintained, not just found.

## Project layout

```
.
├── cmd/aiinventory/       # main package, entrypoint
├── internal/              # server, store, gemini client, etc.
├── web/                   # Preact + TypeScript frontend (pnpm)
│   ├── src/
│   ├── package.json
│   └── dist/              # build output, embedded into the Go binary
└── go.mod
```

The frontend is built and embedded via `go generate`:

```go
//go:generate pnpm --dir web install
//go:generate pnpm --dir web build

//go:embed all:web/dist
var distFS embed.FS
```

Running `go generate ./...` builds the frontend and refreshes the embedded assets; `go build` then produces a single self-contained binary. Since the SQLite driver is pure Go, `CGO_ENABLED=0 go build` works too — handy for cross-compiling (e.g. building for a Raspberry Pi from a Mac) without a C cross-toolchain.

## Configuration

Configured entirely via environment variables:

| Variable | Description | Default |
|---|---|---|
| `PORT` | HTTP(S) listen port | `8080` |
| `DB_PATH` | Path to the SQLite database file | `./aiinventory.db` |
| `GEMINI_API_KEY` | API key for Gemini vision requests | — (required) |
| `TLS_ENABLED` | Serve HTTPS with a self-signed certificate | `false` |
| `SESSION_SECRET` | Key used to cryptographically sign session cookies, so the server can tell a login cookie is genuine rather than forged by a client. Optional — if unset, a random value is generated on first boot and persisted in the `settings` table, so restarts keep working without you managing this by hand. Only set it explicitly if you want sessions to survive a fresh/replaced database. | auto-generated |

## Getting started

Prerequisites: Go 1.26+, Node + `pnpm`.

```bash
# install frontend deps and build the embedded assets
go generate ./...

# run the server
GEMINI_API_KEY=... SESSION_SECRET=... go run ./cmd/aiinventory
```

For local HTTPS (needed for camera access from a phone on your LAN):

```bash
GEMINI_API_KEY=... SESSION_SECRET=... TLS_ENABLED=true go run ./cmd/aiinventory
```

Then open the app from your phone's browser and add it to your home screen for a full-screen, app-like experience.

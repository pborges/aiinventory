# aiinventory

An AI-assisted, camera-first inventory system. Point your phone at an item, snap a photo, and the app figures out what it is, reads the asset tag off a printed label, and files it away — no manual data entry.

> **Status:** all core flows below are implemented end-to-end (capture, reconciliation, search, location view, duplicate finder, item detail, settings, auth, TLS).

## Why

Traditional inventory tools make you type everything. This one leans on a vision model (Gemini) to do the tedious parts: identifying the item, transcribing serial/part numbers, and reading printed tags straight out of the camera frame. The human's job is mostly capture-and-confirm.

## Core flows

### 1. Camera capture — tagging an item

The camera view hugs the top of the header — a square viewport sized to leave room below it for results — with the capture button pinned to the true bottom-right corner of the phone's viewport (thumb-friendly, one-handed use, regardless of how tall the camera square itself is). It's a **preview-then-commit** flow, mirroring [location reconciliation](#2-camera-capture--location-reconciliation) below: nothing is written to the database until the user explicitly accepts.

A **mode toggle** directly under the viewfinder — **📷 Ingest item** vs **🗺️ Locate items** — picks which flow a capture runs. Earlier versions tried to guess (asset-tag flow first, falling back to location-reconciliation if nothing was found), but asset tags and location tags are both just a handful of uppercase letters on a white sticker, and letting Gemini guess which one it was looking for was unreliable in practice. The toggle is disabled mid-capture and stays on whatever you last picked across shots, since scanning a run of the same kind of label back-to-back is the common case.

1. **Capture** — tapping the button freezes the viewfinder on the shot just taken (the live `<video>` stream stays running underneath; the frozen frame is just laid on top) and the button becomes a spinner while the frame is analyzed.
2. **Analyze (preview)** — the frame is resized/compressed client-side to a bounded max dimension *before* it goes anywhere, then uploaded to a preview endpoint that asks Gemini (pinned to `temperature=0`/`topK=1`, since this is a fixed-fact read rather than open-ended generation) to look for:
   - **An asset tag** — a 4-character, uppercase-alpha-only code printed as black text on a white label (e.g. `ZKEI`).
   - **The item itself** — Gemini attempts to identify what the item is, calling out a part's generic type alongside its part number where relevant (e.g. noting `ATF22V10C` is a PLD, `W24512AK` is static RAM), and transcribes any visible serial numbers, part numbers, or other identifying text. Descriptions deliberately never mention quantity or how items are arranged (a bin holding a dozen identical connectors is still described as one connector, not "several loosely packed connectors") — the asset tag identifies an item *type*, not a specific physical count.
   - Analyzing never writes anything by itself.
3. **Validate & resolve the tag** — Gemini's JSON schema alone doesn't enforce the tag's shape, so a well-formed-but-garbled read (wrong letter count, a stray digit, lowercase) is deterministically rejected before it ever reaches the review screen. A shape-valid read is then checked against the **asset-tag registry** (every asset tag ever accepted, plus anything pre-registered in [Settings](#7-settings-desktop)): an exact match is used as-is, a read that's a single letter off from exactly one registered tag is offered as a pre-selected correction, and anything more ambiguous or with no close match makes the operator pick a candidate or type the tag by hand — nothing Gemini reads is ever silently substituted.
4. **Review** — if a tag was resolved, a result card shows the tag, whether accepting will add a new item or a new photo to an existing one, and the short per-image description Gemini read off the photo next to a **Desc** checkbox (checked by default) — this per-image note is *always* saved on Accept regardless of the checkbox; checking it additionally copies that same text onto the item's own description, overwriting whatever was there before. Two buttons replace the capture button: **Cancel** (✕, discards the photo, no server write, camera returns live) and **Accept** (✓, commits it).
5. **Accept (apply)** — accepting re-uploads the same photo to an apply endpoint along with the resolved tag, the per-image description, and whether the Desc checkbox was on. This does the actual write (create-or-append, plus the item-description copy if requested) and is trusted to echo back what the client showed rather than re-calling Gemini. On success the view clears completely and returns straight to a live, ready-to-shoot camera — no lingering confirmation text to dismiss before the next shot. On failure, the frozen frame and an error message stay up until acknowledged.
6. **No tag found** — if the frame contains no asset tag, the camera shows a "no asset tag found" message and a single button to clear and try again (switch to **Locate items** first if the frame actually has a location tag instead).

| Frame contains (Ingest item mode) | Result on Accept |
|---|---|
| An asset tag not yet in the system | A new item is created and the photo is associated with that tag |
| An asset tag that already exists | The photo is added to that item's existing image set |
| No asset tag | Capture is rejected and the user is prompted to retake, or switch to **Locate items** mode |

Every image gets a **short, per-image description** (what Gemini read off that specific photo — serials, part numbers, notable text), saved unconditionally. The item's own description is a separate field: the Desc checkbox on capture is a shortcut that copies a single photo's note directly onto it (skipping a manual step for the common one-clean-read case), but the general path is to leave several photos' worth of notes as raw material and let the item's consolidated description be synthesized from all of them later (see [Search & bulk actions](#3-search--bulk-actions)) — that path also handles cases the capture-time shortcut can't, like reconciling notes across multiple photos of the same item.

### 2. Camera capture — location reconciliation

A **location tag** is `@` followed by 3 uppercase-alpha-only characters (e.g. `@XYZ`) and marks a storage location — a bin, shelf, or box, in a chaotic-storage model similar to Amazon's warehouses. A location can also carry an optional free-text **description**, shown alongside its tag wherever one appears in the UI (e.g. `@XYZ (Shelf A)`).

Selecting **🗺️ Locate items** on the mode toggle (see [tag capture](#1-camera-capture--tagging-an-item) above) routes a capture through this flow instead. By default, rather than trusting a single read, Gemini analyzes the same frame twice — once straight, once rotated (Gemini itself picks the rotation direction off the first read) — as a **dual-read cross-check**: the location tag and every asset tag are diffed between the two reads. A toggle in [Settings](#7-settings-desktop) can turn this off, which skips the rotated second read entirely and falls back to trusting a single read (still subject to the registry check below) — halving the Gemini calls per scan at the cost of that cross-check.

Both the location tag and every asset tag also go through the same deterministic validate-and-resolve step as [tag capture](#1-camera-capture--tagging-an-item) — location tags resolve against their own registry (Settings → [Location Tags](#7-settings-desktop)), independent from the asset-tag one. Anything both reads agree on *and* that resolves to an exact registry match goes straight into the diff below; anything only one read found, or that doesn't cleanly resolve, is surfaced in a **Confirm tags** step where the user individually accepts, corrects, or excludes each disputed tag before the diff is (re)computed.

Once the tag set is settled, the app computes a **reconciliation diff** against the location's current contents:

- Asset tags in the frame that don't match any existing item anywhere → **new** item created (no photo) and linked to this location
- Asset tags in the frame but not currently linked to this location → **added**
- Asset tags currently linked to this location but absent from the frame → **removed**
- Asset tags in the frame that are currently linked to a *different* location → **moved** to this location

The user is shown a git-diff-style summary and must explicitly approve or cancel it before anything is written:

```diff
Reconciling @XYZ
* WXYZ new item created
+ ZKEI added
~ GKEI moved (was @QRS)
- XDKW removed
```

A new item created this way has no photo yet — it shows up in search with an asset-tag placeholder thumbnail instead of an image, findable via the **no photo** filter (see [search filters](#3-search--bulk-actions)) until someone captures a real photo of it later via the tag-capture flow.

Same preview-then-commit shape as tag capture: the diff is computed from a preview call and nothing is linked or unlinked until Approve is pressed. Approving clears the camera straight back to a live, ready-to-shoot state (same as a successful tag-capture Accept); Cancel discards the diff with no write and does the same.

Each proposed change renders as its own card with a 🗑 button to drop just that one before approving — dropping a new/added/moved card excludes its tag from what gets sent on Approve, dropping a removed card keeps that item linked here instead of unlinking it. The rest of the diff still applies normally.

Nothing about an item's *description* changes during reconciliation — only its location link.

### 3. Search & bulk actions

Search is a primary view on **both** mobile and desktop — it's the fast path to "where is X / what's in this bin" — not just a power-user screen. A free-text query box runs a [full-text search](#full-text-search) over item descriptions *and* per-image notes (so a search for a serial number can find an item even before its description has been consolidated), ranked by relevance. Filters can be combined with that query:

- Items missing a description
- Items with no location
- Items with no photo
- A specific location (also reachable by clicking a location badge from the item detail view)

Above the results, a **label filters** card stacks a location-label cloud over an item-label cloud — clickable, color-coded chips for every user-defined [label](#7-settings-desktop). Selecting multiple labels within one cloud is OR'd together (any match); each cloud's selection is then AND'd against every other active filter, including the other cloud. Labels are separate from — and filtered independently of — the physical asset-tag/location-tag identifiers used during capture.

On mobile, search leans toward *finding*, not maintaining: type-ahead results rendered as image-and-description cards (primary photo + consolidated description, so you can visually confirm "yes, that's the drill I'm looking for" at a glance) rather than a dense data table. Tapping a result opens the item detail view. Bulk select/maintenance actions exist but are a secondary, desktop-first concern.

On desktop, results are selectable (with select-all), and bulk actions apply to the current selection:

- **Delete** selected items
- **Regenerate description** — opens a **live-progress modal** listing every selected item (with its current thumbnail) and kicks off a description-regeneration batch on the server. The batch runs detached from the request that started it (a background goroutine, not tied to the HTTP request's lifetime) as an in-memory, mutex-guarded singleton — the same "one job at a time, tracked in the server process, not the database" pattern as the [duplicate finder](#5-duplicate-finder-desktop) — so it keeps running and reporting progress even if the modal is closed or the page is refreshed. The modal polls a status endpoint roughly once a second and updates each row's status (pending/generating/done/error) and description as results come in. Each row also has its own optional **hint** text box (e.g. "blue enclosure") and an individual **Regenerate** button to redo just that one item's description on demand, independent of the batch.

Gemini reviews all per-image descriptions attached to an item's photos and consolidates them into one concise item description, explicitly preserving any serial/part numbers found and never inventing a quantity or count.

### 4. Location view (desktop)

A specialized version of the search page, scoped to browsing and organizing *by location* rather than by item — mostly a desktop experience.

- A **left sidebar** lists all locations, with a **location-label filter cloud** above it to narrow the list by assigned labels; selecting a location filters the main area to items currently linked to it (this is the same underlying filter as "specific location" in search).
- Selecting a location also surfaces its optional **description** and its **labels** inline for editing — the same label pool used to filter here and in [search](#3-search--bulk-actions).
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
- The item's **labels** (colored, user-defined — see [Settings](#7-settings-desktop)), toggled on/off via a chip cloud — the same pool [search](#3-search--bulk-actions) filters by.
- A **shadowbox/lightbox** showing the *local* (per-image) description for whichever photo is focused in the carousel
- The item's current **location**, shown prominently and clickable — clicking it jumps to the search view pre-filtered to that location
- An **activity log** panel for the item (created, images added, moved between locations, description regenerated, merged, etc.) — the same kind of log shown in the [location view](#4-location-view-desktop), scoped to this item instead
- A **Delete item** action (hard delete, same as the search view's bulk delete) — frees its asset tag for reuse

### 7. Settings (desktop)

An administrative page with a section per sidebar item:

**Gemini configuration**
- An API key field — the Gemini API key lives in the `settings` table, not an environment variable, so it can be set/rotated/cleared from here without restarting the server. The key is set-only: once configured, the field never echoes the raw value back, only whether one is set. Saving a new key rebuilds the live Gemini client immediately; clearing it disables AI-dependent routes immediately.
- A dropdown to pick which Gemini model the app uses for every request type (tag capture, location reconciliation, description regeneration, duplicate detection).
- Each of those request types has its own **prompt override**: a text area where a custom prompt can be typed in to replace the app's built-in one for that request. Directly under the text area, a small link opens the app's **default prompt for that type in a shadowbox** — so you can see exactly what you're overriding (or copy it as a starting point) without leaving the page. **If the text area is left empty, the built-in default prompt is used** — overrides are opt-in per request type, not required.
- A **dual-read location tag cross-check** toggle (on by default) — turns the [location reconciliation](#2-camera-capture--location-reconciliation) flow's straight+rotated dual-read cross-check on or off. Disabling it halves the Gemini calls per locate scan (a single straight read is used, still validated against the location-tag registry) at the cost of losing that extra OCR safety net.

**Users**
- List, create, and enable/disable accounts (via the `enabled` flag — see [Auth](#auth)).
- No admin/non-admin distinction yet: any logged-in, enabled user can manage other users' accounts, matching the flat permission model used everywhere else in the app.

**Asset Tags** / **Location Tags**
Two identically-shaped sections — one over asset-tag data, one over location-tag data — each with three subsections:
- **Labels** — full CRUD (name + color, add/edit/delete) over that pool's user-defined labels, the colored chips shown on items/locations and used as filters in [search](#3-search--bulk-actions) and the [location view](#4-location-view-desktop). Item labels and location labels are entirely separate pools.
- **Registered Tags** — CRUD (create, bulk-import from a one-tag-per-line `.txt` file, list, delete — no edit) over the tag registry backing the deterministic OCR-correction step used by [tag capture](#1-camera-capture--tagging-an-item) and [location reconciliation](#2-camera-capture--location-reconciliation). Every tag actually accepted through capture or reconciliation self-registers here automatically, so day-to-day scanning never needs this page — it exists for pre-registering a fresh batch of printed labels in bulk before they've ever been scanned, and for pruning tags that should no longer be recognized.
- **Generate Asset Tags** / **Generate Location Tags** — a one-click replacement for the old manual FreeCAD/LightBurn workflow of designing a sheet of printable tags by hand. Pick a grid (columns × rows) and padding between tags; a live preview redraws as those change, showing fresh, randomly generated codes that are checked against every known tag (registered *and* already in use by an item/location) so a freshly cut label can never collide with one already out in the world. **Download SVG + Register Tags** registers exactly the previewed codes in the same registry the CRUD subsection above manages, then downloads both an SVG (for a quick look, or import into other tools) and a native LightBurn `.lbrn2` project sized for a 60×26mm rounded-rect tag with OCR-B text — pre-split into the three cut layers a fiber/CO2 laser workflow needs, in order: a raster scan fill of the text, a vector cut tracing the text's outline, then a vector cut of the tag's outline. Nothing is registered until that download button is clicked — reloading or tweaking the grid beforehand is free.

## Auth

Simple username/password accounts stored in the database — no external identity provider. Every mutating action (item creation, image ingestion, location reconciliation, moves, deletes, description regeneration) is tagged with the acting username for accountability. Accounts can be disabled (`enabled` flag) without deleting them, so a deactivated user's name stays intact on historical activity. There's no role/permission tiering planned yet; any enabled, logged-in user can perform any action.

## Data model

SQLite-backed, structured around these core entities:

- **User** — a username/password account. Disabling one (the `enabled` flag) blocks login without deleting the account, so its username stays intact on historical activity.
- **Location** — a storage bin/shelf/box, identified by its location tag (`@` + 3 uppercase letters, e.g. `@XYZ`) plus an optional free-text description. Holds zero or more **location labels**.
- **Item** — a physical thing being tracked, identified by its asset tag (4 uppercase letters, e.g. `ZKEI`, freed for reuse once the item is deleted) plus a consolidated description. At most one location at a time. Holds one or more **images** and zero or more **item labels**.
- **Image** — one photo captured for an item: the optimized bytes actually sent to Gemini (no separate full-resolution original), a per-image description (what Gemini read off that specific photo), and a drag-and-drop sort order (lowest = primary image).
- **Label** — a color-coded, human-curated tag (e.g. "Fragile"), distinct from the physical asset/location tags above. Item labels and location labels are two entirely independent pools.
- **Registered tag** — one entry in the asset-tag or location-tag allow-list backing the deterministic OCR-correction step (see [tag capture](#1-camera-capture--tagging-an-item)). Self-registers whenever a tag is actually accepted through capture/reconciliation; Settings can also pre-register or bulk-import entries directly.
- **Settings** — a generic key/value store for the Gemini model choice, per-request-type prompt overrides, the [dual-read toggle](#7-settings-desktop), and the auto-generated session secret.
- **Activity** — an audit-trail entry (item created, moved, merged, labels updated, etc.) tied to the acting user and, optionally, an item and/or location.
- **Duplicate run / group** — a finished duplicate-finder scan and the candidate groups of possibly-duplicate items it flagged, persisted so a report can be worked through over time. Whether a run is *currently* active is never persisted — it lives only in server memory (see [duplicate finder](#5-duplicate-finder-desktop)).

A few notable design decisions:

- **Primary image:** no separate "primary image" reference — the lowest-sorted image for an item is its primary image, and the carousel is drag-and-drop reorderable.
- **Delete:** hard delete, cascading to an item's images — deliberate, so freed asset tags can be reused. No undo/trash.
- **Duplicate-run concurrency:** kept out of the database entirely — "is a run active" lives in an in-memory, mutex-guarded singleton in the server process. A crash mid-run just loses that attempt; nothing gets stuck, and a finished-run row is written only once the run completes.
- **Tag vs. label naming:** "tag" is reserved for the physical, OCR-read identifiers (asset tags, location tags); the color-coded, human-assigned pool is called a "label" instead, so the two concepts don't collide in conversation or code.
- **OCR trust:** raw Gemini reads are never trusted directly — a deterministic shape check plus the registered-tag allow-list catches malformed and misread tags before they can create phantom items or corrupt a reconciliation diff.

Open questions:

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

**Header/nav:** a single shared header on every view — a 📦 brand icon (links home, to search) followed by finger-friendly icon buttons for each section (📷 capture, 🔍 search, 🗺️ locations, 🧬 duplicates, ⚙️ settings, 🚪 sign out). No text hyperlinks, so the same header works as a thumb-friendly mobile toolbar and a desktop nav bar. **Search is the app's home page** (`/`) — the fastest path to "where is X," and the one flow that matters equally on both breakpoints. A matching **footer** on every view (including the sign-in screen) shows the running build's version, from `GET /api/version` — handy for confirming which image actually got deployed.

- **Mobile (iPhone-optimized):** two primary, full-viewport, no-scroll surfaces — **camera capture** (ingest) and **search** (lookup, and the default landing view). Reconciliation diffs and capture feedback appear as overlays rather than pushing the camera off-screen. Mobile search favors image-and-description result cards for fast visual confirmation over dense tables or bulk maintenance.
- **Desktop:** a denser, power-user layout — search adds filters (including label filter clouds), select-all, and bulk actions (delete, regenerate description); the location view adds a location sidebar with label filtering, drag-to-move item cards, and a per-location activity footer; the duplicate finder runs a server-side scan and surfaces a resolvable report; the item edit view adds drag-to-reorder images, label editing, and a per-item activity log. Built with the assumption of a mouse, keyboard, and a larger viewport — this is where items get maintained, not just found.

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

Configured via environment variables, except the Gemini API key — that lives in the `settings` table and is set from the [Settings](#7-settings-desktop) page, not an env var, so it can be rotated without a restart:

| Variable | Description | Default |
|---|---|---|
| `PORT` | HTTP(S) listen port | `8080` |
| `DB_PATH` | Path to the SQLite database file | `./aiinventory.db` |
| `TLS_ENABLED` | Serve HTTPS with a self-signed certificate | `false` |
| `SESSION_SECRET` | Key used to cryptographically sign session cookies, so the server can tell a login cookie is genuine rather than forged by a client. Optional — if unset, a random value is generated on first boot and persisted in the `settings` table, so restarts keep working without you managing this by hand. Only set it explicitly if you want sessions to survive a fresh/replaced database. | auto-generated |

There's also one CLI flag, for debugging OCR misreads rather than everyday deployment:

```bash
aiinventory -store ./scans
```

When set, every capture/reconcile preview writes the resized image actually sent to Gemini (`<prefix>-<id>.jpg`) plus a `<prefix>-<id>.txt` sidecar listing whatever tags/location tag Gemini reported — unfiltered by the deterministic validation described in [tag capture](#1-camera-capture--tagging-an-item) — into that directory. Off by default; omitting `-store` (or passing an empty value) disables it.

## Getting started

Prerequisites: Go 1.26+, Node + `pnpm`.

```bash
# install frontend deps and build the embedded assets
go generate ./...

# run the server
SESSION_SECRET=... go run ./cmd/aiinventory
```

For local HTTPS (needed for camera access from a phone on your LAN):

```bash
SESSION_SECRET=... TLS_ENABLED=true go run ./cmd/aiinventory
```

Then open the app from your phone's browser and add it to your home screen for a full-screen, app-like experience. On first run, bootstrap an account, then open **Settings** and set a Gemini API key to enable the AI-dependent flows (capture, reconciliation, description regeneration, duplicate finder) — everything else works without one.

## Docker

Tagged releases (`vX.Y.Z`) are built and published to GHCR by [`.github/workflows/docker.yml`](.github/workflows/docker.yml) as a `linux/amd64`-only Alpine image — the app has no other platform requirements (pure-Go SQLite driver, no cgo), amd64 is just the only architecture this project actually runs on, so that's all CI builds. The pushed git tag is embedded into the binary as its version (see [`internal/version`](internal/version/version.go)) and surfaced in the webui's footer and at `GET /api/version`.

```bash
docker run -d \
  -p 8080:8080 \
  -v aiinventory-data:/data \
  ghcr.io/pborges/aiinventory:latest
```

The database lives at `/data/aiinventory.db` by default (`DB_PATH`) — mount a volume there to persist it across container restarts. Everything else is configured the same way as [Getting started](#getting-started): environment variables in, plus the Gemini key from the Settings page once it's up.

To build the image locally (e.g. to test a change before tagging a release):

```bash
docker build --build-arg VERSION=dev-local -t aiinventory:dev .
```

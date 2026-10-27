# Nurtur — Album

**Status:** planned
**Branch:** `feat/album`
**Goal:** a fourth bottom-nav page where the household can snap photos of the baby,
scroll them newest-first, and see the local date and time for every photo.

**Golden rule:** the album must stay as calm and one-handed as the rest of Nurtur.
Adding a photo should take two taps from the Album tab, and browsing must never
make logging feel heavier.

---

## Product rules

- **Private by default.** The R2 bucket stays private. No public bucket, no public
  object URLs, no third-party image host, no analytics. Every read goes through
  Nurtur and is scoped to the signed-in account.
- **Date and time are first-class.** A photo is grouped by the local day it was
  taken, and every tile shows its time. A viewer shows the full date and time.
- **Camera first, library second.** On a phone, "Take photo" opens the rear camera
  directly; "Choose photos" opens the library with multi-select.
- **No navigation regressions.** Album is a fourth tab, not a replacement for
  Home, History, or Settings. Quick logging keeps working from every other page.
- **Logging never depends on the album.** If R2 is down or an upload fails, feeding,
  diapers, sleep, and history continue to work normally.
- **Keep a good copy, not every copy.** By default we keep a high-quality display
  copy (long edge 2560 px) and a 480 px thumbnail. We do not keep the untouched
  camera original. That keeps storage and transfer predictable for a phone-first
  app; a `NURTUR_ALBUM_KEEP_ORIGINAL` option can be added later without changing
  the data model (store the original key as an extra variant).

---

## Architecture at a glance

```text
Phone browser
  │  compress + resize + read EXIF date
  │  multipart POST /album/photos
  ▼
Nurtur (Go)
  │  validate, upload two objects
  ├──────────────► Cloudflare R2 (private bucket)
  │                  accounts/<id>/album/<upload-id>/full.jpg
  │                  accounts/<id>/album/<upload-id>/thumb.jpg
  │
  └── Turso/SQLite (metadata only: date, size, keys, caption)
        │  existing debounced sync worker
        ▼
     Turso Cloud
```

The browser never talks to R2 directly, so the bucket needs no CORS policy and the
CSP does not need an R2 host. Reads are proxied by Nurtur with immutable cache
headers. The trade-off is that photo bytes pass through the VPS; for a
single-household album that is small compared with the cost of a CORS +
presigned-upload pipeline. If real uploads are too slow on mobile data, a
presigned direct upload can be added behind the same storage interface without
changing the database or the page UI.

### Decisions

| Decision | Choice | Why |
|---|---|---|
| Photo storage | Cloudflare R2, private bucket | Cheap object storage with no egress fee; the user already plans to use it |
| Storage abstraction | `internal/album.Storage` with `local` and `r2` drivers | Tests and local dev need no network; R2 is production-only |
| S3 client | `github.com/minio/minio-go/v7` | Small S3-compatible client; avoids hand-rolling SigV4. Only Put/Get/Remove/Stat are used |
| Upload path | Browser → Nurtur → R2 | Keeps bucket private, avoids R2 CORS, keeps CSP simple |
| Compression | Browser, via canvas | Works on iOS and Android without server-side HEIC decoding; strips GPS and other metadata |
| Variants | `full` (≤2560 px long edge) + `thumb` (≤480 px) | Grid loads small files; viewer loads a sharp enough copy |
| Date source | EXIF `DateTimeOriginal` → file timestamp → upload time | Most phone photos have EXIF; fallback keeps uploads usable |
| Timezone | Store UTC RFC3339; display in `NURTUR_TZ` | Matches every other timestamp in Nurtur |
| Pagination | Whole local days, newest first, cursor on the day boundary | A date heading never splits across pages, same rule as History |
| Delete | Soft delete + undo, retain R2 objects for 30 days | Accidental taps happen; photos are expensive to lose |
| Auth | Existing account middleware; every query and key scoped by `account_id` | A photo URL from another account must return 404 |
| Read cache | `Cache-Control: private, max-age=31536000, immutable` + ETag | Object keys never change; repeat scrolls are instant |
| Local fallback | `./data/album` directory | `make dev`, tests, and e2e stay hermetic |
| Dependency budget | One new module (`minio-go`) | Correctness of S3 signing beats a hand-rolled implementation |

---

## Data model

Migration `migrations/0006_album.sql` adds one table. No existing table changes,
so the Turso sync engine's known `ALTER TABLE ... RENAME` limitation is avoided.

```sql
CREATE TABLE album_photos (
    id              INTEGER PRIMARY KEY,
    account_id      INTEGER NOT NULL DEFAULT 0,
    upload_id       TEXT NOT NULL,              -- client-generated, idempotent retry key
    taken_at        TEXT NOT NULL,              -- UTC RFC3339
    taken_at_source TEXT NOT NULL
        CHECK (taken_at_source IN ('exif', 'file', 'upload', 'manual')),
    caption         TEXT,
    width           INTEGER NOT NULL,
    height          INTEGER NOT NULL,
    bytes           INTEGER NOT NULL,           -- full object
    thumb_bytes     INTEGER NOT NULL,
    content_type    TEXT NOT NULL,              -- image/jpeg or image/png
    full_key        TEXT NOT NULL,
    thumb_key       TEXT NOT NULL,
    created_at      TEXT NOT NULL,
    deleted_at      TEXT
);

-- Retrying a failed upload must not create a second row.
CREATE UNIQUE INDEX one_album_upload
    ON album_photos (account_id, upload_id);

CREATE INDEX idx_album_account_taken
    ON album_photos (account_id, taken_at DESC, id DESC)
    WHERE deleted_at IS NULL;
```

Rules:

- `taken_at` is the local day grouping key converted to UTC.
- `taken_at_source` records how the date was chosen so the UI can say "Date added
  from photo" or "Date corrected" and so a wrong guess can be fixed later.
- `caption` is optional, ≤280 characters, and is edited in Phase 4.
- `upload_id` is a random 128-bit value generated once per photo in the browser
  and reused on every retry.
- `full_key` and `thumb_key` are opaque; the server never accepts a key from a URL.
  The no-JS degraded upload may set both to the same key.
- Soft-deleted rows keep their objects until the 30-day sweep in Phase 5.
- Every read filters `account_id = ? AND deleted_at IS NULL`.

Object keys:

```text
accounts/<account_id>/album/<upload_id>/full.jpg
accounts/<account_id>/album/<upload_id>/thumb.jpg
```

The `upload_id` is also the unique retry key, so a crash between "upload full" and
"insert row" is recoverable by retrying with the same id and overwriting the same
keys.

---

## Routes

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/album` | Full page, newest day first |
| `GET` | `/album/partial?before=` | Earlier days, appended in place |
| `POST` | `/album/photos` | One multipart photo (full + thumb + date + upload id) |
| `GET` | `/album/photos/{id}/thumb` | Thumbnail bytes, account-scoped |
| `GET` | `/album/photos/{id}/full` | Display copy; `?download=1` sets attachment |
| `GET` | `/album/photos/{id}/edit` | Date/caption sheet |
| `PATCH` | `/album/photos/{id}` | Save date/caption; `POST` fallback |
| `DELETE` | `/album/photos/{id}` | Soft delete; `POST /album/photos/{id}/delete` fallback |
| `POST` | `/album/photos/{id}/restore` | Undo delete |

All routes sit inside the existing auth + account middleware. Album routes must
never be added to the admin surface or the static handler.

---

## Configuration

| Variable | Required | Default | Notes |
|---|---|---|---|
| `NURTUR_ALBUM_STORAGE` | no | `auto` | `auto`, `local`, or `r2` |
| `NURTUR_R2_ENDPOINT` | for R2 | — | `https://<account-id>.r2.cloudflarestorage.com` |
| `NURTUR_R2_BUCKET` | for R2 | — | private bucket name |
| `NURTUR_R2_ACCESS_KEY_ID` | for R2 | — | R2 API token access key |
| `NURTUR_R2_SECRET_ACCESS_KEY` | for R2 | — | R2 API token secret |
| `NURTUR_R2_REGION` | no | `auto` | R2 accepts `auto` |
| `NURTUR_ALBUM_DIR` | no | `./data/album` | local driver only |

Rules:

- `auto` uses R2 when all four R2 values are present; otherwise it uses the local
  driver and logs one warning at startup.
- A partially configured R2 block is a startup error, not a silent fallback.
- `NURTUR_ALBUM_STORAGE=r2` without credentials is a startup error.
- The bucket is private. The API token only needs Object Read & Write on that
  bucket. Document the exact token setup in `DEPLOYMENT.md`.
- The existing `/data` volume already covers the local driver, so Docker needs no
  new volume.
- `NURTUR_ALBUM_*` values are wired through `internal/config` and
  `server.Config`, matching how `PublicURL` and the auth settings are passed.

---

## Phase 1 — Foundation, storage, and the empty Album tab

**Goal:** Album exists as a real page and the storage seam works end to end, but
the page has no photos yet. This phase de-risks the two things that are hardest to
change later: the schema and the storage adapter.

**Work**

- Add the `Album` tab to `web/templates/partials/tabbar.html` with a camera icon.
  Change `.tabbar` from `repeat(3, …)` to `repeat(4, …)` and tighten the label
  size below 360 px so four labels still fit.
- Add `camera` to `internal/view/icons.go` in the same hand-drawn stroke style.
- Add `View: "album"` to `layout.html` and hide the quick-log dock on this page
  (the album needs the vertical space for photos and its own add button).
- Add the page shell `web/templates/album.html` with a title, a short subtitle,
  and the existing Pip empty state ("No photos yet. Snap the first one.").
- Add `migrations/0006_album.sql`, `internal/store/album.go`, and the
  `internal/album` domain package (photo struct, validation, date helpers).
- Add `internal/album/storage.go`:
  - `Put(ctx, key, reader, size, contentType) error`
  - `Get(ctx, key) (io.ReadCloser, int64, error)`
  - `Remove(ctx, key) error`
  - `Stat(ctx, key) (size, contentType, error)`
  - `local` driver rooted at `NURTUR_ALBUM_DIR`
  - `r2` driver using `minio-go` with `Region: "auto"` and path-style off
- Wire config, `server.Config`, `cmd/nurtur/main.go`, `.env-example`, and
  `docker-compose.yaml`.
- Add `GET /album` and `GET /album/partial` (empty state for now).
- Add a temporary storage smoke test (local driver) and an opt-in R2 test that
  runs only when `NURTUR_R2_TEST=1` and the R2 values are set.
- Add store tests for insert/list/paging/soft delete/account isolation.
- Run `graphify update .` after the code change.

**Exit criteria**

- The fourth tab appears and is highlighted on `/album`.
- The page renders inside the normal layout, header, and tabbar on a 320 px-wide
  viewport and in night mode.
- `NURTUR_ALBUM_STORAGE=local` survives a restart and writes under `data/album`.
- A manual R2 smoke test can put, stat, get, and remove one object from the private
  bucket using the configured token.
- `gofmt`, `go vet ./...`, and `go test -p 1 ./...` pass.

---

## Phase 2 — Capture and upload

**Goal:** take or choose photos on a phone and have them land in R2 and in the
album without a page reload.

**Work**

- Add a fixed camera button above the tabbar on the Album page and an "Add photos"
  button in the page head. Both open a small chooser:
  - **Take photo** — `<input type="file" accept="image/*" capture="environment">`
  - **Choose photos** — `<input type="file" accept="image/*" multiple>`
- Add `web/static/js/album.js`:
  - Read EXIF `DateTimeOriginal` (tag `0x9003`), `OffsetTimeOriginal` (`0x9011`),
    and orientation from JPEG APP1, using a small in-repo TIFF/EXIF reader. No
    external library and no network request.
  - Decode with `createImageBitmap(file, { imageOrientation: "from-image" })`,
    falling back to an `<img>` + object URL when `createImageBitmap` is missing.
  - Draw to a canvas and export two JPEGs:
    - full: long edge ≤2560 px, quality 0.85
    - thumb: long edge ≤480 px, quality 0.8
  - Do not upscale small sources. Re-encoding strips GPS and other metadata.
  - Generate `upload_id` with `crypto.randomUUID()`, reused for retries.
  - Upload one photo per request with a queue of two, per-photo progress, cancel,
    and retry. Show blob previews immediately; replace them with server tiles on
    success.
  - Fall back cleanly when canvas or JPEG encoding is unavailable: explain that
    the photo could not be prepared, rather than uploading a 12 MB HEIC that the
    server cannot thumbnail.
- Add `POST /album/photos`:
  - One photo per request; max 12 MiB total, full ≤8 MiB, thumb ≤1 MiB.
  - Raise the body limit only for this route; the global 1 MiB limit stays for
    every other mutation.
  - Verify magic bytes (`http.DetectContentType`), `image.DecodeConfig` for
    dimensions (JPEG/PNG), and reject >12000 px on either edge.
  - Validate `upload_id` is exactly 32 lowercase hex characters (or a UUID),
    `taken_at` is RFC3339 and not in the future by more than a day, caption
    length, and content type.
  - Upload full then thumb; if the second upload fails, best-effort remove the
    first. Insert the row last. On `one_album_upload` conflict, return the existing
    photo instead of duplicating it.
  - Return the new tile, plus an OOB day heading when the photo starts a new day.
    If the photo's day is outside the currently loaded range, return a toast with
    the date and a link to that part of the album rather than silently dropping it.
- No-JS fallback: a plain multipart form posts one file; the server stores the
  original as both `full` and `thumb`, uses upload time, and redirects to `/album`
  with a toast. This is deliberately degraded (larger downloads, no EXIF date).
- Add `blob:` to the CSP `img-src` (local previews) and leave camera permissions
  alone: `capture` uses the native camera picker, not `getUserMedia`.
- Tune `http.Server` read/write timeouts for uploads (start at 2 minutes) and
  document the reverse-proxy body limit. If real-world uploads are still slow,
  Phase 5 can switch to presigned direct uploads.
- Date choice: EXIF with offset → exact UTC; EXIF without offset → interpret in
  the device timezone at upload. An EXIF date more than a day in the future or
  more than 10 years old is treated as a bad camera clock and falls back. Then
  file `lastModified` when plausible (same bounds), else upload time. Store the
  source.
- Tests: EXIF fixtures with and without offset, orientation, no EXIF, future
  timestamps, duplicate `upload_id`, oversized payload, wrong content type, and
  account isolation.

**Exit criteria**

- On an iPhone and an Android phone, both "Take photo" and "Choose photos" work,
  including multi-select.
- A photo taken now appears at the top of Today without a reload; a photo with an
  older EXIF date appears under that date (or produces the "saved to that date"
  toast when it is outside the loaded range).
- The same upload retried after a network drop creates exactly one row.
- Dashboard logging still works while an upload is in flight.
- The R2 objects have the expected keys and the bucket is still private.

---

## Phase 3 — Browse the album

**Goal:** scroll through the album comfortably and always see the date and time.

**Work**

- Build `view.AlbumPageView` and `view.AlbumDay` in `internal/view/models.go`:
  - `Days []AlbumDay`, `HasDays`, `HasMore`, `NextCursor`, `Count`
  - each day has `Key`, `Label` (Today / Yesterday / weekday), and `Rows`
  - each row has `ID`, `TakenAt`, `TimeLabel`, `Alt`, `ThumbURL`, `FullURL`,
    `Width`, `Height`, `AspectRatio`, `Caption`
- Reuse `clock.DayBounds` and the existing `previousLocalDayStart` helper. Load
  whole days newest-first until the page budget (start at 60 photos, max 7 days)
  is met; a busy day loads up to 500 photos. Cursor is the start of the oldest
  loaded day.
- Render days as sections with a sticky day heading and a responsive grid:
  `repeat(auto-fill, minmax(108px, 1fr))`, 3 columns on a typical phone.
  Each tile shows the time in a bottom gradient; the full date stays in the
  heading and the viewer.
- Use `loading="lazy"`, `decoding="async"`, and explicit `width`/`height` (or an
  aspect-ratio box) so images never push the grid around while scrolling.
- Add "Load earlier days" using the same HTMX pattern as History. Keep scroll
  position on append. Add a small "N photos" count in the page head.
- Add `GET /album/photos/{id}/thumb` and `/full`:
  - Look up the row by `id` and `account_id`; 404 for deleted or foreign rows.
  - Stream from the storage adapter with `Content-Type`, `Content-Length`,
    `ETag`, and `Cache-Control: private, max-age=31536000, immutable`.
  - `?download=1` sets `Content-Disposition: attachment` with a safe filename
    such as `nurtur-2026-10-27-1542.jpg`.
- Add empty, first-load skeleton, upload-in-progress, and load-error states.
- Make sure the service worker does not try to cache album HTML or photo URLs:
  it already only handles `/static/` and navigations, so no shell change is
  needed. `album.js` is a normal `/static/` asset and will be cached by the
  existing cache-first rule; if its URL or the CSS changes in a way that matters,
  the asset fingerprint in `asset` already busts the HTML reference.
- Tests: paging across day boundaries, a day with no photos, cache headers,
  deleted photo 404, foreign account 404, and grid rendering in the server test.

**Exit criteria**

- Scrolling a few hundred photos stays smooth on a phone; thumbnails are visibly
  smaller than the full copies.
- Every tile shows a time, and every day heading shows the date in the household
  timezone.
- Loading earlier days keeps the current scroll position and never splits a date.
- A copied `/album/photos/{id}/thumb` URL from another account returns 404.
- Night mode and reduced-motion preferences are respected.

---

## Phase 4 — View, correct, and delete

**Goal:** make the album safe to live with: open a photo, fix a wrong date, add a
caption, and undo an accidental delete.

**Work**

- Add a `<dialog class="album-viewer">` full-screen viewer:
  - full image, full "Monday, August 12 at 3:42 PM" label, caption
  - previous/next buttons, keyboard arrows, Escape to close
  - horizontal swipe with pointer events; no drag when the user prefers reduced
    motion
  - restore focus to the tile that opened it
  - prefetch only the next/previous full image, not the whole album
- Tile click opens the viewer with HTMX/JS. A plain link to the full image is the
  no-JS fallback.
- Add the date/caption sheet:
  - `datetime-local` input prefilled in `NURTUR_TZ`, caption textarea
  - validation: required date, not more than a day in the future, caption ≤280
  - on save, update `taken_at`, `taken_at_source = 'manual'`, and `caption`
  - if the photo stays in the same day, update the tile and viewer in place; if
    it moves to another day, remove the old tile, update the old day's count, and
    show a toast with a link to the new date (a full reload is acceptable here if
    in-place movement gets noisy)
- Delete:
  - soft delete the row, remove the tile, show the standard toast with Undo
  - if that was the last photo in a day, remove the day heading
  - undo restores the row and tile; R2 objects were never removed
  - deleted photos are excluded from all reads immediately
- Add a 30-day retention sweep in Phase 5, not here. Until then deleted objects
  stay in R2, which is intentional and safe.
- Add `GET /album/photos/{id}/full?download=1` as the "Save photo" action.
- Tests: edit date within a day, edit date across days, caption trim/length,
  delete/restore, viewer markup, and no-JS fallbacks.

**Exit criteria**

- A photo with a wrong camera clock can be corrected and moves to the right day.
- Deleting a photo is recoverable for the toast window, and undo puts it back.
- The viewer is usable with one thumb and with a keyboard.
- Full date and time are visible in the viewer and correct in `NURTUR_TZ`.

---

## Phase 5 — Harden, polish, and ship

**Goal:** make the album safe to leave running, document it, and release it.

**Work**

- Hardening:
  - validate magic bytes and dimensions before storing; never fully decode user
    images on the server
  - cap photos per upload batch and requests per minute per account (reuse a
    small in-memory limiter if one already exists; otherwise add a minimal one)
  - strip/reject suspicious filenames; never use the uploaded filename as a key
  - ensure `?download=1` cannot reflect a filename into headers
  - add `X-Content-Type-Options: nosniff` (already global) and only serve
    `image/jpeg`/`image/png`
- Retention sweep:
  - on startup and once a day, hard-delete rows whose `deleted_at` is older than
    30 days, after removing their R2 objects
  - best-effort cleanup on upload failure; log orphaned objects but do not block
  - expose a single structured log line for each sweep
- Backups:
  - document that Turso sync carries album metadata but not photo bytes
  - recommend R2 versioning and a separate bucket backup (rclone or Cloudflare's
    tools); include album rows in `GET /export.json` with keys and dates
  - note that restoring a bucket with the same keys restores the photos
- PWA/offline:
  - bump the service-worker `VERSION` only if the shell changed
  - verify album navigations show the existing offline page gracefully
  - do not queue uploads for background sync; show "Connection lost — retry"
  - verify the browser cache does not serve a photo after it was deleted (the
    proxy returns 404; the tile is removed)
- Accessibility and visual polish:
  - alt text from caption or "Photo from 3:42 PM"
  - focus rings, 44 px tap targets, contrast in day/night themes
  - reduced-motion viewer transitions
  - empty and error copy in the Nurtur voice
- Performance:
  - measure first grid paint and a 500-photo scroll on a real phone
  - confirm thumbnails are ≤~80 KB and full copies are ≤~2 MB for typical photos
- Tests and docs:
  - store/server/e2e coverage for the full flow
  - update `README.md`, `DEPLOYMENT.md` (R2 bucket + token + proxy body limit),
    `.env-example`, and this file's status
  - add a `v0.5.0` release to `internal/changelog/changelog.go` and bump the
    version (new top-level page = major change under the repo's versioning rule)
  - run `gofmt`, `go vet ./...`, `go test -p 1 ./...`, and `make e2e`
  - run `graphify update .`
- Do not create a tag or push unless explicitly asked.

**Exit criteria**

- CI is green and the e2e suite covers upload → browse → view → edit → delete →
  undo.
- The album works on an iPhone, an Android phone, and a desktop browser.
- The R2 bucket is still private, credentials are only in the environment, and
  the deployment doc explains how to back up and restore the photos.
- Release notes describe the new tab and how photos are stored.

---

## Cross-phase guardrails

- Every phase ends with `gofmt`, `go vet ./...`, and `go test -p 1 ./...`.
- Run `make e2e` when a phase touches navigation, templates, or `album.js`.
- Keep `PageData` the single page model; do not add a second parallel layout.
- Keep every mutation a real form first and an HTMX enhancement second.
- Reuse `view.FormatClock`, `view.DayLabelRelative`, `clock.DayBounds`, and the
  existing toast/dialog patterns instead of inventing album-specific formatting.
- Never expose R2 keys, bucket names, or credentials in HTML, logs, or errors.
- Keep the album out of the service-worker shell cache and out of the offline
  HTML cache.
- Run `graphify update .` after each code phase.

---

## Risks and mitigations

| Risk | Why it matters | Mitigation |
|---|---|---|
| HEIC photos from iPhones | Browsers differ in decode support; the server should not decode HEIC | Re-encode in the browser with canvas; show a clear error if the browser cannot decode; test on a real iPhone in Phase 2 |
| Wrong EXIF timezone | `DateTimeOriginal` has no timezone | Use `OffsetTimeOriginal` when present, otherwise device timezone; store the source and allow manual correction in Phase 4 |
| Slow mobile uploads through the VPS | 2–3 MB can take a while on a weak connection | Compress to ≤2560 px, show progress/retry, tune timeouts; if needed, add presigned direct upload in Phase 5 |
| Orphaned R2 objects | Failed inserts or crashes can leave bytes with no row | Upload keys derive from `upload_id`; retries overwrite; best-effort delete on failure; document a manual bucket audit |
| Large albums | Hundreds of full images would hurt scrolling | Thumbnails, lazy loading, whole-day pagination, immutable caching |
| Private photos in browser cache after logout | Shared-device privacy | `Cache-Control: private`; document the trade-off; do not use a public bucket or public URLs |
| R2 outage | Uploads and image reads fail | Album fails independently; logging continues; show a friendly retry state |
| Turso/R2 backup mismatch | DB can be restored without the bucket, or the reverse | Document separate backups and key stability; include metadata in JSON export |
| Four nav items on narrow phones | Labels can crowd or wrap | Tighten tabbar sizing at ≤359 px; test 320 px width in Phase 1 |

---

## Out of scope (for now)

- Video, Live Photos, or audio.
- Face detection, AI tagging, or automatic "best photo" selection.
- Public sharing links, comments, or household member roles.
- Multiple babies per account.
- Maps or GPS display (GPS is stripped during re-encoding).
- Full-resolution RAW/HEIC originals.
- Background/offline upload queue.
- Editing the image itself (crop, filters, rotate after upload).

---

## Manual test matrix

Run before calling the album shipped:

- iPhone Safari: camera capture, photo library, multi-select, HEIC source, screen
  lock during upload, night mode, installed PWA.
- Android Chrome: camera capture, photo library, multi-select, rotation, weak
  network, back button in the viewer.
- Desktop Chrome/Firefox: drag-and-drop, file picker, keyboard viewer, no-JS
  fallback.
- Two accounts on the same server: each sees only its own photos, including
  guessed photo IDs.
- Date edge cases: photo just before midnight, photo with no EXIF, camera clock
  wrong, photo taken in another timezone, future timestamp.
- Delete/undo, delete the last photo in a day, delete while another tab is open.
- R2 misconfiguration: upload shows a clear error; dashboard still works.

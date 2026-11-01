## Context

Grass has a flag-only Go CLI, a separate scheduled Lambda entry point, and two storage implementations. `storage.Storer` exposes collector writes and duplicate/cursor checks but no archive queries. `SearchResult` contains `Content`, but neither backend currently persists it; there are no structured author fields. Some searchers use collection time as `Timestamp`, while others use publication time. DynamoDB also contains platform cursors, Lambda leases, and YouTube budget items in the same table.

The requested experience is archive exploration: a Go server and polished Vue frontend launched with `grass serve`, connecting to the existing database. It does not need a tailnet endpoint or changes to where Lambda runs.

## Goals / Non-Goals

**Goals:**
- Search and browse previously collected results without calling external platforms.
- Link keyword counts, word clouds, author rankings, and activity charts to the same search/filter state.
- Provide meaningful functionality for existing archives and richer analytics for newly collected results.
- Ship an embedded frontend in a Go executable and preserve existing collection commands.
- Support DynamoDB and SQLite with explicit, bounded read costs and honest data coverage.

**Non-Goals:**
- Editing keywords, credentials, schedules, or other collector configuration in this change.
- On-demand provider searches, historical API backfills, or a provider proxy.
- Tailscale integration, Lambda-hosted HTTP ingress, public multi-tenant hosting, or a new search service.
- Claiming rankings represent all activity on a platform or inferring identities from titles/URLs.

## Decisions

### 1. A separate serve mode inside the existing CLI

Dispatch `serve` explicitly before invoking the existing flag-only collector parser; parse its options using an isolated parser rather than making a new default subcommand that changes existing invocations. Support `grass serve --db=dynamodb --table-name=grass --listen=127.0.0.1:8080` and the equivalent SQLite selection. Continue honoring `SOCIAL_SEARCH_TABLE_NAME`, AWS SDK credentials, and optional `.env` loading. Provider constructors and notifier setup are never invoked in serve mode.

Use `net/http` and an archive service package, with a separate read-only `ArchiveReader` interface alongside `Storer`. Keep database credentials inside backend storage initialization. Default to loopback and allow an explicit listen override for deployment behind the user's chosen proxy. No built-in login is added; remote hosting documentation must state that access control belongs to the hosting environment.

Alternative: a separate UI daemon executable. Rejected because the requested `grass serve` interface and single downloadable executable are simpler to use.

### 2. Additive archive metadata, populated from existing responses

Add optional `AuthorID`, `AuthorHandle`, and `AuthorName` strings to `SearchResult` and persist those plus existing `Content`. Author identity is scoped to `Platform`: use stable IDs when available; otherwise an explicit normalized handle from the provider response. Handle-only fallback uses Unicode lowercase and trim; names alone never establish identity.

Provider mappings:

| Provider | Identity and label | Content already available in search response |
| --- | --- | --- |
| Hacker News | `author` username as handle; no invented numeric ID | `comment_text`, and `story_text` when supplied |
| Reddit | `author_fullname` when present, otherwise `author`; omit `[deleted]` | `selftext` when present |
| Bluesky | author DID, handle, display name | record text |
| Fediverse | account URL/URI as globally qualified ID, full `acct` with instance qualification when local, display name | status content |
| YouTube | snippet channel ID, channel title as display name | snippet description |
| X | `author_id`, expanded username/name when supplied | post text |

Read available response fields without new enrichment calls or wider pagination. Store missing values as absent/empty. Do not change existing result keys, duplicate suppression, or notification ordering; old duplicates are not re-notified or re-fetched to fill fields. Analytics describe the stored first-match keyword, not every possible match or the live configured keyword list.

SQLite writer initialization adds missing columns idempotently using schema inspection. The serve reader uses read-only mode and supports pre-migration schemas by selecting empty aliases for missing columns; it must not create or migrate a database. DynamoDB fields are additive and missing fields decode to empty. Old collectors can continue reading data; rollback does not require removing columns.

Keep `Timestamp` semantics unchanged and label time-based views as **recorded timestamp**. Correcting all provider watermark semantics and reconstructing historical publication times are separate changes.

### 3. A bounded in-memory archive snapshot for the first version

Load a complete bounded snapshot on server startup in the background. SQLite reads a consistent transaction; DynamoDB follows `LastEvaluatedKey` through sequential Scan pages with projection of archive fields. Exclude reserved metadata partitions and `LastSearchTime` rows and require result fields including a valid HTTP(S) URL. DynamoDB scans are not a point-in-time transaction; report a load time rather than claiming strict database snapshot isolation.

Defaults: at most 50,000 result records, 100,000 examined source items, 64 MiB of decoded record data, and 10 minutes per load. Expose positive overrides through serve flags `--archive-max-records`, `--archive-max-scanned`, `--archive-max-bytes`, and `--archive-load-timeout`. DynamoDB uses pages of at most 100 examined items and requests consumed capacity, pacing subsequent pages to `--archive-read-capacity` (default 2 RCU/second). Pacing is cancellation-aware. These defaults accommodate the observed roughly 25,000-item archive on a 5-RCU provisioned table. Cancel the operation promptly on limits or timeout; never publish truncated results as a complete snapshot. Counters include skipped metadata for the examined-item limit, and readers enforce bounds during loading rather than after accumulating the full table.

Log table/region configuration, load start, periodic progress, successful completion, and underlying failures to the server console. HTTP responses provide actionable sanitized classifications. Initial failed loads return `archive_load_failed`, not `archive_loading`. Status exposes the next permitted refresh time; the UI shows a countdown and preserves the original load error instead of replacing it with a cooldown message.

Publish a completed snapshot atomically with an opaque ID and loaded-at timestamp. Initial API reads return `503 archive_loading` until ready. Keep serving the last successful snapshot if refresh fails, exposing the failure and stale state. A Refresh action starts a background reload, coalesces concurrent requests, and allows at most one new load per minute. No full scan is triggered by a search keystroke or a status poll. This version has no automatic periodic scanning.

Alternative: DynamoDB queries plus scans per UI request. Rejected because the current indexes cannot efficiently implement arbitrary text filtering and consistent aggregate calculations. Alternative: OpenSearch or another indexing database. Deferred until archive size or latency warrants operating another service. Above the stated snapshot limits, show an actionable limit error; increasing limits is explicit rather than silently increasing read costs.

### 4. One shared filter and aggregation pipeline

Expose:
- `GET /api/status`: source type, loading/ready/error state, active snapshot ID, load time, refresh state, and non-secret diagnostics.
- `GET /api/archive`: filtered page plus total matches, facets, analytics, and data-coverage counts from one snapshot.
- `POST /api/archive/refresh`: initiate/coalesce a database reload, returning `202` while accepted/in progress or `429` during the reload cooldown.

`/api/archive` parameters: `q` (maximum 256 characters), repeatable `platform` and `keyword`, optional opaque `author`, optional normalized word-cloud `term`, RFC3339 `from`/`to`, `page` (default 1), `page_size` (default 25, maximum 100), and optional `snapshot`. Text search lowercases and whitespace-splits `q`; every term must appear as a literal substring in the combined plain-text title/content/author label. Filters across categories intersect; multiple values within a category are ORed. Time bounds use `from <= Timestamp < to` in UTC. Invalid parameters return a structured `400`.

Sort by timestamp descending, then platform and URL ascending for ties. Distinct results are identified by `(Platform, URL)`. Apply filters and aggregates before pagination. Return an opaque stable record ID, stored fields, normalized author identity, and plain-text content. Explicit snapshot IDs must match the active snapshot or return `409 snapshot_changed`; the UI then resets pagination and reloads. This prevents mixing pages silently after refresh without retaining arbitrarily many snapshots.

All aggregates describe the entire filtered set, not the current page:
- Platform and stored-keyword counts: one contribution per archived result with that value; label missing keywords separately.
- Activity: UTC calendar-day buckets, including zero-count dates between the first and last filtered date.
- Author ranking: top 20 by archived result count, ties by platform and identity; exclude unknown identities and report known/unknown coverage. Never merge the same handle across platforms.
- Word cloud: top 50 terms from plain-text title plus content. Unicode lowercase and letter/digit tokenization, 3–40 characters, excluding standalone numbers, URLs, and a versioned built-in English stopword list. Count each term once per result (document frequency) so repeated text cannot inflate a result's influence. Ties sort lexically. The optional `term` filter uses the exact same tokenization. Report records with/without content; legacy titles still contribute.

Return empty arrays and zero counts for an empty selection. Do not cache by user input indefinitely; filtering runs over the bounded immutable snapshot.

### 5. Vue frontend with a deliberate visual hierarchy

Use Vue 3, TypeScript, Vite, Vue Router, Tailwind CSS, and accessible headless primitives where needed. Prefer simple SVG charts and a deterministic, responsive word-cloud layout over an additional heavyweight dashboard stack. Use Vitest/Vue Test Utils and Playwright for interaction verification.

Desktop layout:

```text
Grass / Archive                         Source · Last refreshed · Refresh
Search collected posts…                 platform / keyword / recorded time
Active filter chips                     Clear all
┌──────────────┬──────────────┬──────────────┬───────────────────────────┐
│ Results      │ Known authors│ Content cover│ Activity over time        │
└──────────────┴──────────────┴──────────────┴───────────────────────────┘
┌──────────────────────────────────────────┬────────────────────────────┐
│ Result cards/table + pagination          │ Keywords                   │
│                                          │ Word cloud + count list    │
│ Select result → detail drawer            │ Most active authors        │
└──────────────────────────────────────────┴────────────────────────────┘
```

Use a restrained botanical green accent, neutral surfaces, readable typography, consistent spacing, platform badges, and light/dark modes. Collapse to a stacked mobile layout. Persist filter state in the URL; debounce typing and abort/ignore obsolete responses. Reset page on filter changes. Clicking a keyword, author, or cloud term updates the same filters and all views. Chart interaction has a keyboard-operable equivalent, visible counts, and no color-only meaning. Provide skeleton loading, retryable errors, stale-data indicators, honest coverage labels, empty archive and empty-filter states, and a detail drawer with source link and available text/author fields. Render provider content as text, not trusted HTML, and allow only HTTP(S) external links.

### 6. Embed production assets without burdening Lambda builds

Put frontend source under `web/`; generate its production assets into the web-serving Go package's embedded filesystem. Keep an intentional tracked placeholder asset so `go test ./...` can compile on a clean checkout; `grass serve` detects a placeholder build and gives the exact frontend-build command. Release packaging must always build the real assets first and verify the placeholder is absent. Released binaries must not require Node, npm, or a development server at runtime. The separate `cmd/lambda` executable must not import the web package.

Add reproducible npm scripts/lockfile and Make targets for frontend development and release builds. Vite proxies `/api` to the Go server in development; production is same-origin. Update GoReleaser hooks and CI to build frontend assets once before cross-compiling CLI targets. Preserve the existing CGO-disabled Lambda path. Existing SQLite support uses `go-sqlite3` and needs a CGO-enabled build; document/test SQLite in that build mode and DynamoDB in the existing cross-platform CGO-disabled release mode, rather than silently claiming to fix the existing SQLite release limitation.

## Risks / Trade-offs

- [Legacy records lack authors/content] → Show coverage explicitly; enrich future collection only and avoid author inference.
- [Full archive reads consume DynamoDB capacity] → Sequential paginated loads, bounded records/bytes/items/time, manual refresh throttling, and documentation of read costs; use a separate read-only server identity with table Scan permission.
- [Large archives exceed the initial architecture] → Fail visibly, retain the last complete snapshot, and document limits; future indexed search is a separate design.
- [Mixed timestamp provenance] → Label recorded timestamps consistently and do not promise publication-time analytics.
- [First-match keywords underrepresent multi-keyword overlap] → Explain metrics as stored matches and preserve deduplication semantics.
- [Untrusted post content reaches browsers] → Render text and validate source-link schemes; no raw HTML rendering.
- [CLI parser or frontend asset changes disrupt collection/release] → Regression-test legacy invocation and isolate the web package from Lambda; exercise clean-checkout builds.
- [No built-in authentication] → Loopback by default and explicit hosting documentation for non-loopback deployment.

## Migration Plan

1. Add metadata parsing and additive persistence, with fixtures for both old and new records; deploy collector updates independently if desired.
2. Add read-only readers and bounded query/analytics service; verify against synthetic SQLite and DynamoDB fixtures before accessing a live archive.
3. Build the Vue UI and embedded serve command, then verify live read-only archive browsing without provider credentials.
4. Update release/CI workflows and document build modes, database read permissions, snapshot limits, and incomplete historical analytics.
5. Roll back the server by stopping it or using the prior binary. Additive storage attributes/columns can remain. No historical rewrite or destructive database migration is required.

## Open Questions

No blocking decisions remain. Actual archive size and preferred visual details can be evaluated during implementation within the bounded-read and accessibility requirements above. Full-text indexing beyond the snapshot limits, management editing, and historical backfill are deferred follow-up capabilities.

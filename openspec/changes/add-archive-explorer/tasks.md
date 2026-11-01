## 1. Archive Metadata And Backward Compatibility

- [x] 1.1 Add optional author ID, handle, and display name fields to `SearchResult`; define platform-scoped author identity and unknown/deleted-author handling without changing provider interfaces.
- [x] 1.2 Populate author/content fields from existing Hacker News, Reddit, and Bluesky response payloads; add fixture tests covering missing fields and deleted authors without extra network calls.
- [x] 1.3 Populate author/content fields from existing Fediverse, YouTube, and X response payloads; test instance-qualified Fediverse identities, YouTube channel IDs, and X author expansions.
- [x] 1.4 Persist content and author fields in DynamoDB and SQLite, adding idempotent collector-side SQLite column migrations; verify old schemas and legacy items remain compatible.
- [x] 1.5 Add regression coverage demonstrating duplicate results are still suppressed and metadata changes do not cause additional notifications, provider requests, or timestamp/key changes.

## 2. Read-Only Archive Access

- [x] 2.1 Define a context-aware archive reader boundary and normalized record model alongside `Storer`, with stable result IDs, plain-text normalization, and platform-scoped author IDs.
- [x] 2.2 Implement read-only SQLite archive loading with legacy-column aliases, transaction-scoped reads, missing-file errors, and tests proving serve reads do not create or migrate the database.
- [x] 2.3 Implement paginated DynamoDB archive loading with projected fields and operational-item exclusion; test multiple pages, empty pages with continuation keys, legacy records, and mixed cursor/lease/quota items.
- [x] 2.4 Enforce result-count, examined-item, byte, and timeout limits incrementally in both readers; test boundary cases and cancellation without publishing partial data.

## 3. Snapshot, Search, And Analytics Service

- [x] 3.1 Implement initial background loading, atomic snapshot publication, snapshot IDs/load times, and retention of the last successful snapshot on refresh failure.
- [x] 3.2 Implement manual refresh coalescing and one-minute reload throttling; test concurrent refresh requests and verify queries/status polling never trigger scans.
- [x] 3.3 Implement shared text/platform/keyword/author/term/time filtering, deterministic sort, and pagination with the documented validation bounds and snapshot-conflict behavior.
- [x] 3.4 Implement whole-filtered-set keyword/platform counts, UTC daily activity buckets, and top-author rankings with metadata coverage; test cross-platform handle collisions and legacy-only archives.
- [x] 3.5 Implement deterministic word-cloud document frequencies, versioned stopwords, Unicode tokenization, HTML/URL exclusion, and exact token drill-down; verify repeated words count once per record.
- [x] 3.6 Add fixture-based tests proving aggregates match full filtered result sets across page boundaries and return coherent zero/empty values for no matches.

## 4. Go HTTP Server And CLI

- [x] 4.1 Add isolated `grass serve` argument dispatch, database/listen/limit options, and environment handling; regression-test legacy collection flags, help/version behavior, and operation without provider credentials.
- [x] 4.2 Implement `/api/status`, `/api/archive`, and `/api/archive/refresh` with structured response/error contracts and HTTP tests for validation, loading, stale snapshots, refresh throttling, and snapshot replacement.
- [x] 4.3 Add embedded-asset serving with SPA/API route separation, placeholder-build detection, loopback default, startup diagnostics, and cancellation-aware graceful shutdown.

## 5. Vue Archive Experience

- [x] 5.1 Scaffold Vue 3/TypeScript/Vite with locked dependencies, Vue Router, Tailwind styling, API proxying, and frontend test scripts; establish typography, green accents, spacing, light/dark themes, and responsive shell.
- [x] 5.2 Build search, platform/keyword/author/time filter controls, removable chips, result list, pagination, and detail drawer with URL-synchronized state and accessible source links.
- [x] 5.3 Add debounced query loading, cancellation/obsolete-response handling, browser back/forward restoration, and snapshot-conflict recovery; test the corresponding interactions.
- [x] 5.4 Build linked keyword counts, daily activity visualization, deterministic word cloud/count list, and most-active-author panels using the API's filtered aggregates.
- [x] 5.5 Add skeletons, initial/refresh status, source/freshness labels, content/author coverage, empty archive/no-match views, and retryable stale/error/limit states.
- [x] 5.6 Verify keyboard operation, visible focus, light/dark contrast, narrow-screen layouts, safe text rendering, and all chart drill-downs with component tests and focused browser tests.

## 6. Packaging, Documentation, And Final Verification

- [x] 6.1 Add reproducible frontend/embedded CLI Make targets and update GoReleaser/CI to build production assets before release compilation; ensure clean-checkout Go tests work with placeholder assets.
- [x] 6.2 Verify a built CLI serves the embedded production UI without Node at runtime, CGO-enabled SQLite works with a legacy fixture, and the CGO-disabled DynamoDB CLI and standalone Lambda bootstrap still build.
- [x] 6.3 Document serve usage, database/read-only IAM setup, developer workflows, archive limits/refresh costs, recorded-time and legacy-metadata semantics, non-loopback hosting, and SQLite build requirements.
- [x] 6.4 Run frontend type checks, production build, component tests, and browser tests covering archive filtering, linked analytics, legacy-data coverage, refresh errors, and mobile navigation.
- [x] 6.5 Run standard Go formatting and `go test ./...`, review requirement coverage against the four capability specs, and record any remaining verification limitations.

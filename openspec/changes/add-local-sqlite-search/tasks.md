## 1. Local SQLite Foundation

- [x] 1.1 Pin a pure-Go SQLite driver, prove FTS5 create/insert/MATCH support with CGO disabled, verify supported release-target builds, and confirm coexistence with the collector's existing SQLite driver.
- [x] 1.2 Add a serve-only local-index package with deterministic default paths, explicit path override, source binding, schema-version checks, and rejection of a source/index path collision.
- [x] 1.3 Implement transactional migrations for records, FTS5 triggers, distinct cloud terms, metadata, bootstrap staging, and continuation/checkpoint state; verify reopening preserves data and generation identity.
- [x] 1.4 Configure WAL/read concurrency and a fenced renewable sync-writer lease; test competing owners, takeover, stale-owner commit rejection, and rollback on disk/write failures.

## 2. SQL Search And Analytics

- [x] 2.1 Implement safe literal-token AND query construction and FTS5 matching across title/content/author fields; test Unicode/diacritics, punctuation-only input, operator-like input, complete-token semantics, and the text-length limit.
- [x] 2.2 Implement parameterized platform/keyword/author/term/time filters, stable ordering, pagination, and total counts over the local records table; verify existing category-combination and time-boundary behavior.
- [x] 2.3 Implement SQL keyword/platform/daily/author/term aggregates and metadata coverage using the complete filtered set; preserve cloud-token semantics and define/test bounded handling of malformed recorded-date spans.
- [x] 2.4 Run each response in one read transaction with durable generation checks; test concurrent sync commits, no-op synchronization, stale snapshot requests, and empty/legacy datasets without whole-archive Go allocations.

## 3. Collector Change Journal

- [x] 3.1 Define reserved journal control/event keys, epoch/version/sequence encoding, reference event payload, and persisted write identity; add decoding and archive-exclusion fixtures.
- [x] 3.2 Implement strongly consistent sequence allocation and atomic conditional result/counter/event transactions with stable attempt tokens, bounded retries, and uncertain-outcome resolution.
- [x] 3.3 Group configured keywords into a provider run using one starting watermark; advance to run-start only after all searches/checks/writes succeed, retain cursors on failure or quota skips, and handle conditional duplicates without re-notifying; regression-test early-keyword failure followed by later success.
- [x] 3.4 Test initial control creation, concurrent writers, duplicate inserts, timeout-after-commit recovery, throttling, sequence overflow/version errors, and transaction atomicity.
- [x] 3.5 Review Terraform and alternative deployment IAM for actual transaction item actions and local-reader GetItem/Query/Scan access; document incremental write/storage cost and verify the Lambda build excludes the local-index package.

## 4. Source Adapters And Synchronization

- [x] 4.1 Refactor archive reading into context-aware streaming/page adapters so importing never requires a complete source slice; expose strong control reads, ordered journal Query, and referenced-result BatchGet for DynamoDB.
- [x] 4.2 Share consumed-capacity pacing across Scan/Query/BatchGet, retry unprocessed keys with bounded backoff, and preserve cancellation/resource bounds; test actual request consistency flags and read-budget accounting.
- [x] 4.3 Implement resumable staged bootstrap with persisted initial epoch/checkpoint and atomic page/continuation commits; test restart at each phase and operational-item exclusion.
- [x] 4.4 Implement post-scan journal catch-up and atomic initial publication; test writes concurrent with scanning, duplicate observations, pre-bootstrap committed records, changed epochs, and limits exceeded before publication.
- [x] 4.5 Implement bounded incremental replay through a captured high-water mark, transactional contiguous checkpoints, FTS/term updates, and no-op detection; verify ordinary cached sync performs no DynamoDB Scan.
- [x] 4.6 Implement explicit staged reconciliation, missing-event/reference detection, legacy-mode bootstrap, and journal activation handling; test failed reconciliation preserving the old generation and removal of source-absent local rows on successful replacement.
- [x] 4.7 Add read-only streaming SQLite-source import with legacy-schema aliases and atomic changed-data replacement; verify the source file/schema is never modified and no-op imports preserve generation IDs.
- [x] 4.8 Add background scheduling, zero-interval opt-out, coalescing/cooldown, distinct reconciliation request handling, progress/error state, and cancellation-aware shutdown; test offline recovery without blocking cached reads.

## 5. Serve API And Vue Integration

- [x] 5.1 Add `--archive-index` and `--archive-sync-interval`; open a committed local index and bind HTTP before remote initialization, preserving existing source/environment options and synchronization limits.
- [x] 5.2 Replace in-memory query service wiring with the SQL index and sync coordinator; retain response field compatibility and add sync phase/progress/checkpoint/target/freshness fields plus explicit reconciliation routing.
- [x] 5.3 Update the Vue UI for immediate cached results, background synchronization, legacy mode, reconciliation, and literal-token search guidance; preserve URL filters, linked analytics, errors during cooldown, and generation-conflict recovery.
- [x] 5.4 Add HTTP/component/browser tests for cached startup with unavailable AWS, initial bootstrap, sync failure with visible cached results, no-op sync, reconciliation, token search, and mobile/keyboard interactions.

## 6. Packaging, Documentation, And Verification

- [x] 6.1 Update build/release/CI checks to exercise the pure-Go local index and FTS5 under CGO-disabled CLI builds, embedded production assets, existing collector compatibility, and frontend-independent Lambda builds.
- [x] 6.2 Document index location/lifecycle, background interval, strong bootstrap read cost, journal deployment order, IAM, legacy-mode reconciliation, token semantics, source identity, and rollback/retention limitations.
- [x] 6.3 Verify with a representative 25,000-record fixture that restart serves cached data before any remote access, only paginated results are materialized, and incremental sync reads only journal additions and their referenced records; record startup/query/memory observations.
- [x] 6.4 Run crash/restart and concurrent-reader/writer tests, `go test ./...`, focused race checks, frontend type checks/build/tests, and OpenSpec validation; record live-testing and cross-platform verification scope.
- [x] 6.5 Review the four new capability specs against implementation and prepare baseline-spec integration notes for archiving `add-archive-explorer` before this change, explicitly reconciling its superseded in-memory, substring-search, and refresh requirements.

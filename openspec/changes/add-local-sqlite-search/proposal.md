## Why

The explorer currently rebuilds an in-memory snapshot at every startup. Loading the live archive of roughly 25,000 records takes about 4.5 minutes on its provisioned DynamoDB table; a durable local search index would make subsequent starts immediate and avoid repeated full-table reads.

## What Changes

- Replace the serve-mode in-memory dataset with a persistent local SQLite database, FTS5 text index, and SQL-backed filtering and analytics.
- Open the last committed local generation immediately, including when AWS credentials or the remote source are temporarily unavailable.
- Bootstrap DynamoDB once with a paced scan, then synchronize in the background through a durable, ordered change journal written atomically with new collector results.
- Persist source identity, schema version, sync checkpoint, and generation metadata so interrupted syncs recover without duplicates or gaps.
- Keep the existing Vue explorer and result/analytics response shapes, with explicit synchronization progress, stale/error states, and a documented change from substring search to literal token search.
- Add local-index path, sync interval, and explicit reconciliation controls while retaining existing database selection and rate-limited reads.
- Support existing SQLite collection databases through a separate local index without modifying the source database.

## Capabilities

### New Capabilities
- `local-search-index`: Durable SQLite schema, FTS5 availability, source binding, local generations, and SQL-backed archive queries/analytics.
- `archive-change-journal`: Atomic DynamoDB result and ordered journal writes that allow reliable incremental consumption without timestamp-based watermarks.
- `local-archive-sync`: Bootstrap, checkpointed background incremental sync, reconciliation, recovery, and local-source import.
- `local-search-serving`: Serve-mode lifecycle, synchronization API/UI integration, CLI options, packaging, and diagnostics for the local index.

### Modified Capabilities

None in the main specification tree: `openspec/specs/` is currently empty. This change builds on the implemented, unarchived `add-archive-explorer` change; its specs describe that baseline, while these new capabilities define the replacement persistence/sync behavior. Archive the explorer baseline before this change so both sets of specifications retain their intended order.

## Impact

- `archive/`, `storage/`, and `webserver/`: replace full-snapshot query evaluation with transactional local queries, add sync workers and collector journal writes, and preserve existing HTTP field shapes and metadata coverage semantics.
- CLI: introduce `--archive-index`, `--archive-sync-interval`, and an explicit reconciliation action; retain `--db`, `--table-name`, AWS configuration, read-budget and load-limit controls. No new provider credentials or calls are introduced.
- Persisted data: add local SQLite/FTS5 tables and additive reserved DynamoDB journal/control items. Existing result keys and recorded timestamps remain unchanged; no historical provider backfill occurs.
- AWS: reader permissions include table `Scan`, `Query`, `GetItem`, and `BatchGetItem`; collector IAM must authorize the operations used in atomic transactions. Journal retention consumes additional storage and transaction writes increase write capacity usage. No new GSI or external search service is required.
- Build/release: choose a pure-Go SQLite driver with verified FTS5 support for the new local-index package so existing CGO-disabled CLI release targets can serve DynamoDB-backed archives. Preserve the current collector SQLite driver and standalone Lambda build boundary.
- Frontend/docs/tests: distinguish local readiness from source-sync readiness, document token search and compatibility with older collectors, and verify crash/restart, transactional consistency, and incremental read behavior.

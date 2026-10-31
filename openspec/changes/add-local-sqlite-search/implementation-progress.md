## Implementation complete

All 30 tasks are complete. `grass serve` now uses `localindex/`,
`archivesource/`, and `archivesync/` for durable local SQLite/FTS5 queries and
background synchronization. The Vue explorer exposes progress, legacy mode,
explicit reconciliation, and complete-token search guidance.

The approved collection-boundary change is implemented: all keywords for a
provider share a starting watermark, which advances to run-start only after
every search/check/write succeeds. Conditional duplicates suppress notification.
DynamoDB new-result saves atomically commit the result, counter, and journal
reference; fixtures cover contention, throttling, and a lost commit response.

## Verification (2026-10-25)

- `go test ./...` passed.
- `CGO_ENABLED=0 go test ./localindex ./archivesource ./archivesync -v` passed.
- Focused race checks passed for `localindex`, `archivesource`, `archivesync`,
  `storage`, `bot`, and `webserver`.
- The representative race test exposed competing local publication/lease-renewal
  writes. Local writer operations are now serialized, and a publishing
  transaction renews its fenced lease before releasing SQLite's writer lock.
  Readers remain concurrent. The representative test passes with this fix.
- Abrupt subprocess termination inside a transaction preserves only the committed
  staging rows and cursor. Recovery tests cover scan and catch-up restart,
  incremental committed-prefix recovery, journal activation, and failed
  reconciliation retaining cached data.
- HTTP tests exercise cached token search with unavailable credentials, stale
  status, generation conflicts, and reconciliation cooldown.
- Frontend build/type checks and unit tests passed; all 14 desktop/mobile
  Playwright tests passed, including cached results during synchronization.
- CGO-disabled full CLI builds passed for darwin/linux/windows on amd64/arm64.
  Execution was verified only on darwin/arm64. Cross-compilation does not claim
  execution on the other targets.
- A CGO-disabled release-binary smoke test served the embedded production UI,
  imported a read-only SQLite fixture, and served FTS results after restart with
  the source removed. No Node or SQLite executable was needed by the server.
- `make lambda-package` passed. Dependency inspection excludes the local-index,
  sync, frontend, and modernc SQLite packages from the standalone Lambda.
  CI now checks that boundary.
- `openspec validate add-local-sqlite-search --strict` and `git diff --check`
  passed. Baseline integration steps are in `baseline-integration.md`.

## Representative fixture observations

On the local darwin/arm64 host with CGO disabled, 25,000 synthetic records took
2.38 seconds to import into the local index. Reopen plus an FTS search and all
analytics took 146 ms, returned 25 paginated records, and allocated approximately
49 KB on the Go heap. This is incremental Go allocation, not process RSS or
SQLite's total memory footprint. The fixture uses in-process source pages and
does not estimate network/bootstrap time on a provisioned DynamoDB table.

Separate fixtures verify that a cached query succeeds while source initialization
is blocked, no-op synchronization preserves generation, and incremental replay
does not call Scan. DynamoDB request fixtures check consistent reads, ordered
events, bounded reference batches, unprocessed-key retry, and cancellation.

## Deployment scope

No live collector deployment, production journal writes, or production IAM/table
changes were performed for this change. Earlier live archive-scan verification
belongs to the explorer baseline, not the new journal path. Deploy the updated
collector and reconcile any legacy local baseline as described in
`docs/archive.md`. `.build/grass` contains the updated serving implementation.

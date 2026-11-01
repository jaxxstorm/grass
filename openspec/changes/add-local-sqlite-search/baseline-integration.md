# Specification integration order

Archive `add-archive-explorer` first, then integrate this change. Neither change
has been archived by this implementation. Before archiving local search, update
the newly established baseline requirements below so the combined main specs
do not promise superseded behavior.

| Baseline capability / requirement | Integration required |
| --- | --- |
| `archive-query-api` / Read-only archive snapshots | Preserve source read-only access and operational-item exclusion. Specify full Scan for bootstrap/reconciliation, journal-only normal DynamoDB sync, and durable local generations instead of whole-archive in-memory snapshots. |
| `archive-query-api` / Bounded loading and explicit freshness | Preserve resource defaults, cooldown, cancellation, and first-import failure behavior. Replace manual-only refresh with five-minute background sync and zero-interval opt-out. Distinguish staged bootstrap/reconciliation from coherent incremental committed prefixes. Extend freshness with last successful sync and checkpoint/target. |
| `archive-query-api` / Shared search and filter semantics | Replace the baseline implementation's substring interpretation with literal Unicode token AND search, including complete-word matching, diacritic preservation, and punctuation-only no-match behavior. Preserve category filters and time bounds. |
| `archive-query-api` / Stable pagination and snapshot identity | Make snapshot IDs durable local generations. Preserve ordering, page sizes, and 409 conflicts; clarify that no-op sync does not replace the generation. |
| `archive-query-api` / Consistent archive analytics | Retain full-filter-set statistics and coverage; require SQL-backed execution and bounded daily chart expansion. |
| `archive-web-server` / Predictable server lifecycle and routing | Source SQLite remains read-only; the separate local index is writable. Bind/serve cached data before remote initialization and report remote failures through sync status. |
| `archive-web-server` / Build and operation documentation | Serve-mode SQLite/FTS works with CGO disabled; distinguish the existing CGO-dependent SQLite collector. Keep the standalone Lambda dependency boundary. |
| `archive-explorer-ui` / Loading, error, empty, and stale states | Preserve cached results during sync, distinguish local data time from last successful remote sync, expose legacy status and explicit full reconciliation. Retain URL filters, accessibility, and conflict recovery. |

The record-metadata capability remains additive and compatible. The four new
capabilities were reviewed against their implementations:

- `local-search-index`: `localindex/` schema, transaction, query, crash, and
  generation tests.
- `archive-change-journal`: `storage/journal.go`, journal fixtures, and grouped
  provider watermark/duplicate-notification tests in `bot/`.
- `local-archive-sync`: streaming sources and sync coordinator tests for restart,
  catch-up, no-Scan incremental operation, legacy activation, and reconciliation.
- `local-search-serving`: server CLI/API composition, offline HTTP and executable
  smoke checks, Vue/browser tests, release builds, and operations documentation.

These notes are an archival integration checklist, not a claim that contradictory
baseline requirements have already been merged into `openspec/specs/`.

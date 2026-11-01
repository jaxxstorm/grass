## Context

`grass serve` currently scans the selected source into an immutable Go slice and evaluates every query and aggregate in memory. The live DynamoDB archive contains approximately 25,000 results and takes about 4.5 minutes to load at the configured 2 RCU/second. Pacing solved throttling on the 5-RCU table but did not solve cold starts or repeated scans.

The existing DynamoDB key is `(Platform, SortKey)` where a result's sort key is its URL. `TimestampIndex` has `Timestamp` as its HASH key, not a range key. Existing timestamps mix publication and collection time, so neither the index nor the field is a reliable incremental ingestion cursor. Collector results are insert-only under the current duplicate-suppression policy. SQLite collection sources have legacy schemas and must remain readable without serve-mode migration.

This change depends on the implemented `add-archive-explorer` baseline. Its main specs have not yet been archived; implementation and eventual spec integration must account for that ordering.

## Goals / Non-Goals

**Goals:** persistent local search; immediate cached startup; SQLite/FTS5-powered queries and aggregates; bounded, resumable synchronization; reliable incremental DynamoDB reads independent of source timestamps; existing explorer/API compatibility except explicitly documented token-search behavior.

**Non-Goals:** replacing Lambda's primary DynamoDB storage, adding PostgreSQL/OpenSearch, live provider searches, multi-user authorization, editing collector settings, or providing a general-purpose bidirectional replication/deletion service. This version journals newly inserted collector results. Out-of-band writes, modifications, and deletions require explicit reconciliation.

## Decisions

### 1. Dedicated pure-Go SQLite search package

Use `modernc.org/sqlite` in a new local-index package imported by serve mode, not the Lambda composition root. Verify `CREATE VIRTUAL TABLE ... USING fts5` on the pinned version and supported release targets before building the implementation around it. Register/use its distinct `sqlite` driver name; leave the existing `sqlite3` collector driver unchanged. This preserves CGO-disabled DynamoDB CLI releases while avoiding an unrelated collector migration. Local SQLite source reads for serving can use the pure-Go driver in read-only mode.

Alternative: enable FTS5 in `go-sqlite3` with build tags and cross-compilers. Rejected for this index because current releases use `CGO_ENABLED=0`. Alternative: persist JSON snapshots. Rejected because it would retain whole-archive memory loading and per-request scans.

Default index directory is `os.UserCacheDir()/grass/archive/`, with `--archive-index` accepting an explicit file path. A deterministic filename hashes the configured source locator: database kind, configured AWS profile/region/table and custom endpoint when applicable, or canonical SQLite source path. Persist the locator and verified remote journal epoch in the database. Report the selected index path in startup logs. Credentials are never persisted. An explicit index path bound to a different locator is rejected; a remote epoch mismatch stops incremental sync and requires explicit reconciliation. Previously cached data remains available, clearly stale, while credentials/remote identity are unavailable.

Use WAL, foreign keys, busy timeout, parameterized SQL, and transactional schema migrations. One sync writer owns an index through a renewable, fenced database lease; every publishing/checkpoint transaction verifies its ownership token so an expired owner cannot commit after takeover. A second process must not concurrently advance its checkpoint. Read transactions can proceed concurrently. An index path equal to the source SQLite path is rejected. Corruption or a newer unsupported schema produces an actionable error and does not trigger destructive automatic recreation.

### 2. SQL data model and query contract

Store normalized result fields in `records`, unique on `(platform,url)`, with stable public result IDs and scoped author identities. Index recorded timestamp/platform/keyword/author as appropriate. Maintain an external-content FTS5 index covering title, content, author handle, and author label with transactional insert/update/delete triggers. Store distinct cloud tokens in `record_terms(record_id,term)` with unique pairs and a term index. The existing Unicode cloud tokenizer and `en-v1` stopword rules remain authoritative for word clouds; FTS tokenization does not redefine cloud statistics.

Metadata includes schema/tokenizer versions, source locator, journal epoch/sequence, local generation ID, last successful sync time, and bootstrap state. Stage initial/reconciliation data and scan continuation keys in separate tables, not in a full Go slice. Page-sized buffers are permitted. Existing record/byte/examined-item limits bound each bootstrap or reconciliation; incremental sync bounds each batch and resulting live index size. Limits describe decoded indexed data rather than SQLite/WAL filesystem size; disk-full failures are handled transactionally.

`q` becomes case-insensitive literal token AND search, using FTS5 `unicode61` with diacritic preservation. Tokenize user text as Unicode letters/digits; quote each token and bind the generated MATCH expression. Do not expose raw MATCH operators, wildcard/prefix syntax, or user SQL. A nonempty input with no eligible tokens returns zero matches. The existing 256-character input limit remains. This intentionally replaces arbitrary substring matching: `network` matches a token `network`, not `networking`. Add UI help and migration documentation; empty `q` matches all rows.

Preserve existing category AND/value OR filters, opaque author IDs, cloud-term exact filtering, UTC inclusive-from/exclusive-to bounds, timestamp-desc/platform-asc/URL-asc ordering, page sizes, metadata coverage, top-20 author ranking, top-50 terms, and keyword semantics. SQL computes counts and aggregates over the full matched set before pagination. Word counts join `record_terms`; source records are never materialized wholesale for analytics. Construct missing daily buckets from grouped SQL results and bound expansion by the supported recorded-date domain/query limits, reporting an actionable error rather than allocating unbounded output for malformed timestamps.

Every `/api/archive` response runs in one read transaction. It reads the committed generation ID, applies the `snapshot` precondition, and obtains the page plus all aggregates from that same generation. A conflicting generation still returns `409 snapshot_changed`. The existing `Snapshot` field is now a durable local-generation identifier; `LoadedAt` is its commit time. A no-op sync updates sync freshness without changing generation, avoiding unnecessary pagination resets.

### 3. Ordered DynamoDB journal in the existing table

New result saves atomically transact three writes:

1. Conditional insertion of the existing result item, preserving its key and optional metadata.
2. Conditional update of a reserved journal control item with a monotonically increasing integer sequence and persistent randomly generated epoch.
3. Insertion of an immutable journal event referencing the result's platform and URL at that sequence.

Use partition `__grass_archive_journal__`, control sort key `state`, and event sort keys `event#` plus a fixed-width zero-padded sequence. The control item records a journal format version and epoch. The writer reads the counter strongly consistently, then uses compare-and-swap in the transaction. Competing writers retry with bounded jitter; SDK retries reuse an idempotency token for the same transaction attempt. Failed transactions consume no logical sequence. Unknown transaction outcomes are resolved by checking the result and journal state, never by blindly allocating an extra event. Sequence overflow or an unsupported journal version fails explicitly.

Events contain references and a unique write-attempt identity, not full post bodies, reducing duplicated storage and avoiding event payloads approaching DynamoDB's item limit. Persist the matching identity/epoch/sequence on the inserted result so an uncertain response can distinguish that writer's successful transaction from a concurrent duplicate. Synchronization fetches referenced records using strongly consistent BatchGetItem requests, retrying unprocessed keys with bounded backoff. Missing referenced records are an explicit inconsistency, not a silently skipped cursor position.

Distinguish a conditional duplicate insert from a new successful save through a storage sentinel error handled by the bot to suppress duplicate notifications. Keep the public `Storer` method signatures. A failed result/journal transaction must not advance that provider's collection watermark; a later collection can retry, and already saved records remain deduplicated. Preserve normal notification ordering after successful persistence.

Collection groups all configured keywords into one provider run. Read its watermark once, search each keyword against that same watermark, and advance only if every keyword search, existence check, and new-result write succeeds. A quota-skipped keyword leaves the watermark unchanged. Advance to the captured provider-run start time rather than completion time. `Bot.Run(keyword)` remains a single-keyword wrapper; application composition calls `RunKeywords` once with all keywords. Existing persisted watermark keys and storage interfaces remain unchanged.

No TTL/automatic journal compaction is introduced: an offline client can replay from any persisted sequence. Journal retention/compaction is a later design requiring a retention floor and forced-rebootstrap protocol. Cursor/lease/budget/journal items never enter archive results.

Alternatives: ingestion-time GSI with overlapping lookback windows adds an index and can miss delayed writes/index propagation beyond the overlap; DynamoDB Streams has limited retention unsuitable for arbitrarily offline local clients. The ordered transactional journal costs additional reads/writes and serializes counter allocation, acceptable for this personal collector's low write rate in exchange for a precise durable cursor. It requires no new index or table. IAM is expressed using the item-level actions actually authorized by DynamoDB transactions (`GetItem`, `PutItem`, `UpdateItem` here), not a guessed `TransactWriteItems` IAM action.

### 4. Bootstrap and incremental synchronization

Open and serve an existing committed index before constructing remote clients or retrieving AWS credentials. Start a background worker afterward. Default `--archive-sync-interval=5m`; `0` disables periodic sync but not explicit refresh or first bootstrap. Coalesce sync triggers and retain the current minimum one-minute start cooldown. Failed syncs retain the last committed generation and checkpoint with clear error/retry state. All network activity is inside source integration adapters.

For a journal-capable DynamoDB source:

1. Read the control item's epoch and high-water sequence `S0` strongly consistently. Persist them with bootstrap staging state.
2. Perform one capacity-paced, strongly consistent paginated full scan into staging, excluding metadata. Strong reads ensure previously committed pre-`S0` results are not omitted by eventual consistency; this increases initial scan capacity cost relative to the previous eventual-read snapshot loader. Commit each staged page and its continuation key atomically; restart resumes the same bootstrap.
3. Read a fresh high-water `S1`, then strongly consistently Query the journal for `(S0,S1]`, paginate in key order, and fetch referenced result records. Upsert them into staging. This closes races where records were inserted during the scan; duplicate scan/journal observations are idempotent.
4. Verify the epoch is unchanged, validate bounds, then atomically publish staged records/FTS/terms and checkpoint `S1` as the first complete local generation. Writes beyond `S1` are handled by the next incremental sync. A DynamoDB scan is not a historical point-in-time snapshot; observed newer rows are harmless upserts on later replay.

For subsequent syncs, capture target sequence `T` and Query only `(checkpoint,T]`. Each bounded batch transaction upserts records, refreshes FTS/terms, advances the contiguous journal checkpoint, and updates the generation only when visible data changes. A sync interrupted after a batch exposes a coherent committed prefix, with sync status still incomplete; it does not claim to have reached `T`. Verify expected sequence continuity and epoch before advancing. Network calls, pacing waits, and SQL transactions observe cancellation. Pace Scan, Query, and BatchGet using consumed capacity against one shared 2-RCU/second budget; handle SDK retries and unprocessed keys without resetting the budget or checkpoint.

The first run against an older collector with no journal may complete a paced bootstrap and serve it in explicit `legacy` mode. Normal refresh returns an actionable journal-unavailable status without silently scanning again. `POST /api/archive/reconcile` is the explicit full-scan fallback. Deploy the journal-capable collector, then reconcile once before switching to incremental mode: otherwise old-writer inserts between bootstrap and activation could be missed. Mixed old/new writers and direct table edits are outside the incremental contract and require reconciliation. An epoch change, missing sequence, or missing referenced item stops replay and requests reconciliation rather than guessing.

Reconciliation stages a replacement and uses the same scan-plus-journal catch-up algorithm, keeping the existing generation queryable until publication. A failed reconciliation leaves existing rows and checkpoints intact. Successful replacement removes locally cached rows absent from the reconciled source; concurrent out-of-band deletes are not given stronger guarantees than a DynamoDB scan.

For a SQLite source, stream a read-only source transaction into staging on initial import and subsequent requested/periodic syncs. Local full reads do not incur DynamoDB costs and tolerate legacy schemas. Atomic replacement supplies equivalent user-visible generations; compare staged data to avoid changing the generation on no-op imports. This version does not require collector-side SQLite triggers or alter the source schema.

### 5. Server and UI integration

The primary user-facing command is `grass analyse --destination sqlite serve`.
Analysis sources collected results from DynamoDB and materializes them into the
SQLite destination (the default). Options are accepted before or after `serve`.
The existing `grass serve --db=...` entry point remains a compatibility path and
supports existing SQLite collection sources; the analysis command does not use
`--db` to select its destination.

Keep `GET /api/status`, `GET /api/archive`, and `POST /api/archive/refresh`; refresh now requests synchronization instead of replacing an in-memory slice. Add `POST /api/archive/reconcile` for explicitly requested full reconciliation. Both share writer coalescing/cooldown controls. A reconciliation requested during an incremental sync must be queued or rejected explicitly, never silently treated as completed.

Add status fields for local readiness/generation/count, sync mode (`journal`, `legacy`, or `sqlite`), phase, examined/imported counts, last successful sync, target/checkpoint sequence, and retry time. Retain existing fields during frontend migration. Freshness distinguishes local data availability from remote synchronization success. Before the first completed import, queries return a loading/unavailable response; after a successful import they work even when the source is offline.

Update the Vue UI to show cached results immediately, background-sync progress without clearing results, actionable legacy/journal errors, and the explicit reconciliation action with its full-read cost described. Preserve filter URLs, linked charts, snapshot conflict recovery, and light/dark/mobile layouts. Add token-search help rather than implying substring behavior.

Existing `--db`/`--table-name` and source environment variables keep their meaning. Add `--archive-index` and `--archive-sync-interval`; existing archive resource/read-capacity flags apply to synchronization. No new environment variable is required. Shutdown cancels synchronization and closes local SQL connections after bounded HTTP draining.

## Risks / Trade-offs

- [Transactional journaling increases write cost and contention on a 5-WCU table] → Small reference events, bounded retries, realistic capacity fixtures, operator-visible failure logs, and explicit permission/capacity documentation; do not silently drop journaling when throttled.
- [Older writers bypass the journal] → Document rollout order, expose legacy mode, and require reconciliation during activation or after out-of-band changes.
- [Local index/journal grows] → Retain configurable index bounds, report disk-full errors, and document journal storage cost; no destructive automatic cleanup.
- [FTS5/build compatibility differs from current SQLite integration] → First implementation task proves FTS5 on the pure-Go driver, tests existing collector coexistence, and checks release targets with CGO disabled.
- [Search semantics change] → Explicit literal token AND contract, UI guidance, and regression cases covering punctuation, Unicode, and operator-like input.
- [Partial imports or crashes expose inconsistent data] → Persist staging separately, commit row/index/checkpoint changes atomically, and query one generation per read transaction.
- [Existing baseline specs overlap new behavior] → Archive the explorer first and reconcile its snapshot/search lifecycle requirements with these replacement capabilities before final archival; do not silently retain contradictory substring or startup-scan promises.

## Migration Plan

1. Prove driver/FTS5/release support and introduce versioned local schema and SQL query tests.
2. Deploy collector journal writes and required IAM before enabling journal-based synchronization. Existing records do not need rewriting; the first full import supplies history.
3. Deploy the new server, importing once into its selected local path. Existing browser links remain valid; text matching uses the documented token contract.
4. For a server bootstrapped before collector rollout, run explicit reconciliation to establish a complete journal baseline. Thereafter resume from the durable checkpoint.
5. Verify restart with the source unavailable, append-only incremental sync without Scan, and writer/sync failure recovery on fixtures before controlled live testing.
6. Rollback: the old server can ignore reserved journal items; stopping the new server leaves the source untouched. Rolling the collector back disables reliable journal completeness; mark/reconcile local indexes before resuming incremental mode. Do not delete journal/control items as part of rollback.

## Open Questions

No product decision blocks artifact completion. Implementation must establish the pinned pure-Go driver's FTS5 and cross-platform compatibility and measure transaction throughput on a representative provisioned-capacity fixture. If those checks contradict the chosen architecture, revise this design before proceeding. Journal compaction and change capture for out-of-band updates/deletions are future capabilities.

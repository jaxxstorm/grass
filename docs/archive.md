# Archive explorer

Grass can serve an interactive Vue archive explorer from its Go executable:

```sh
make build
.build/grass analyse --destination sqlite serve --table-name=grass
# Local SQLite: reads grass.db without modifying it
.build/grass serve --db=sqlite --table-name=grass
```

Open **http://127.0.0.1:8080**. Search collected text, filter by platform,
stored keyword and recorded time, and click keywords, cloud terms, or authors
to narrow every panel together. Filter URLs can be bookmarked or shared.
Result details link to the original source. The interface supports mobile,
keyboard navigation, and light/dark themes.

`analyse` reads collected results from DynamoDB and materializes them in a local
SQLite search database. `--destination sqlite` selects the analysis destination,
not the collection store; SQLite is the default destination. `serve` starts the
explorer and background synchronization. The initial import reads the complete
archive within the configured resource limits; later starts use the local cache
and synchronize new journal entries. Options may appear before or after `serve`.
For example, `grass analyse serve` uses the default table and local index path.
The earlier `grass serve --db=...` entry point remains available for compatibility
and for reading an existing SQLite collection database.

## Database access and collection

Serve reads the selected archive; it needs no search-provider or notifier
credentials. It does not invoke collection or edit collector configuration.
Existing flag-only collection and the scheduled Lambda continue independently.
The usual `.env` loading and `SOCIAL_SEARCH_TABLE_NAME` default still apply.

For DynamoDB, use the normal AWS SDK credential chain (`AWS_PROFILE`, IAM role,
or AWS environment credentials) and configure the region (`AWS_REGION`). A
separate read-only server identity needs `Scan`, `Query`, `GetItem`, and
`BatchGetItem` on the archive table. For example, substitute your region/account/table:

```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": ["dynamodb:Scan", "dynamodb:Query", "dynamodb:GetItem", "dynamodb:BatchGetItem"],
    "Resource": "arn:aws:dynamodb:REGION:ACCOUNT:table/TABLE"
  }]
}
```

SQLite serving opens the existing `<table-name>.db` read-only. It neither creates
missing databases nor migrates old schemas. Collector initialization adds the
optional content/author columns idempotently. DynamoDB fields are additive.

## Persistent local search and synchronization

The server stores a durable SQLite/FTS5 index under your operating system's user
cache directory, in `grass/archive/`. Its filename is derived from the configured
source locator (including the AWS profile/region overrides or SQLite path).
Use `--archive-index=/path/to/archive.db` to choose a stable explicit location.
An index belongs to one source; incompatible source/schema reuse is rejected.
The index must be separate from a SQLite collection database.

Subsequent starts serve the committed local index immediately, even if AWS is
unavailable. Search, pagination, coverage, and analytics use SQL transactions;
they do not reconstruct the archive in Go memory. Text search matches complete
Unicode tokens with AND semantics: `network` does not match `networking`.
FTS operators and SQL are not accepted as executable query syntax. Chart date
ranges are bounded to 100 years; narrow the time filters for unusual records.

The first import streams a strongly consistent, capacity-paced DynamoDB scan
into staging, then catches up through the collector's ordered change journal.
Strong reads cost more than the earlier eventual-read snapshot loader; a large
first import may take several minutes. Interrupted imports retain their cursor
and staging data for the next retry. DynamoDB Scan still does not provide a
historical point-in-time snapshot across pages.

Once a journal baseline exists, normal sync queries only new journal entries and
fetches their referenced records. **Refresh archive** requests sync; background
sync defaults to every five minutes. `--archive-sync-interval=0` disables periodic
sync. Concurrent operations coalesce, with at most one new start per minute.
Each committed incremental batch is a coherent local generation. Failures retain
the last committed rows and checkpoint; an incomplete sync is not reported as
fully synchronized. No-op syncs keep pagination valid.

**Synchronization options → Reconcile archive** explicitly rebuilds from a full
source read, retaining cached results until replacement succeeds. Use it after
out-of-band source modifications/deletions, a journal identity change, or upgrading
an older collector. Existing SQLite sources use read-only transactional local
imports on each sync and never require source-schema changes.

### Collector rollout

Deploy the updated collector before the initial journal-backed import. New result
saves atomically write the result, an ordered reference event, and the shared
journal counter. Existing result keys and timestamps are unchanged. This adds
one event per new record, transaction read/write costs, and a shared counter;
monitor write capacity on low-provisioned tables. Journal entries have no TTL or
automatic compaction in this version so long-offline clients can catch up.

The existing Terraform/SAM collector policies already authorize the required
`GetItem`, `PutItem`, and `UpdateItem` item operations. Do not add a fictitious
`TransactWriteItems` IAM action. Local readers use the separate read-only policy
above. All configured keywords for a provider share one starting watermark; it
advances to run-start only after all searches and persistence succeed. Failed
or quota-skipped keywords retain it, and saved results remain deduplicated.

An older collector can still be imported once in **legacy** mode. Normal refresh
then explains that incremental updates need the journal; it does not silently
scan again. After deploying the new collector, explicitly reconcile once to
establish a complete baseline. Mixing old/new collectors or editing table rows
directly bypasses the append-only journal contract. Rolling a collector back
requires reconciliation before reliable incremental operation resumes. Do not
delete the journal/control items during rollback.

Defaults and overrides:

| Flag | Default |
| --- | --- |
| `--archive-index` | source-specific file in the user cache directory |
| `--archive-sync-interval` | `5m` (`0` disables periodic sync) |
| `--archive-max-records` | `50000` |
| `--archive-max-scanned` | `100000` |
| `--archive-max-bytes` | `67108864` (64 MiB decoded fields) |
| `--archive-load-timeout` | `10m` |
| `--archive-read-capacity` | `2` DynamoDB read-capacity units/second |

Resource limits must be positive; the sync interval can be zero. A failed or
oversized initial import/reconciliation never publishes its staging data.
The last committed generation stays visible after synchronization failure.
Increase limits explicitly if appropriate for your archive and machine; the
byte limit describes decoded indexed record fields, not SQLite/FTS/WAL file size
or total process memory. Keep sufficient disk space for staging and the WAL.

DynamoDB scans, journal queries, and referenced-record reads share consumed-
capacity pacing against `--archive-read-capacity`. This supports the default
5-RCU provisioned table without an unpaced scan exhausting read capacity.
Initial loads can take several minutes. The server logs the selected index,
sync start, periodic examined/imported counts, completion, and underlying errors.
Increase the read budget only when the table has sufficient spare capacity.
The UI retains the original load error during cooldown and displays when a
retry becomes available. Remote configuration/credential errors are reported
by background synchronization rather than preventing cached startup.

## Interpreting the charts

- Counts describe **collected records**, not platform-wide activity.
- Existing deduplication stores the first matching keyword. Keyword counts do
  not represent every possible keyword match or the currently configured list.
- Time filters use inclusive `from` and exclusive `to` in UTC. The UI calls this
  **recorded timestamp** because historical providers used mixed publication
  and collection-time semantics.
- Old records may have no content or author. Titles still contribute to word
  clouds; coverage indicators distinguish missing data from real activity.
- Future collection preserves available content and explicit account IDs or
  handles. There are no historical re-fetches or additional enrichment calls.
  Authors are scoped by platform; display names alone do not identify authors.
- Cloud terms count once per matching record, even if repeated. Unicode
  letter/digit terms of 3–40 characters are lowercased; URLs, standalone numbers,
  and the built-in `en-v1` English stopword list are excluded.

## Development and releases

Use Node 24+ and the repository's Go version. `make build` installs locked
frontend dependencies, type-checks and builds Vue assets, and embeds them in
`.build/grass`. Production binaries need no Node, npm, CDN, or frontend server.
GoReleaser builds assets before compiling binaries. Plain `go test ./...` works
on a clean checkout using the tracked placeholder; running an unbuilt server
reports the frontend build command.

For live frontend development:

```sh
make build
.build/grass serve --db=sqlite --table-name=grass
# In another terminal; Vite proxies /api to localhost:8080
npm --prefix web run dev
```

Verification:

```sh
go test ./...
npm --prefix web run typecheck
npm --prefix web test
npx --prefix web playwright install chromium
npm --prefix web run test:e2e
```

The local search index and serve-mode SQLite source reader use a pure-Go SQLite
driver with FTS5 and work in CGO-disabled releases. The older one-shot SQLite
collector still uses `go-sqlite3` and needs a CGO-enabled build and C compiler;
use `CGO_ENABLED=1 make build` for that collection mode.
The separate native ARM64 Lambda build (`make lambda-package`) does not import
the web server or require Node/frontend assets.

The default listener is loopback. To host elsewhere, explicitly set
`--listen=0.0.0.0:8080` and supply access control/TLS in your hosting environment
or reverse proxy; Grass has no built-in login. Database credentials stay in
the Go process. No Tailscale integration is required.

The HTTP API uses `GET /api/status`, `GET /api/archive`, and
`POST /api/archive/refresh`, and `POST /api/archive/reconcile`. Status includes
local generation, sync mode/phase, progress, checkpoint/target, and last success.
Query parameters are `q`, repeated `platform` and
`keyword`, opaque `author`, cloud `term`, RFC3339 `from`/`to`, `page`,
`page_size` (1–100), and `snapshot`. A replaced snapshot returns
`409 snapshot_changed`; clients restart pagination. API errors are structured
JSON with `code` and `message` fields.

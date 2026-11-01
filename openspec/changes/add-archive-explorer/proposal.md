## Why

Grass collects useful social search results, but users currently have to inspect notifications or database records to explore them. A searchable visual archive would make it possible to discover keyword trends, recurring topics, and the most active authors in the collected results without issuing more paid or quota-limited provider searches.

## What Changes

- Add `grass serve`: a Go HTTP backend with an embedded, polished Vue frontend that connects to an existing DynamoDB or SQLite archive.
- Provide paginated archive browsing, text search, and platform, stored keyword, author, and time filters with shareable URL state.
- Add linked visualizations for keyword counts, activity over time, word clouds, and most-active authors, computed over the same filtered archive as the result list.
- Preserve optional post content and structured author identity during future collection so analytics can use real data. Existing records remain readable and explicitly report missing content/author information.
- Add a read-only archive access boundary alongside the collector's existing storage interface, with bounded reads and visible snapshot freshness.
- Keep existing one-shot CLI commands and Lambda collection available. `grass serve` runs independently and does not require Tailscale, trigger provider searches, or edit collector configuration.

## Capabilities

### New Capabilities
- `archive-record-metadata`: Persist optional content and structured author information from existing provider responses, with backward-compatible archive reads.
- `archive-query-api`: Read, filter, paginate, and aggregate collected results through a Go HTTP API with defined snapshot and resource-limit behavior.
- `archive-explorer-ui`: Provide a responsive Vue search and analytics experience with linked keyword, word-cloud, author, and activity views.
- `archive-web-server`: Launch and package `grass serve` with embedded assets, database selection, and predictable lifecycle behavior.

### Modified Capabilities

None. There are currently no main specification files under `openspec/specs/`.

## Impact

- CLI and runtime: add a serve command and server flags while preserving flag-only collection and `--version`. Reuse `--db`, `--table-name`, `SOCIAL_SEARCH_TABLE_NAME`, and the AWS SDK credential chain; serve does not require provider or notifier credentials.
- Storage: additive optional result fields in DynamoDB and SQLite, idempotent collector-side SQLite migration, and a separate read-only archive reader. Lease, cursor, and YouTube-budget records must never appear as search results.
- Integrations: populate author/content fields from the six existing searchers' response payloads. No additional provider requests, new credentials, authentication changes, or historical re-fetching. Existing deduplication and notification behavior remain intact.
- Frontend and packaging: add Vue 3, TypeScript, Vite, a consistent visual component system, frontend tests, and a reproducible asset build integrated with Go embedding and release builds.
- Operations: the server needs read access to its selected archive; DynamoDB snapshot reads have capacity cost. The initial bounded snapshot architecture targets personal archives and makes limits and freshness visible rather than presenting truncated analytics as complete.

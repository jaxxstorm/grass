## ADDED Requirements

### Requirement: Read-only archive snapshots
The backend SHALL read existing SQLite or DynamoDB results through a read-only archive boundary, without initializing searchers/notifiers or calling external providers. DynamoDB loading SHALL follow all scan pages until completion or a configured limit. Cursor, lease, quota, and other non-result records SHALL be excluded.

#### Scenario: Read a multipage DynamoDB archive
- **WHEN** results span several scan pages interspersed with operational items
- **THEN** the snapshot contains every valid result within its limits and no operational items

#### Scenario: Serve without integration credentials
- **WHEN** valid database access is available but provider/notifier credentials are absent
- **THEN** archive queries work without triggering external searches or notifications

### Requirement: Bounded loading and explicit freshness
Snapshot loading SHALL default to limits of 50,000 result records, 100,000 examined items, 64 MiB of decoded record data, and 10 minutes. DynamoDB reads SHALL use pages of at most 100 items and pace subsequent requests using reported consumed capacity, defaulting to 2 read-capacity units per second. The server SHALL enforce these limits during loading, publish only complete snapshots, and expose snapshot ID, load time, loading/refresh state, next permitted refresh time, and sanitized errors through `GET /api/status`. Refreshes SHALL be manual after the initial load, coalesced while in progress, and limited to one new load per minute.

#### Scenario: Provisioned table has limited read capacity
- **WHEN** a scan page reports consumed capacity
- **THEN** the loader waits before the next page as required by the configured read budget and responds promptly to cancellation during the wait

#### Scenario: Initial load fails and refresh is rate-limited
- **WHEN** the first archive load fails before a snapshot exists
- **THEN** queries return `503 archive_load_failed`, server logs include the underlying error, and status retains the sanitized failure alongside the next permitted refresh time

#### Scenario: Initial snapshot is loading
- **WHEN** `/api/archive` is requested before an initial load completes
- **THEN** it returns structured `503 archive_loading` and status reports loading progress/state

#### Scenario: Loading exceeds a limit
- **WHEN** a source exceeds a record, examined-item, byte, or duration limit
- **THEN** loading stops with an actionable limit error and no truncated dataset is published

#### Scenario: A refresh fails
- **WHEN** a complete snapshot exists and a later refresh fails
- **THEN** the previous snapshot remains queryable with its original timestamp and an exposed refresh error

#### Scenario: Multiple users refresh together
- **WHEN** refresh requests arrive during an active load or its cooldown
- **THEN** requests in progress are coalesced with a `202` response and new loads during cooldown return `429`, without additional scans

### Requirement: Shared search and filter semantics
`GET /api/archive` SHALL support literal case-insensitive text search across title, plain-text content, and author label; platform, stored-keyword, author, exact cloud-term, and recorded-time filters; and pagination. Text words SHALL be ANDed, repeated values within a filter category SHALL be ORed, and different filter categories SHALL be ANDed. Time bounds SHALL use inclusive `from` and exclusive `to`. Invalid input SHALL return a structured `400` response.

#### Scenario: Combined filters
- **WHEN** a query selects two platforms, one keyword, and a time range
- **THEN** only results on either platform with that stored keyword and a timestamp inside the range match

#### Scenario: Literal text search
- **WHEN** a user searches for terms containing punctuation or SQL/regular-expression syntax
- **THEN** terms are interpreted as literal search text rather than executable expressions

#### Scenario: Invalid request parameters
- **WHEN** text exceeds 256 characters, a timestamp is invalid, the time range is reversed, or pagination exceeds defined bounds
- **THEN** the API returns `400` with an actionable validation error

### Requirement: Stable pagination and snapshot identity
Results SHALL sort by recorded timestamp descending, platform ascending, and URL ascending. The API SHALL default to page 1 with 25 results and permit page sizes of 1–100. It SHALL return total filtered count, snapshot ID, and stable result identifiers. A request naming a replaced snapshot SHALL receive `409 snapshot_changed` rather than silently mixing snapshots.

#### Scenario: Tied timestamps cross a page boundary
- **WHEN** several matching results share a timestamp
- **THEN** repeated pagination over the same snapshot has stable ordering without lost or repeated items

#### Scenario: Refresh replaces the active snapshot
- **WHEN** a subsequent page request references the prior snapshot
- **THEN** the API returns `409 snapshot_changed` so the client can restart pagination

### Requirement: Consistent archive analytics
The archive response SHALL aggregate the entire filtered result set before pagination, providing stored-keyword counts, platform counts, UTC daily activity buckets, top 20 authors, and top 50 cloud terms. All analytics SHALL use the same snapshot and filters as the result list. Author counts SHALL represent archived results, never platform-wide activity; unknown identities SHALL be excluded from rankings and included in coverage counts.

#### Scenario: Aggregates span multiple result pages
- **WHEN** 200 results match but only 25 are returned on the current page
- **THEN** counts and charts are computed from all 200 matches

#### Scenario: Same handle appears on different platforms
- **WHEN** two platforms contain authors with the same handle
- **THEN** rankings maintain separate platform-scoped identities

#### Scenario: All authors are unknown
- **WHEN** a legacy result set has no usable author identities
- **THEN** the ranking is empty with explicit unknown-author coverage, while results and title-based analytics remain usable

#### Scenario: Empty results
- **WHEN** no records match the filters
- **THEN** the API returns an empty result list, zero counts, and empty analytics without an error

### Requirement: Explainable word-cloud calculations
Word clouds SHALL use document frequency over plain-text titles and available content. Tokenization SHALL lowercase Unicode letters/digits, accept terms of 3–40 characters, exclude standalone numbers, URLs, and a versioned English stopword list, and count each token at most once per result. Ties SHALL sort lexically. The cloud-term filter SHALL use identical normalization. The response SHALL expose content coverage.

#### Scenario: Repeated terms and markup
- **WHEN** a result repeats a term in its title and HTML content
- **THEN** the term contributes one document, HTML tags contribute none, and no raw HTML is required by the client

#### Scenario: Filter from a cloud term
- **WHEN** a displayed term is used as the term filter
- **THEN** matching records contain that exact normalized token according to the cloud tokenizer

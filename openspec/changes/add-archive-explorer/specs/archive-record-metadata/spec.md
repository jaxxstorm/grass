## ADDED Requirements

### Requirement: Persist content and structured author metadata
Newly saved search results SHALL retain available content, author ID, author handle, and author display name alongside the existing archive fields in both DynamoDB and SQLite. Missing fields SHALL remain absent or empty, and existing result identity and duplicate-notification behavior SHALL be preserved.

#### Scenario: Round-trip a rich result
- **WHEN** either backend saves and reads a result with content and author fields
- **THEN** those fields and the existing platform, keyword, title, URL, and timestamp are preserved

#### Scenario: Previously saved result is encountered again
- **WHEN** a provider returns an already-stored result
- **THEN** the existing duplicate check suppresses notifications and no historical enrichment request is issued

### Requirement: Populate metadata from existing provider responses
Each supported searcher SHALL populate author and content fields from its existing search response when available, without additional provider calls. Identity SHALL be platform-scoped and use explicit source IDs or handles; display names and result titles SHALL NOT be used as inferred identities. Deleted Reddit authors SHALL be treated as unknown; Fediverse identities SHALL distinguish different instances.

#### Scenario: Provider supplies author identity
- **WHEN** a Hacker News, Reddit, Bluesky, Fediverse, YouTube, or X response includes the author fields described in the design
- **THEN** the normalized result exposes the corresponding stable ID or handle and available display name

#### Scenario: Identity is unavailable
- **WHEN** a response contains only a title/display name or a deleted author marker
- **THEN** the result remains readable with unknown author identity rather than a guessed account

### Requirement: Backward-compatible archive evolution
Existing DynamoDB items and SQLite databases SHALL remain readable. SQLite collector initialization SHALL add optional columns idempotently. Archive serving SHALL read legacy schemas without writing migrations or creating missing databases. Existing timestamps and keys SHALL retain their meanings.

#### Scenario: Read an old archive
- **WHEN** an archive has no content or author fields
- **THEN** its results remain browsable and expose missing metadata as unknown

#### Scenario: Repeat SQLite migration
- **WHEN** the collector opens an old SQLite database and later opens it again
- **THEN** required optional columns exist without duplicate-column errors or loss of existing rows

#### Scenario: Serve with read-only SQLite access
- **WHEN** the server opens a legacy database with read-only access
- **THEN** it can read existing fields without issuing schema changes

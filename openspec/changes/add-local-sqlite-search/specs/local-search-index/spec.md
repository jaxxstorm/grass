## ADDED Requirements

### Requirement: Persistent source-bound search index
Serve mode SHALL persist normalized archive records, FTS5 data, cloud terms, source identity, schema version, synchronization checkpoint, and local generation in a dedicated SQLite file. The index SHALL remain usable after process restart without loading all records into Go memory. An index bound to a different configured source or an unsupported schema SHALL fail explicitly without destructive recreation.

#### Scenario: Restart with a populated index
- **WHEN** the server restarts with a previously committed compatible index
- **THEN** it serves that generation without waiting for remote reads or credentials

#### Scenario: Reuse an incompatible file
- **WHEN** an explicit index path belongs to another source or a newer schema
- **THEN** startup reports the incompatibility and preserves the file

### Requirement: Transactional index consistency
Record, FTS5, cloud-term, checkpoint, and generation changes SHALL be committed atomically. Initial imports and reconciliation SHALL use separate staging data until publication. Concurrent readers SHALL observe one committed generation per request. Sync writers SHALL be serialized with a fenced ownership mechanism.

#### Scenario: Process stops during a sync commit
- **WHEN** the process terminates before the transaction commits
- **THEN** restart observes the prior committed records and checkpoint and safely replays the uncommitted work

#### Scenario: A second server uses the same index
- **WHEN** another process attempts synchronization while a valid writer owns the index
- **THEN** it cannot concurrently publish rows or advance the checkpoint

### Requirement: Literal token search with FTS5
Text search SHALL use parameterized FTS5 queries over title, content, author handle, and author label with case-insensitive literal token AND semantics. Unicode letter/digit tokens SHALL preserve diacritics. User input SHALL NOT execute FTS operators, wildcard syntax, or SQL. Empty text SHALL match all records; nonempty text with no searchable tokens SHALL match none. The text length limit SHALL remain 256 characters.

#### Scenario: Search a complete token
- **WHEN** the user searches for `network`
- **THEN** rows containing that token match while rows containing only `networking` do not match

#### Scenario: Operator-like input
- **WHEN** input contains quotes, punctuation, SQL fragments, or FTS operator syntax
- **THEN** it is converted into literal tokens rather than executed as query syntax

### Requirement: SQL-backed filters and analytics
SQL SHALL produce the paginated result set and full-filtered-set analytics using the existing category AND/value OR semantics, UTC time bounds, stable result ordering, scoped author identities, and coverage counts. Cloud terms SHALL use the existing document-frequency tokenizer independently from FTS5. Queries SHALL NOT reconstruct the complete archive as an in-memory slice.

#### Scenario: Matching results span several pages
- **WHEN** only one page of a larger filtered set is returned
- **THEN** keyword, platform, daily activity, author, and term statistics describe the entire matching set

#### Scenario: Historical fields are absent
- **WHEN** legacy records lack authors or content
- **THEN** title search and cloud terms remain available and coverage identifies missing fields without invented identities

### Requirement: Durable generation contract
The existing snapshot precondition SHALL identify the committed local generation. One response SHALL read its generation, page, totals, and analytics within a single SQL read transaction. No-op synchronization SHALL preserve the generation identifier while updating synchronization freshness.

#### Scenario: Data changes between page requests
- **WHEN** a request names a generation replaced by a data-changing commit
- **THEN** the API returns `409 snapshot_changed`

#### Scenario: Synchronization finds no changes
- **WHEN** a successful synchronization produces identical visible records
- **THEN** existing pagination remains valid and last-successful-sync time advances

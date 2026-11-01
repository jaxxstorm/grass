## ADDED Requirements

### Requirement: Atomic ordered result journal
DynamoDB result insertion SHALL atomically create the result, advance a journal counter, and insert a reference event at a unique ordered sequence. The journal SHALL have a persistent epoch and version. Existing result keys and recorded timestamps SHALL retain their semantics. Sequence allocation SHALL NOT depend on wall-clock or publication timestamps.

#### Scenario: New result is saved
- **WHEN** a new result transaction succeeds
- **THEN** the result and its event are both durable and the committed counter includes that event

#### Scenario: Transaction fails
- **WHEN** throttling or another failure prevents a transaction from committing
- **THEN** neither an orphan result nor a counter-only advance is published

### Requirement: Concurrent and uncertain write recovery
Writers SHALL allocate sequences using strongly consistent counter reads and conditional transactions with bounded retries. The same transaction attempt SHALL use a stable idempotency token. Uncertain outcomes SHALL be resolved using persisted write identity. Conditional duplicates SHALL be distinguishable from newly inserted results and SHALL suppress duplicate notification.

#### Scenario: Writers compete for the next sequence
- **WHEN** two writers attempt to allocate the same next sequence
- **THEN** at most one commits at that sequence and the other retries or reports a bounded failure

#### Scenario: Successful write response is lost
- **WHEN** a transaction commits but its response is unavailable
- **THEN** retry/recovery does not create a second event or mistake another writer's insertion for its own

### Requirement: Collection failures remain retryable
A failed result/journal write SHALL prevent advancement of that provider's collection watermark for the affected run. Previously saved records SHALL remain deduplicated on retry. Notifications SHALL occur only after successful new-result persistence.

All configured keywords for a provider SHALL use the same starting watermark. The provider watermark SHALL advance to the captured run-start time only after every keyword search and persistence operation succeeds. A failed existence check or quota-skipped keyword SHALL also retain the starting watermark.

#### Scenario: Later keyword succeeds after an earlier failure
- **WHEN** one keyword fails to persist a result and a later keyword succeeds
- **THEN** both searches use the original provider watermark and the later success does not advance it past the failed result

#### Scenario: One result transaction fails during collection
- **WHEN** other results have been saved but a result/journal transaction fails
- **THEN** the provider watermark is retained so the failed result can be retrieved again and successful records are not re-notified on retry

### Requirement: Durable queryable journal history
Journal events SHALL be queryable by ordered sequence on the existing table's primary key, SHALL contain source result references and write identity, and SHALL have no automatic expiration in this version. Reserved journal/control items SHALL be excluded from archive results. Epoch and sequence inconsistencies SHALL be detectable by readers.

#### Scenario: Client resumes after a long absence
- **WHEN** its journal epoch remains valid and its committed checkpoint predates recent activity
- **THEN** the retained journal permits replay without a timestamp lookback window or historical full scan

#### Scenario: Journal history has a gap
- **WHEN** an expected sequence or referenced result is missing
- **THEN** synchronization reports an inconsistency and does not advance beyond the gap

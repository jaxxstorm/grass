## ADDED Requirements

### Requirement: Resumable complete bootstrap
The first DynamoDB import SHALL stream capacity-paced, strongly consistent scan pages into local staging, excluding operational items. Staged pages and continuation state SHALL commit together. A journal-capable bootstrap SHALL record an initial high-water mark, catch up through a later high-water mark, and publish only after validating the unchanged epoch and configured bounds.

#### Scenario: Results arrive during initial scanning
- **WHEN** journal-capable writers insert records after the bootstrap's initial checkpoint
- **THEN** catch-up includes their journal events through the captured target and duplicate scan observations do not duplicate local records

#### Scenario: Bootstrap is interrupted
- **WHEN** the server restarts before the staged import is published
- **THEN** it resumes from durable scan state and does not expose staging as a complete archive

### Requirement: Incremental checkpointed synchronization
After bootstrap, normal DynamoDB sync SHALL use strongly consistent journal queries and referenced-result reads instead of a full Scan. Each bounded committed batch SHALL atomically update local rows/indexes and the contiguous checkpoint. Interrupted sync SHALL resume at the last committed checkpoint.

#### Scenario: Only a few new records exist
- **WHEN** a journal-ready cached archive synchronizes
- **THEN** it reads events after its checkpoint and referenced records without rescanning historical results

#### Scenario: A later batch fails
- **WHEN** earlier incremental batches committed but a subsequent network request fails
- **THEN** the coherent committed prefix remains queryable, status reports incomplete synchronization, and retry resumes without skipping the failed batch

### Requirement: Bounded background scheduling and pacing
Synchronization SHALL run in the background with a default five-minute interval and an option to disable periodic scheduling. Triggers SHALL coalesce, retain the one-minute start cooldown, and honor cancellation. Scan, Query, and BatchGet reads SHALL share the configured consumed-capacity budget, defaulting to 2 RCU/second. Read limits, index-size bounds, deadlines, and unprocessed-key retries SHALL prevent incomplete work from being presented as fully synchronized.

#### Scenario: Source is slow while users search
- **WHEN** remote sync is waiting for capacity or retrying a bounded request
- **THEN** cached SQL queries continue and progress/error state is visible

#### Scenario: Shutdown during a pacing wait
- **WHEN** server shutdown cancels synchronization
- **THEN** the wait terminates promptly and the durable checkpoint remains valid

### Requirement: Explicit legacy and reconciliation behavior
Sources without a journal SHALL support a first full import in clearly identified legacy mode. Normal refresh SHALL NOT silently repeat a DynamoDB full scan in that mode. Explicit reconciliation SHALL build a replacement while retaining the previous generation, and SHALL be required to establish a journal baseline after legacy import or to recover from journal discontinuity/out-of-band changes.

#### Scenario: Older collector is still deployed
- **WHEN** a legacy-mode archive requests normal refresh
- **THEN** status explains that journal support or explicit full reconciliation is required and no automatic full Scan is issued

#### Scenario: Reconciliation fails
- **WHEN** an explicit reconciliation is interrupted or exceeds limits
- **THEN** the prior published generation and checkpoint remain usable

#### Scenario: Source journal is replaced
- **WHEN** the observed epoch differs from the saved epoch
- **THEN** incremental sync stops with an actionable reconciliation requirement rather than applying unrelated sequence numbers

### Requirement: Read-only SQLite source import
Existing SQLite collection databases SHALL be streamed through read-only transactions into the separate local index, including legacy schemas. The server SHALL NOT migrate or modify the source and SHALL reject an index path equal to the source path. Subsequent local-source synchronization SHALL atomically replace changed indexed data.

#### Scenario: Import a legacy SQLite source
- **WHEN** the source lacks optional content and author columns
- **THEN** its records are indexed with missing-field defaults while the source schema remains unchanged

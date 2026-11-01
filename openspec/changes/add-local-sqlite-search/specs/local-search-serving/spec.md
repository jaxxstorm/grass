## ADDED Requirements

### Requirement: Analysis destination command
`grass analyse --destination sqlite serve` SHALL import collected DynamoDB results into a local SQLite destination and serve the explorer with background synchronization. SQLite SHALL be the default destination. Source table and synchronization options SHALL be accepted before or after `serve`. The existing `grass serve` entry point SHALL remain compatible.

#### Scenario: Analyse the collected archive
- **WHEN** the user runs `grass analyse --destination sqlite serve`
- **THEN** the server uses DynamoDB as the source and local SQLite as the search destination, importing the archive initially and reusing the cached index on subsequent starts

### Requirement: Local-first server startup
`grass serve` SHALL open a compatible committed local index before performing remote initialization. Cached search SHALL remain available if remote configuration, authentication, or connectivity fails. Startup with no complete index SHALL clearly report bootstrap/loading or actionable failure state.

#### Scenario: AWS is unavailable at startup
- **WHEN** a valid populated local index exists but AWS cannot be reached
- **THEN** the UI and queries serve cached data and report synchronization failure independently

### Requirement: Synchronization controls and status
Serve SHALL expose `--archive-index` and `--archive-sync-interval`, preserve existing source selection and synchronization resource controls, and log the index location and sync lifecycle. `POST /api/archive/refresh` SHALL request normal synchronization; `POST /api/archive/reconcile` SHALL explicitly request full reconciliation. Status SHALL distinguish local readiness, sync mode/phase/progress, last success, error, retry time, and checkpoint/target state.

#### Scenario: User requests full reconciliation
- **WHEN** the reconciliation action is selected
- **THEN** the UI describes the full source-read cost, requests reconciliation explicitly, and retains any current local results during the operation

#### Scenario: Sync request arrives during another sync
- **WHEN** an operation is already running
- **THEN** the server coalesces equivalent work and explicitly queues or rejects a different reconciliation request rather than claiming it completed

### Requirement: Explorer compatibility and search guidance
The frontend SHALL retain response-field compatibility, filter URLs, pagination, linked analytics, record details, metadata coverage, and responsive light/dark layouts. It SHALL explain literal-token text search and show background progress without clearing cached results. Sync failures SHALL remain visible during cooldown.

#### Scenario: Background sync fails
- **WHEN** a complete local generation exists and synchronization fails
- **THEN** its result list and analytics remain available with a stale/error indicator and actionable retry state

#### Scenario: User enters a partial word
- **WHEN** token search does not match a longer token
- **THEN** visible search guidance accurately describes complete-token matching rather than promising substring search

### Requirement: Release and operational compatibility
The local index SHALL provide working FTS5 in supported CGO-disabled CLI release builds without requiring an external SQLite executable. The Lambda entry point SHALL remain independent of the local-index package. Documentation SHALL cover source/index paths, token semantics, journal rollout, IAM, extra write/storage costs, sync pacing, legacy mode, reconciliation, and rollback.

#### Scenario: Run a release without CGO or Node
- **WHEN** an embedded CLI release starts with a populated index on a supported platform
- **THEN** local FTS5 search and the Vue UI work without runtime compiler, SQLite CLI, or frontend tooling

#### Scenario: Build the collector Lambda
- **WHEN** the native Lambda build is produced
- **THEN** it includes journal-capable persistence without importing the local search-index or frontend packages

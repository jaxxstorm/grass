## ADDED Requirements

### Requirement: Serve command and collector compatibility
The CLI SHALL support `grass serve` with `--db`, `--table-name`, and `--listen`, defaulting to `127.0.0.1:8080`, while retaining existing flag-only collection, help, version, and Lambda entry points. Serve SHALL honor existing table-name environment configuration and AWS database credentials without requiring platform credentials.

#### Scenario: Start the archive server
- **WHEN** a user runs `grass serve --db=dynamodb --table-name=grass`
- **THEN** a Go HTTP server exposes the UI and archive API on the default loopback address and begins loading the selected archive

#### Scenario: Existing collection invocation
- **WHEN** a user invokes the existing keyword/searcher/bot flags without `serve`
- **THEN** the existing one-shot collection behavior runs rather than starting an HTTP listener

### Requirement: Explicit archive resource limits
Serve SHALL expose positive `--archive-max-records`, `--archive-max-scanned`, `--archive-max-bytes`, `--archive-load-timeout`, and `--archive-read-capacity` options with the defaults defined by the query capability. Invalid limits SHALL fail with a clear startup error. The server SHALL log load start, progress, completion, and underlying errors to its console.

#### Scenario: Configure a larger personal archive
- **WHEN** a user explicitly raises a snapshot limit
- **THEN** the reader applies that limit while retaining the other configured bounds

### Requirement: Embedded production frontend
Released CLI binaries SHALL embed the built Vue assets and serve the frontend and API from the same origin without Node or npm at runtime. Frontend assets SHALL be built reproducibly before CLI release compilation. Lambda builds SHALL remain independent of frontend assets and tooling.

#### Scenario: Run a release binary on a machine without Node
- **WHEN** database access is configured and `grass serve` is launched
- **THEN** the production UI and API function without a development server or external asset CDN

#### Scenario: Build Lambda independently
- **WHEN** the native Lambda build target runs without a frontend build
- **THEN** it still produces its bootstrap artifact without importing the web-serving package

#### Scenario: Run an unbuilt source checkout
- **WHEN** a source-built server contains only the intentional asset placeholder
- **THEN** it reports the required frontend build command rather than serving a broken blank UI

### Requirement: Predictable server lifecycle and routing
The server SHALL return structured JSON errors for API failures, distinguish API routes from SPA navigation, report bind/startup errors, and stop gracefully on termination. Refresh/read operations SHALL support cancellation. Serve SHALL open SQLite read-only and report missing files without creating them.

#### Scenario: Unknown API route
- **WHEN** a client requests an undefined `/api/` path
- **THEN** the server returns a JSON 404 rather than the frontend HTML page

#### Scenario: Termination during a load
- **WHEN** the process receives a termination signal
- **THEN** it stops accepting requests, cancels archive work, and closes server/database resources within a bounded shutdown period

### Requirement: Build and operation documentation
Documentation SHALL cover local development, production asset embedding, database selection, read-only DynamoDB permissions, snapshot cost/limits, legacy-data coverage, and non-loopback hosting. SQLite's existing CGO requirement and the DynamoDB-capable CGO-disabled release path SHALL be stated accurately.

#### Scenario: Operator connects to an existing archive
- **WHEN** the operator follows the documented serve procedure
- **THEN** they can choose SQLite or DynamoDB, identify the applicable build/credential requirements, and understand that browsing does not invoke provider APIs

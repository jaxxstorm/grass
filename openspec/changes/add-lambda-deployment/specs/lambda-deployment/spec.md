## ADDED Requirements

### Requirement: Native Lambda deployment package
The system SHALL provide version-controlled build and deployment assets that package Grass as a Linux `arm64` Go executable named `bootstrap` in a ZIP archive for the AWS Lambda `provided.al2023` runtime. Building and deploying this package SHALL NOT require a Docker image or Docker daemon.

#### Scenario: Native build creates Lambda bootstrap artifact
- **WHEN** an operator runs the documented Lambda build command on a supported Go development environment
- **THEN** the command produces a ZIP deployment artifact containing a Linux `arm64` executable named `bootstrap`

#### Scenario: Deployment uses a non-container runtime
- **WHEN** an operator deploys the provided infrastructure definition
- **THEN** the Lambda function is configured with the `provided.al2023` runtime and ZIP package type rather than an image package type

### Requirement: Scheduled Lambda execution
The infrastructure definition SHALL configure an EventBridge-triggered Lambda function that invokes one Grass run on the configured schedule. The schedule expression and Lambda timeout SHALL be configurable without source changes, and the function SHALL be limited to one concurrent execution.

#### Scenario: Scheduled event invokes Grass
- **WHEN** an EventBridge event matches the configured schedule
- **THEN** Lambda invokes the Grass handler once

#### Scenario: Operator customizes schedule
- **WHEN** an operator supplies a valid schedule expression during deployment
- **THEN** the EventBridge trigger uses that expression for future invocations

#### Scenario: Overlapping invocations are prevented
- **WHEN** an invocation remains in progress when another scheduled event arrives
- **THEN** the deployment configuration permits no more than one concurrent Grass invocation

### Requirement: Lambda runtime configuration
The Lambda handler SHALL read `GRASS_KEYWORDS`, `GRASS_SEARCHERS`, and `GRASS_BOTS` as comma-separated environment-variable lists and SHALL read `SOCIAL_SEARCH_TABLE_NAME` as the DynamoDB state table name. It SHALL reject invocation setup when any required setting is absent or contains no selected values. Existing searcher and notifier credential environment variable names and authentication behavior SHALL remain unchanged.

#### Scenario: Handler runs configured integrations
- **WHEN** the required Lambda configuration and integration credentials are present
- **THEN** the handler initializes the selected searchers and notifiers and runs each configured keyword once

#### Scenario: Missing Lambda configuration is reported
- **WHEN** a required list or `SOCIAL_SEARCH_TABLE_NAME` is absent or empty
- **THEN** the handler reports a configuration error and does not begin a search

#### Scenario: Existing provider credentials are used
- **WHEN** a selected searcher or notifier requires credentials supplied through its existing environment variables
- **THEN** the Lambda execution uses those values without requiring new credential names

### Requirement: Durable DynamoDB state for Lambda
Lambda execution SHALL use the existing DynamoDB storage backend and SHALL NOT use SQLite. The deployment assets SHALL provision or accept a DynamoDB table with keys compatible with Grass's `Platform` partition key and `SortKey` sort key, and SHALL grant the execution role only the DynamoDB actions required to read and write Grass state.

#### Scenario: Lambda persists new results
- **WHEN** a selected searcher returns a result not already present in the configured DynamoDB table
- **THEN** the handler saves the result and notifies each configured notifier using the existing bot flow

#### Scenario: Lambda suppresses a stored duplicate
- **WHEN** a selected searcher returns a result whose platform and URL are already recorded in the configured DynamoDB table
- **THEN** the handler does not save or notify that result again

#### Scenario: Lambda retains search cursors between invocations
- **WHEN** one Lambda invocation completes a platform search and a later invocation begins
- **THEN** the later invocation reads that platform's persisted last-search timestamp from DynamoDB

#### Scenario: Execution role is scoped to state storage
- **WHEN** the infrastructure definition creates the Lambda execution role
- **THEN** its DynamoDB permissions are limited to the configured Grass state table and its required actions

### Requirement: Lambda deployment documentation
The project documentation SHALL describe prerequisites, native build and deployment commands, required non-secret configuration, credential handling guidance, DynamoDB state behavior, schedule selection, monitoring, and rollback for Lambda. Documentation SHALL state that Lambda's ephemeral filesystem makes SQLite unsupported for this deployment.

#### Scenario: Operator deploys without Docker
- **WHEN** an operator follows the documented Lambda deployment procedure
- **THEN** the procedure does not require building or publishing a container image

#### Scenario: Operator configures provider limits
- **WHEN** an operator selects a Lambda schedule
- **THEN** the documentation directs them to choose an interval compatible with the selected providers' API rate limits

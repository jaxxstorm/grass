## 1. Shared Application Runtime

- [x] 1.1 Extract searcher, storage, notifier, and one-shot bot construction from `main.go` into an importable application package while preserving the CLI's existing flags and behavior.
- [x] 1.2 Define validated runtime configuration that supports the current CLI inputs and comma-separated Lambda environment variables for keywords, searchers, bots, and DynamoDB table name.
- [x] 1.3 Add focused tests for configuration parsing, required Lambda settings, supported integration selection, and invalid selections.

## 2. Lambda Entry Point

- [x] 2.1 Add a Lambda executable that registers an EventBridge-compatible handler and invokes the shared one-shot application runner.
- [x] 2.2 Force the Lambda runtime to use DynamoDB, reject missing or empty Lambda configuration before searching, and preserve existing credential environment variable behavior.
- [x] 2.3 Add handler tests that verify a valid scheduled invocation runs the configured runner and invalid configuration returns an error without running searches.

## 3. Native AWS Deployment

- [x] 3.1 Add the AWS Lambda Go runtime dependency and native build targets that cross-compile a Linux arm64 `bootstrap` executable and create the deployment ZIP without Docker.
- [x] 3.2 Add a SAM/CloudFormation template that creates the `provided.al2023` ZIP Lambda function, a configurable EventBridge schedule, one reserved concurrent execution, configurable timeout, and CloudWatch logging.
- [x] 3.3 Add DynamoDB state-table resources with `Platform` and `SortKey` keys and least-privilege execution-role permissions for the function to read and write that table.
- [x] 3.4 Pass non-secret Lambda runtime configuration through template parameters or environment configuration without committing provider or notifier credentials.

## 4. Documentation And Verification

- [x] 4.1 Document Lambda prerequisites, non-Docker native build and deployment commands, required configuration, credential handling, DynamoDB state behavior, schedule/rate-limit guidance, monitoring, rollback, and SQLite limitations.
- [x] 4.2 Run `go test ./...` and validate the rendered infrastructure template and native Lambda packaging workflow.

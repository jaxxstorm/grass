## Context

Grass is a one-shot CLI. `main.go` parses flags, constructs searchers, a storage backend, and notifiers, then invokes the bot for each keyword. GitHub Actions currently supplies scheduling, but scheduled workflows are paused after extended repository inactivity.

Lambda has an ephemeral filesystem, so SQLite cannot retain result de-duplication or last-search timestamps between invocations. The existing DynamoDB storer already persists both under a `(Platform, SortKey)` key model and uses AWS's default credential provider chain, which is compatible with Lambda execution roles.

## Goals / Non-Goals

**Goals:**

- Run the existing search, de-duplication, persistence, and notification flow from a scheduled AWS Lambda invocation.
- Keep the CLI behavior and flag interface available.
- Build a native Go custom-runtime ZIP containing `bootstrap`, with no Docker image or Docker daemon required.
- Provision or describe all AWS resources needed to run safely: Lambda, schedule, IAM permissions, and DynamoDB state.
- Accept Lambda runtime settings and existing provider/notifier credentials from environment variables without embedding secrets in deployment assets.

**Non-Goals:**

- Replacing the CLI, GitHub Actions deployment, existing searchers, or existing notifier integrations.
- Adding an HTTP API, a container-image deployment, secret values, or a new secret-management product.
- Migrating existing SQLite data to DynamoDB automatically.
- Changing external platform rate limits or adding retry behavior beyond the providers' current behavior.

## Decisions

### Share application construction between entry points

Move reusable construction and one-shot execution out of the CLI entry point into an importable application package. The CLI remains a thin adapter that translates its existing flags into that package's configuration. A dedicated Lambda `main` registers an EventBridge-compatible handler, loads the same configuration from environment variables, and invokes the same one-shot runner.

This avoids duplicating provider, storage, and notifier selection. Keeping all provider construction inside the application layer preserves the existing `Searcher`, `Storer`, and `Notifier` boundaries. A separate Lambda-only implementation was rejected because its configuration and supported integrations would drift from the CLI.

### Use a native Go custom runtime ZIP

Build the Lambda executable for `linux` and `arm64` as `bootstrap`, then package it in a ZIP for the `provided.al2023` runtime. The infrastructure template uses a makefile-based build hook or equivalent native build commands so local packaging and deployment do not require Docker.

A container image was rejected because it introduces a Docker dependency contrary to the requested deployment model. A managed Go runtime was rejected because Go Lambda deployments use the custom runtime model.

### Use SAM/CloudFormation for deployment resources

Add a version-controlled AWS SAM/CloudFormation template and deployment instructions. The template defines the function, EventBridge schedule, execution role permissions, and DynamoDB table or accepts a supplied table name. It exposes non-secret parameters for schedule, architecture, function settings, and configuration selectors; credentials are supplied at deploy time through environment variables or referenced AWS-managed configuration rather than committed values.

SAM is selected because it produces CloudFormation resources and supports a native makefile build. Hand-written AWS CLI commands were rejected because they make repeatable schedule, permissions, and rollback management harder. Terraform/CDK are out of scope because they add a new deployment toolchain without a project requirement.

### Lambda configuration is environment-based and DynamoDB-only

The Lambda adapter accepts comma-separated `GRASS_KEYWORDS`, `GRASS_SEARCHERS`, and `GRASS_BOTS` settings. It uses `SOCIAL_SEARCH_TABLE_NAME` for the DynamoDB table, forces the DynamoDB backend, and validates that all required lists and the table name are present before running. Existing provider and notifier environment variables retain their names and authentication behavior.

Environment variables match Lambda's native configuration model and preserve the existing integration constructors. Lambda does not accept the CLI's SQLite selection because `/tmp` is not durable. Parsing a command-line string or retaining flag parsing inside Lambda was rejected because Lambda invocation events are not a stable place for runtime configuration.

### Schedule safely and retain state-based duplicate behavior

The template configures an EventBridge schedule and limits the function to one concurrent execution. The function calls the same bot flow, which checks DynamoDB before saving and notifying and persists a last-search timestamp per platform. As a result, later invocations do not re-notify results already recorded in the configured table.

The schedule expression is parameterized rather than hard-coded so deployments can select an interval appropriate for provider quotas. Multiple concurrent invocations were rejected because the current read-then-write duplicate check is not designed for concurrent notifications.

## Risks / Trade-offs

- [Provider call duration exceeds Lambda's configured timeout] → Document and parameterize the timeout; set an appropriate deployment default and monitor Lambda duration.
- [A schedule runs too frequently for a provider quota] → Make the schedule explicit and document that operators must select an interval compatible with each provider's limits.
- [Credentials stored directly in Lambda environment variables are exposed to configuration readers] → Do not commit credentials; document deployment-time configuration and support secure AWS references where Lambda environment variables are configured.
- [DynamoDB table is missing or IAM is incomplete] → Validate table configuration at initialization and create narrowly scoped table permissions in the template.
- [Existing deployments use SQLite] → Treat the Lambda deployment as a fresh DynamoDB state store and document that historical SQLite state is not migrated.
- [A failed provider search is logged but does not fail the existing bot run] → Preserve current behavior initially; use CloudWatch logs and metrics to identify provider failures rather than introducing unplanned retry semantics.

## Migration Plan

1. Deploy the infrastructure template with a new or selected DynamoDB table, a schedule expression, non-secret runtime settings, and provider/notifier credentials supplied outside source control.
2. Deploy the native ZIP artifact and confirm one manual or scheduled invocation creates state and sends only new results.
3. Monitor CloudWatch logs, Lambda duration, throttles, and provider/API quotas before increasing schedule frequency.
4. Keep the CLI and GitHub Actions deployment available during adoption; their SQLite state does not transfer to Lambda.
5. To roll back, disable or remove the EventBridge schedule and Lambda function. Retain the DynamoDB table unless intentionally deleting the persisted de-duplication history.

## Open Questions

- Should the initial template create a new DynamoDB table by default, require an existing table, or support both options?
- Which EventBridge schedule expression and Lambda timeout should be the documented defaults for the supported providers?
- Should the template use direct Lambda environment variables only, or include optional AWS Secrets Manager dynamic references for credentials?

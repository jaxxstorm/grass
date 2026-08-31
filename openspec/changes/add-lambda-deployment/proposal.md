## Why

Grass currently relies on a GitHub Actions schedule, which GitHub pauses after 90 days of repository inactivity. A scheduled AWS Lambda deployment provides an independently operated runtime while retaining Grass's one-shot search and notification behavior.

## What Changes

- Add a deployable native Go AWS Lambda entry point that runs the configured Grass search once per invocation without requiring a Docker image.
- Add infrastructure and packaging configuration for a scheduled Lambda function, its EventBridge trigger, required DynamoDB state storage, and least-privilege IAM permissions.
- Define Lambda-specific configuration and deployment documentation while preserving the existing CLI invocation and its configuration interface.
- Require the Lambda deployment to use DynamoDB for durable result de-duplication and search cursors; SQLite remains available only to non-Lambda CLI use.

## Capabilities

### New Capabilities
- `lambda-deployment`: Package and operate Grass as a scheduled, native Go AWS Lambda function with durable DynamoDB-backed state.

### Modified Capabilities

None.

## Impact

- Affected code: application composition currently in `main.go`, Lambda handler wiring, dependency definitions, and tests.
- Affected deployment systems: AWS Lambda, EventBridge Scheduler or EventBridge rules, IAM, CloudFormation/SAM-compatible infrastructure, and DynamoDB.
- Affected configuration: existing searcher and notifier credentials remain environment variables; Lambda adds scheduling and deployment parameters but no credentials in source or templates.
- External search and notification APIs retain their current authentication, rate-limit behavior, and duplicate-result handling; persisted DynamoDB state remains the source of truth for duplicates and last-search timestamps.

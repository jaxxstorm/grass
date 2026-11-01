# Terraform deployment

This deployment uses account `780219548054`, region `us-west-2`, and the existing
Terraform-managed `grass` DynamoDB table. The Lambda runs a native ARM64 ZIP.
Overlapping Lambda runs are prevented with a conditional DynamoDB lease in the
same table, rather than reserved concurrency. Crashed runs expire one minute
after their Lambda deadline; lock contention skips the invocation successfully.

From the repository root:

```sh
make lambda-package
cd deploy
terraform init
terraform plan -out=lambda.tfplan
terraform apply lambda.tfplan
```

The five-minute schedule is disabled by default while credentials are being
regenerated. Create an ignored `deploy/credentials.auto.tfvars` file with
`integration_environment` set to a map of the required environment variables:

- `DISCORD_BOT_TOKEN`, `DISCORD_CHANNEL_ID`
- `REDDIT_CLIENT_ID`, `REDDIT_CLIENT_SECRET`, `REDDIT_REFRESH_TOKEN`
- `BSKY_USERNAME`, `BSKY_PASSWORD`
- `YOUTUBE_API_KEY`
- `HACHYDERM_IO_ACCESS_TOKEN` (or `HACHYDERM_IO_CLIENT_ID` and `HACHYDERM_IO_CLIENT_SECRET`)

Terraform supplies the table name and `FEDIVERSE_INSTANCES` automatically.
Slack credentials are unnecessary because this deployment uses print and Discord.
Secret values are marked sensitive in output but are stored in Terraform state;
keep the local state and credential file private.

Apply credentials first, then set `schedule_enabled = true` in that file and
review/apply another plan to start scheduled runs. Keep the credential file for
subsequent applies so Terraform continues to configure the complete environment.
Set `schedule_enabled = false` to pause runs without deleting the existing table.

Logs: `aws logs tail /aws/lambda/grass --region us-west-2 --follow`.

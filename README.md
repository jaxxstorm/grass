# Grass

> They're such a [grass](https://www.urbandictionary.com/define.php?term=Grass)!

Grass is a bot that searches various platforms (Hacker News, Reddit, Bluesky, Fediverse, YouTube, and X) for posts containing specified keywords. It then saves results to a pluggable database and can notify via Discord or print the results to standard output.

## Features

- Search for specific keywords across multiple platforms (e.g., Hacker News, Reddit, Bluesky, X)
- Store results in DynamoDB or SQlite
- Notify via Discord or stdout
- Supports running as a one-shot job, making it easy to run locally or via CI/CD pipelines (e.g., GitHub Actions)

---

## Prerequisites

- **Go**: Make sure Go is installed. [Download Go here](https://golang.org/dl/).
- **Discord Bot**: Set up a bot in Discord for notifications.
- **AWS Credentials** (if using DynamoDB): Required to connect to AWS DynamoDB. Use IAM with permissions to read and write to your table.
- **SQLite** (optional): Install SQLite if you prefer local testing.

## 1. Installing the Bot into Discord

To receive notifications in Discord, you need to create a bot in the Discord Developer Portal and invite it to your server.

### Steps to Set Up the Discord Bot

1. **Create a New Discord Application**:
   - Go to the [Discord Developer Portal](https://discord.com/developers/applications).
   - Click **New Application** and name your application.

2. **Add a Bot to the Application**:
   - In your application's settings, go to **Bot**.
   - Click **Add Bot** and confirm.

3. **Set Up Bot Permissions**:
   - Under **OAuth2** > **URL Generator**:
     - Select the **bot** scope.
     - Grant permissions for **Send Messages**, **Embed Links**, and any other required permissions.

4. **Invite the Bot to Your Server**:
   - Copy the generated OAuth2 URL and open it in your browser.
   - Select the server where you want the bot to be added and authorize it.

5. **Set Up Environment Variables**:
   - Copy your bot token and save it in a `.env` file as `DISCORD_BOT_TOKEN`.

6. **Get the Channel ID**:
   - Enable Developer Mode in Discord under **User Settings** > **Advanced**.
   - Right-click the channel where the bot will post and select **Copy ID**.
   - Add this ID to your `.env` file as `DISCORD_CHANNEL_ID`.

## 2. Obtaining API Credentials for Searchers

Each searcher requires its own set of credentials, detailed below:

### Reddit API Credentials

1. **Create a Reddit Application**:
   - Go to [Reddit Apps](https://www.reddit.com/prefs/apps).
   - Click **Create App** and select **Script** as the app type.
   - Note down the **Client ID** and **Client Secret**.

2. **Set Up Environment Variables**:
   - In your `.env` file, add:
     ```env
     REDDIT_CLIENT_ID=<Your Client ID>
     REDDIT_CLIENT_SECRET=<Your Client Secret>
     REDDIT_USERNAME=<Your Reddit Username>
     REDDIT_PASSWORD=<Your Reddit Password>
     ```

### Bluesky API Credentials

1. **Obtain Your Bluesky Handle and App Password**:
   - Create an app-specific password in your Bluesky account settings.
   - Add these values to your `.env` file:
     ```env
     BSKY_USERNAME=<Your Bluesky Handle>
      BSKY_PASSWORD=<Your App Password>
      ```

### X API Credentials

1. Create an approved App in the [X Developer Console](https://console.x.com) and generate a Bearer Token.
2. Add the token to your `.env` file:
   ```env
   X_BEARER_TOKEN=<Your X API Bearer Token>
   ```

The `x` searcher uses X's official recent-search API. It searches posts from the last seven days and is billed by X per post returned. X query operators can be passed directly as a keyword, for example `golang lang:en -is:retweet`.

### Optional: AWS Credentials for DynamoDB

If you’re using DynamoDB, set up your AWS credentials in `~/.aws/credentials` or configure environment variables as follows:

```env
AWS_ACCESS_KEY_ID=<Your AWS Access Key>
AWS_SECRET_ACCESS_KEY=<Your AWS Secret Key>
AWS_REGION=<Your AWS Region>
SOCIAL_SEARCH_TABLE_NAME=<Your DynamoDB Table Name>
```

## 3. Running Locally with `print` for Testing

To test locally, you can run the bot with the `print` bot type, which outputs results to the terminal instead of sending notifications to Discord.

### Example Usage

1. **Set Up Your Environment Variables**: Ensure your `.env` file contains all necessary credentials and configurations.
2. **Run the Bot with Print**: Use the following command to run the bot locally and print results to the console:
   ```bash
   go run main.go --keyword="tailscale" --keyword="kubernetes" --bot=print --searchers=hackernews --searchers=reddit
   ```

   - **Options**:
     - `--keyword`: Specify keywords to search for (repeatable).
     - `--bot`: Specify notification types (`print`, `discord`).
      - `--searchers`: Specify which searchers to use (`hackernews`, `reddit`, `bluesky`, `fediverse`, `youtube`, `x`).

3. **Check Output**: The bot will display search results in the terminal. This is useful for validating functionality without sending messages to Discord.

---

## Example `.env` File

Here’s a sample `.env` file with placeholders for required environment variables:

```env
# Discord
DISCORD_BOT_TOKEN=<Your Bot Token>
DISCORD_CHANNEL_ID=<Your Channel ID>

# Reddit
REDDIT_CLIENT_ID=<Your Reddit Client ID>
REDDIT_CLIENT_SECRET=<Your Reddit Client Secret>
REDDIT_USERNAME=<Your Reddit Username>
REDDIT_PASSWORD=<Your Reddit Password>

# Bluesky
BSKY_USERNAME=<Your Bluesky Handle>
BSKY_PASSWORD=<Your App Password>

# X
X_BEARER_TOKEN=<Your X API Bearer Token>

# AWS DynamoDB (if using DynamoDB)
AWS_ACCESS_KEY_ID=<Your AWS Access Key>
AWS_SECRET_ACCESS_KEY=<Your AWS Secret Key>
AWS_REGION=<Your AWS Region>
SOCIAL_SEARCH_TABLE_NAME=<Your DynamoDB Table Name>
```

## Troubleshooting

- **Authentication Issues**: Ensure all required environment variables are correctly set.
- **Permissions**: Verify that the bot has necessary permissions in the Discord channel.
- **API Limits**: Running frequent searches on certain platforms may trigger rate limits. Be mindful of API quotas.

---

## Deploying To AWS Lambda

Grass can run as a scheduled, native AWS Lambda function. This avoids reliance on GitHub Actions schedules and uses DynamoDB to retain duplicate-result and last-search-time state between invocations. Lambda cannot use SQLite because its filesystem is ephemeral.

The deployment is a Linux ARM64 Go `bootstrap` ZIP for Lambda's `provided.al2023` runtime. It does not build or publish a Docker image.

### Prerequisites

- Go 1.26 or later and the `zip` command.
- AWS SAM CLI and AWS credentials permitted to create CloudFormation, Lambda, EventBridge, IAM, CloudWatch Logs, and DynamoDB resources.
- Credentials for each selected searcher and notifier. The Lambda execution role supplies AWS credentials for DynamoDB; do not configure `AWS_ACCESS_KEY_ID` or `AWS_SECRET_ACCESS_KEY` for the function.

### Configuration

The SAM template supplies these required, non-secret Lambda environment variables:

| Variable | Description |
| --- | --- |
| `GRASS_KEYWORDS` | Comma-separated keywords, such as `grass,lambda`. |
| `GRASS_SEARCHERS` | Comma-separated searchers: `hackernews`, `reddit`, `bluesky`, `fediverse`, `youtube`, or `x`. |
| `GRASS_BOTS` | Comma-separated notifiers: `print`, `discord`, or `slack`. |
| `SOCIAL_SEARCH_TABLE_NAME` | Created automatically by the template; do not override it. |

Configure only the existing credential variables needed by the selected integrations. Do not commit their values or include them in `template.yaml`:

- Reddit: `REDDIT_CLIENT_ID`, `REDDIT_CLIENT_SECRET`, `REDDIT_USERNAME`, `REDDIT_PASSWORD`
- Bluesky: `BSKY_USERNAME`, `BSKY_PASSWORD`
- YouTube: `YOUTUBE_API_KEY`
- X: `X_BEARER_TOKEN`
- Discord: `DISCORD_BOT_TOKEN`, `DISCORD_CHANNEL_ID`
- Slack: `SLACK_BOT_TOKEN`, `SLACK_CHANNEL_ID`
- Fediverse: `FEDIVERSE_INSTANCES` plus the per-instance `*_ACCESS_TOKEN` or `*_CLIENT_ID` and `*_CLIENT_SECRET` variables described by the searcher

Inject those credentials from a secure deployment system or set the Lambda environment configuration after deployment. If they are applied separately, reapply the complete environment map after each `sam deploy`, because CloudFormation manages the non-secret configuration in the template.

### Build And Deploy

Build the standalone ZIP without Docker:

```bash
make lambda-package
```

The artifact is `dist/grass-lambda.zip` and contains an executable named `bootstrap`.

Build and deploy the SAM stack without `--use-container`:

```bash
sam build --template-file template.yaml
sam deploy --guided \
  --parameter-overrides \
    GrassKeywords='grass,lambda' \
    GrassSearchers='hackernews' \
    GrassBots='print' \
    ScheduleExpression='rate(1 hour)' \
    FunctionTimeout=300
```

The `ScheduleExpression` is an EventBridge schedule and defaults to `rate(1 hour)`. Select an interval that fits every enabled provider's API quotas. The function is limited to one concurrent invocation to preserve the existing DynamoDB-backed duplicate check.

### Monitoring And Rollback

CloudWatch logs are retained for 30 days by default. Inspect them with:

```bash
sam logs --stack-name <stack-name> --name GrassFunction --tail
```

Monitor Lambda duration, errors, throttles, and provider API responses before reducing the schedule interval. If a provider can exceed the default five-minute timeout, deploy with a larger `FunctionTimeout` value up to 900 seconds.

To stop scheduled runs, remove the stack:

```bash
sam delete --stack-name <stack-name>
```

The DynamoDB table is retained intentionally so duplicate history and search cursors survive rollback. Delete that table separately only when you intentionally want to reset Grass state.

---

This should get your Grass Bot up and running! For any issues, please refer to platform-specific documentation or API guides.

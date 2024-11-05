# Grass

> They're such a [grass](https://www.urbandictionary.com/define.php?term=Grass)!

Grass is a bot that searches various platforms (Hacker News, Reddit, and Bluesky) for posts containing specified keywords. It then saves results to a pluggable database and can notify via Discord or print the results to standard output.

## Features

- Search for specific keywords across multiple platforms (e.g., Hacker News, Reddit, Bluesky)
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
     - `--searchers`: Specify which searchers to use (`hackernews`, `reddit`, `bluesky`).

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

This should get your Grass Bot up and running! For any issues, please refer to platform-specific documentation or API guides.
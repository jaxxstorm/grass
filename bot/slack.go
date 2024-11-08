// bot/slack.go
package bot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/charmbracelet/log"
	"github.com/jaxxstorm/grass/search"
)

type SlackNotifier struct {
	token     string
	channelID string
}

func NewSlackNotifier() *SlackNotifier {
	token := os.Getenv("SLACK_BOT_TOKEN")
	channelID := os.Getenv("SLACK_CHANNEL_ID")

	if token == "" {
		log.Fatal("SLACK_BOT_TOKEN environment variable is not set")
	}
	if channelID == "" {
		log.Fatal("SLACK_CHANNEL_ID environment variable is not set")
	}

	return &SlackNotifier{token: token, channelID: channelID}
}

// Notify sends a formatted message to the specified Slack channel.
func (s *SlackNotifier) Notify(result search.SearchResult) error {
	// Convert Unix timestamp to a human-readable format
	timestamp := time.Unix(result.Timestamp, 0).Format("01/02/2006 03:04 PM")

	// Format the message with markdown-like styling for Slack
	message := fmt.Sprintf(
		"*%s*\n*Platform*: %s\n*Keyword*: %s\n*Posted*: %s\n%s\n<%s|Link>",
		result.Title,    // Bold title
		result.Platform, // Platform name
		result.Keyword,  // Keyword
		timestamp,       // Human-readable timestamp
		result.Content,  // Content of the post
		result.URL,      // URL as a clickable link
	)

	// Build the JSON payload for the Slack API request
	payload := map[string]interface{}{
		"channel": s.channelID,
		"text":    message,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Error("Failed to marshal payload", "error", err)
		return err
	}

	req, err := http.NewRequest("POST", "https://slack.com/api/chat.postMessage", bytes.NewBuffer(payloadBytes))
	if err != nil {
		log.Error("Failed to create Slack request", "error", err)
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Error("Failed to send message to Slack", "error", err)
		return err
	}
	defer resp.Body.Close()

	// Check if the request was successful
	if resp.StatusCode != http.StatusOK {
		log.Error("Slack API request failed", "status_code", resp.StatusCode)
		return fmt.Errorf("Slack API request failed with status code: %d", resp.StatusCode)
	}

	log.Info("Posted to Slack", "title", result.Title, "url", result.URL)
	return nil
}

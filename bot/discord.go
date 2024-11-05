package bot

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jaxxstorm/grass/search"
)

type DiscordNotifier struct {
	session   *discordgo.Session
	channelID string
}

func NewDiscordNotifier() *DiscordNotifier {
	token := os.Getenv("DISCORD_BOT_TOKEN")
	channelID := os.Getenv("DISCORD_CHANNEL_ID")

	if token == "" {
		log.Fatal("DISCORD_BOT_TOKEN environment variable is not set")
	}
	if channelID == "" {
		log.Fatal("DISCORD_CHANNEL_ID environment variable is not set")
	}

	session, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatalf("Failed to create Discord session: %v", err)
	}

	err = session.Open()
	if err != nil {
		log.Fatalf("Error opening connection to Discord: %v", err)
	}

	return &DiscordNotifier{session: session, channelID: channelID}
}

// Notify sends a formatted message with markdown to the specified Discord channel.
func (d *DiscordNotifier) Notify(result search.SearchResult) error {
	// Convert Unix timestamp to a human-readable format
	timestamp := time.Unix(result.Timestamp, 0).Format("01/02/2006 03:04 PM")

	// Format the message using markdown
	message := fmt.Sprintf(
		"**%s**\n*Platform*: %s\n*Keyword*: %s\n*Posted*: %s\n%s\n%s",
		result.Title,             // Bold title
		result.Platform,          // Platform name
		result.Keyword,           // Keyword
		timestamp,                // Human-readable timestamp
		result.Content,           // Content of the post
		result.URL,               // URL (should unfurl automatically)
	)

	// Send the markdown-formatted message
	_, err := d.session.ChannelMessageSend(d.channelID, message)
	if err != nil {
		log.Printf("Failed to send message to Discord: %v", err)
		return err
	}

	log.Printf("Posted to Discord: %s - %s\n", result.Title, result.URL)
	return nil
}

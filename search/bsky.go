// search/bluesky.go
package search

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/charmbracelet/log"
	"net/http"
	"os"
	"strings"
	"time"
)

type BlueskySearcher struct {
	accessToken string
}

// NewBlueskySearcher initializes the BlueskySearcher with API credentials.
func NewBlueskySearcher() (*BlueskySearcher, error) {
	username := os.Getenv("BSKY_USERNAME")
	password := os.Getenv("BSKY_PASSWORD")

	if username == "" || password == "" {
		return nil, errors.New("missing Bluesky API credentials: BSKY_USERNAME and BSKY_PASSWORD are required")
	}

	searcher := &BlueskySearcher{}
	if err := searcher.authenticate(username, password); err != nil {
		return nil, fmt.Errorf("failed to authenticate with Bluesky: %w", err)
	}
	return searcher, nil
}

// authenticate logs in to Bluesky and retrieves an access token.
func (b *BlueskySearcher) authenticate(username, password string) error {
	url := "https://bsky.social/xrpc/com.atproto.server.createSession"
	payload := map[string]string{"identifier": username, "password": password}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("authentication failed with status code: %d", resp.StatusCode)
	}

	var result struct {
		AccessJwt string `json:"accessJwt"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to parse access token: %w", err)
	}

	b.accessToken = result.AccessJwt
	return nil
}

// Platform returns the platform name for this searcher.
func (b *BlueskySearcher) Platform() string {
	return "Bluesky"
}

// convertAtURLToHTTPS converts a Bluesky-specific "at://" URL to a clickable HTTPS URL.
func convertAtURLToHTTPS(atURL string) string {
	// Remove the "at://" prefix and split the remaining string
	parts := strings.Split(atURL, "/")
	if len(parts) < 5 {
		return atURL // Return as-is if the URL format is unexpected
	}
	did := parts[2]
	postID := parts[4]
	return fmt.Sprintf("https://bsky.app/profile/%s/post/%s", did, postID)
}

// Search queries Bluesky for posts matching a keyword.
func (b *BlueskySearcher) Search(keyword string, afterEpochSecs int64) ([]SearchResult, error) {
	url := fmt.Sprintf("https://bsky.social/xrpc/app.bsky.feed.searchPosts?q=%s", keyword)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+b.accessToken)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search request failed with status code: %d, response: %v", resp.StatusCode, resp)
	}

	// Parse the response from Bluesky
	var data struct {
		Posts []struct {
			Uri    string `json:"uri"`
			Author struct {
				DisplayName string `json:"displayName"`
			} `json:"author"`
			Record struct {
				CreatedAt string `json:"createdAt"` // Timestamp is nested in the "record" field
				Text      string `json:"text"`
			} `json:"record"`
		} `json:"posts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to parse search results: %w", err)
	}

	// Convert results to the SearchResult format
	var results []SearchResult
	for _, post := range data.Posts {
		// Skip if Record.CreatedAt is empty
		if post.Record.CreatedAt == "" {
			log.Printf("Skipping post with missing Record.CreatedAt field: %s", post.Uri)
			continue
		}

		// Parse the created time from the Record.CreatedAt field
		createdTime, err := time.Parse(time.RFC3339, post.Record.CreatedAt)
		if err != nil {
			log.Printf("Skipping post with invalid Record.CreatedAt format: %s", post.Record.CreatedAt)
			continue
		}

		// Filter by the specified epoch time
		if createdTime.Unix() > afterEpochSecs {
			results = append(results, SearchResult{
				Platform:  b.Platform(),
				Keyword:   keyword,
				Title:     fmt.Sprintf("Post by %s", post.Author.DisplayName),
				URL:       convertAtURLToHTTPS(post.Uri), // Convert URL to clickable format
				Timestamp: createdTime.Unix(),
			})
		}
	}

	return results, nil
}

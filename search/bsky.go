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
	
	// Try authentication with retries
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		err := searcher.authenticate(username, password)
		if err == nil {
			return searcher, nil
		}

		// Check if it's a rate limit error
		if strings.Contains(err.Error(), "status code: 429") {
			retryDelay := time.Duration(5*(i+1)) * time.Second // Exponential backoff
			log.Warn("authentication rate limited, retrying...",
				"attempt", i+1,
				"max_attempts", maxRetries,
				"retry_delay", retryDelay)
			time.Sleep(retryDelay)
			continue
		}

		// If it's not a rate limit error, return the error immediately
		return nil, fmt.Errorf("failed to authenticate with Bluesky: %w", err)
	}

	// If we've exhausted all retries
	log.Warn("could not authenticate due to rate limits, continuing with empty searcher")
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

	if resp.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf("status code: 429")
	}

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
	// If we don't have an access token, return empty results
	if b.accessToken == "" {
		log.Warn("search attempted without valid authentication",
			"platform", "Bluesky",
			"keyword", keyword)
		return []SearchResult{}, nil
	}

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

	// Handle rate limiting
	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := resp.Header.Get("Retry-After")
		log.Warn("rate limit exceeded", 
			"platform", b.Platform(),
			"keyword", keyword,
			"retry_after", retryAfter)
		return []SearchResult{}, nil
	}

	if resp.StatusCode != http.StatusOK {
		log.Warn("search request failed",
			"platform", b.Platform(),
			"keyword", keyword,
			"status_code", resp.StatusCode)
		return []SearchResult{}, nil
	}

	var data struct {
		Posts []struct {
			Uri    string `json:"uri"`
			Author struct {
				DisplayName string `json:"displayName"`
			} `json:"author"`
			Record struct {
				CreatedAt string `json:"createdAt"`
				Text      string `json:"text"`
			} `json:"record"`
		} `json:"posts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		log.Warn("failed to parse search results",
			"platform", b.Platform(),
			"keyword", keyword,
			"error", err)
		return []SearchResult{}, nil
	}

	var results []SearchResult
	for _, post := range data.Posts {
		if post.Record.CreatedAt == "" {
			log.Warn("skipping post with missing created_at",
				"platform", b.Platform(),
				"uri", post.Uri)
			continue
		}

		createdTime, err := time.Parse(time.RFC3339, post.Record.CreatedAt)
		if err != nil {
			log.Warn("skipping post with invalid date format",
				"platform", b.Platform(),
				"created_at", post.Record.CreatedAt,
				"error", err)
			continue
		}

		if createdTime.Unix() > afterEpochSecs {
			results = append(results, SearchResult{
				Platform:  b.Platform(),
				Keyword:   keyword,
				Title:     fmt.Sprintf("Post by %s", post.Author.DisplayName),
				URL:       convertAtURLToHTTPS(post.Uri),
				Timestamp: createdTime.Unix(),
			})
		}
	}

	return results, nil
}
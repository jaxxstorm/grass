package search

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"
)

// YouTubeSearcher implements the Searcher interface for YouTube.
type YouTubeSearcher struct {
	apiKey string
}

// NewYouTubeSearcher initializes YouTubeSearcher with the API key.
func NewYouTubeSearcher() (*YouTubeSearcher, error) {
	apiKey := os.Getenv("YOUTUBE_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("missing YouTube API key: YOUTUBE_API_KEY is required")
	}

	return &YouTubeSearcher{apiKey: apiKey}, nil
}

// Platform returns the platform name for this searcher.
func (y *YouTubeSearcher) Platform() string {
	return "YouTube"
}

// Search performs a keyword search on YouTube and filters results based on the timestamp.
func (y *YouTubeSearcher) Search(keyword string, afterEpochSecs int64) ([]SearchResult, error) {
	// YouTube API URL
	searchURL := fmt.Sprintf(
		"https://www.googleapis.com/youtube/v3/search?part=snippet&q=%s&key=%s&type=video&order=date",
		url.QueryEscape(keyword), y.apiKey,
	)

	// Send HTTP GET request
	resp, err := http.Get(searchURL)
	if err != nil {
		return nil, fmt.Errorf("failed to perform YouTube search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("YouTube search request failed with status code: %d", resp.StatusCode)
	}

	// Parse response JSON
	var data struct {
		Items []struct {
			ID struct {
				VideoID string `json:"videoId"`
			} `json:"id"`
			Snippet struct {
				Title       string `json:"title"`
				PublishedAt string `json:"publishedAt"`
				Description string `json:"description"`
			} `json:"snippet"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to parse YouTube search results: %w", err)
	}

	// Filter and format results
	var results []SearchResult
	for _, item := range data.Items {
		// Parse the publication time
		publishedTime, err := time.Parse(time.RFC3339, item.Snippet.PublishedAt)
		if err != nil {
			continue
		}

		// Only include results after the specified epoch time
		if publishedTime.Unix() > afterEpochSecs {
			videoURL := fmt.Sprintf("https://www.youtube.com/watch?v=%s", item.ID.VideoID)
			results = append(results, SearchResult{
				Platform:  y.Platform(),
				Keyword:   keyword,
				Title:     item.Snippet.Title,
				URL:       videoURL,
				Timestamp: publishedTime.Unix(),
				Content:   item.Snippet.Description,
			})
		}
	}

	return results, nil
}

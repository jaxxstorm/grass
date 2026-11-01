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
	return y.SearchPages(keyword, afterEpochSecs, nil)
}

// SearchPages reserves continuation requests before each additional HTTP page.
// The caller reserves the initial request, including its keyword cooldown.
func (y *YouTubeSearcher) SearchPages(keyword string, afterEpochSecs int64, reservePage func() error) ([]SearchResult, error) {
	params := url.Values{"part": {"snippet"}, "q": {keyword}, "key": {y.apiKey}, "type": {"video"}, "order": {"date"}, "maxResults": {"50"}, "publishedAfter": {time.Unix(afterEpochSecs, 0).UTC().Format(time.RFC3339)}}
	var results []SearchResult
	seen := map[string]bool{}
	for {
		searchURL := "https://www.googleapis.com/youtube/v3/search?" + params.Encode()

		// Send HTTP GET request
		resp, err := http.Get(searchURL)
		if err != nil {
			return nil, fmt.Errorf("failed to perform YouTube search request: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("YouTube search request failed with status code: %d", resp.StatusCode)
		}

		// Parse response JSON
		var data struct {
			NextPageToken string `json:"nextPageToken"`
			Items         []struct {
				ID struct {
					VideoID string `json:"videoId"`
				} `json:"id"`
				Snippet struct {
					Title        string `json:"title"`
					PublishedAt  string `json:"publishedAt"`
					Description  string `json:"description"`
					ChannelID    string `json:"channelId"`
					ChannelTitle string `json:"channelTitle"`
				} `json:"snippet"`
			} `json:"items"`
		}
		err = json.NewDecoder(resp.Body).Decode(&data)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to parse YouTube search results: %w", err)
		}

		// Filter and format results
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
					Platform:   y.Platform(),
					Keyword:    keyword,
					Title:      item.Snippet.Title,
					URL:        videoURL,
					Timestamp:  publishedTime.Unix(),
					Content:    item.Snippet.Description,
					AuthorID:   item.Snippet.ChannelID,
					AuthorName: item.Snippet.ChannelTitle,
				})
			}
		}

		if data.NextPageToken == "" {
			return results, nil
		}
		if seen[data.NextPageToken] {
			return nil, fmt.Errorf("YouTube repeated pagination token")
		}
		seen[data.NextPageToken] = true
		if reservePage != nil {
			if err := reservePage(); err != nil {
				return nil, err
			}
		}
		params.Set("pageToken", data.NextPageToken)
	}
}

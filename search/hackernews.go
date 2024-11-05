// search/hackernews.go
package search

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

type HackerNewsSearcher struct{}

func NewHackerNewsSearcher() *HackerNewsSearcher {
	return &HackerNewsSearcher{}
}

// Platform returns the name of the platform for this searcher.
func (h *HackerNewsSearcher) Platform() string {
	return "HackerNews"
}

// Search performs a keyword search on Hacker News after a specified epoch time.
func (h *HackerNewsSearcher) Search(keyword string, afterEpochSecs int64) ([]SearchResult, error) {
	apiURL := fmt.Sprintf(
		"https://hn.algolia.com/api/v1/search_by_date?query=%s&tags=(story,comment)&numericFilters=created_at_i>%d",
		keyword, afterEpochSecs,
	)
	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %s", resp.Status)
	}

	var result struct {
		Hits []struct {
			Title    string `json:"title"`
			URL      string `json:"url"`
			ObjectID string `json:"objectID"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var results []SearchResult
	timestamp := time.Now().Unix()
	for _, hit := range result.Hits {
		if hit.Title == "" || hit.ObjectID == "" {
			log.Printf("Skipping hit due to missing title or objectID")
			continue
		}

		// Use the Hacker News post URL rather than the external URL
		hackerNewsURL := fmt.Sprintf("https://news.ycombinator.com/item?id=%s", hit.ObjectID)
		results = append(results, SearchResult{
			Platform:  h.Platform(),
			Keyword:   keyword,
			Title:     hit.Title,
			URL:       hackerNewsURL,
			Timestamp: timestamp,
		})
	}

	return results, nil
}

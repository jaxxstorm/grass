package search

import (
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/log"
	"net/http"
	"net/url"
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
	params := url.Values{
		"query":          {keyword},
		"tags":           {"(story,comment)"},
		"numericFilters": {fmt.Sprintf("created_at_i>%d", afterEpochSecs)},
	}
	apiURL := "https://hn.algolia.com/api/v1/search_by_date?" + params.Encode()
	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("Hacker News request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Hacker News search failed: HTTP %d", resp.StatusCode)
	}

	var result struct {
		Hits []struct {
			Title       string   `json:"title"`
			URL         string   `json:"url"`
			ObjectID    string   `json:"objectID"`
			CreatedAt   int64    `json:"created_at_i"`
			CommentText string   `json:"comment_text"`
			StoryTitle  string   `json:"story_title"`
			Type        []string `json:"_tags"`
		} `json:"hits"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode Hacker News search response: %w", err)
	}

	var results []SearchResult
	timestamp := time.Now().Unix()
	for _, hit := range result.Hits {
		if hit.ObjectID == "" {
			log.Debug("skipping hit due to missing objectID")
			continue
		}

		// Build the HN URL
		hackerNewsURL := fmt.Sprintf("https://news.ycombinator.com/item?id=%s", hit.ObjectID)

		// Check if this is a comment by looking for "comment" in the tags array
		isComment := false
		for _, tag := range hit.Type {
			if tag == "comment" {
				isComment = true
				break
			}
		}

		title := hit.Title
		content := ""

		if isComment {
			// For comments, use the story title and comment text
			if hit.StoryTitle != "" {
				title = fmt.Sprintf("Comment on: %s", hit.StoryTitle)
			}
			content = hit.CommentText
		}

		// Skip if we couldn't determine a title
		if title == "" {
			log.Debug("skipping hit due to missing title", "objectID", hit.ObjectID)
			continue
		}

		results = append(results, SearchResult{
			Platform:  h.Platform(),
			Keyword:   keyword,
			Title:     title,
			URL:       hackerNewsURL,
			Content:   content,
			Timestamp: timestamp,
		})
	}

	return results, nil
}

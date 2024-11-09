package search

import (
    "encoding/json"
    "fmt"
    "github.com/charmbracelet/log"
    "net/http"
    "strings"
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
            Title       string `json:"title"`
            URL         string `json:"url"`
            ObjectID    string `json:"objectID"`
            CreatedAt   int64  `json:"created_at_i"`
            CommentText string `json:"comment_text"`
            StoryTitle  string `json:"story_title"`
            Type        string `json:"_tags"` // Changed to string
        } `json:"hits"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, err
    }

    var results []SearchResult
    timestamp := time.Now().Unix()
    for _, hit := range result.Hits {
        if hit.ObjectID == "" {
            log.Debug("Skipping hit due to missing objectID")
            continue
        }

        // Build the HN URL
        hackerNewsURL := fmt.Sprintf("https://news.ycombinator.com/item?id=%s", hit.ObjectID)

        // Check if this is a comment
        isComment := strings.Contains(hit.Type, "comment")

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
            log.Debug("Skipping hit due to missing title", "objectID", hit.ObjectID)
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
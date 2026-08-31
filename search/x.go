package search

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"
)

const xRecentSearchURL = "https://api.x.com/2/tweets/search/recent"

// XSearcher searches public X posts through the official X API.
type XSearcher struct {
	bearerToken string
	client      *http.Client
	searchURL   string
}

// NewXSearcher initializes an X searcher using an app Bearer token.
func NewXSearcher() (*XSearcher, error) {
	bearerToken := os.Getenv("X_BEARER_TOKEN")
	if bearerToken == "" {
		return nil, errors.New("missing X API bearer token: X_BEARER_TOKEN is required")
	}

	return &XSearcher{
		bearerToken: bearerToken,
		client:      http.DefaultClient,
		searchURL:   xRecentSearchURL,
	}, nil
}

// Platform returns the platform name for this searcher.
func (x *XSearcher) Platform() string {
	return "X"
}

// Search queries X's recent-search endpoint and returns posts newer than the watermark.
func (x *XSearcher) Search(keyword string, afterEpochSecs int64) ([]SearchResult, error) {
	client := x.client
	if client == nil {
		client = http.DefaultClient
	}

	searchURL := x.searchURL
	if searchURL == "" {
		searchURL = xRecentSearchURL
	}

	params := url.Values{
		"query":       {keyword},
		"max_results": {"100"},
		"sort_order":  {"recency"},
		"post.fields": {"created_at,author_id"},
		"expansions":  {"author_id"},
		"user.fields": {"name,username"},
	}

	// Recent search only accepts a start time within the last seven days. Omitting it
	// on an initial or stale run lets the API return its full available window.
	if afterEpochSecs >= time.Now().Add(-7*24*time.Hour).Unix() {
		params.Set("start_time", time.Unix(afterEpochSecs, 0).UTC().Format(time.RFC3339))
	}

	var results []SearchResult
	for {
		requestURL := searchURL + "?" + params.Encode()
		req, err := http.NewRequest(http.MethodGet, requestURL, nil)
		if err != nil {
			return nil, fmt.Errorf("create X search request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+x.bearerToken)

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("perform X search request: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("X search request failed: %s", resp.Status)
		}

		var data struct {
			Data []struct {
				ID        string `json:"id"`
				Text      string `json:"text"`
				CreatedAt string `json:"created_at"`
				AuthorID  string `json:"author_id"`
			} `json:"data"`
			Includes struct {
				Users []struct {
					ID       string `json:"id"`
					Name     string `json:"name"`
					Username string `json:"username"`
				} `json:"users"`
			} `json:"includes"`
			Meta struct {
				NextToken string `json:"next_token"`
			} `json:"meta"`
		}

		err = json.NewDecoder(resp.Body).Decode(&data)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("parse X search response: %w", err)
		}

		users := make(map[string]struct{ name, username string }, len(data.Includes.Users))
		for _, user := range data.Includes.Users {
			users[user.ID] = struct{ name, username string }{user.Name, user.Username}
		}

		for _, post := range data.Data {
			createdAt, err := time.Parse(time.RFC3339, post.CreatedAt)
			if err != nil || createdAt.Unix() <= afterEpochSecs {
				continue
			}

			user := users[post.AuthorID]
			postURL := fmt.Sprintf("https://x.com/i/web/status/%s", post.ID)
			title := "Post on X"
			if user.username != "" {
				postURL = fmt.Sprintf("https://x.com/%s/status/%s", user.username, post.ID)
				title = fmt.Sprintf("Post by @%s", user.username)
			}
			if user.name != "" && user.username != "" {
				title = fmt.Sprintf("Post by %s (@%s)", user.name, user.username)
			}

			results = append(results, SearchResult{
				Platform:  x.Platform(),
				Keyword:   keyword,
				Title:     title,
				URL:       postURL,
				Timestamp: createdAt.Unix(),
				Content:   post.Text,
			})
		}

		if data.Meta.NextToken == "" {
			break
		}
		params.Set("next_token", data.Meta.NextToken)
	}

	return results, nil
}

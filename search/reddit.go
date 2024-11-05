// search/reddit.go
package search

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"
)

type RedditSearcher struct {
	clientID     string
	clientSecret string
	username     string
	password     string
	accessToken  string
}

func NewRedditSearcher() (*RedditSearcher, error) {
	clientID := os.Getenv("REDDIT_CLIENT_ID")
	clientSecret := os.Getenv("REDDIT_CLIENT_SECRET")
	username := os.Getenv("REDDIT_USERNAME")
	password := os.Getenv("REDDIT_PASSWORD")

	if clientID == "" || clientSecret == "" || username == "" || password == "" {
		return nil, errors.New("missing Reddit API credentials")
	}

	searcher := &RedditSearcher{
		clientID:     clientID,
		clientSecret: clientSecret,
		username:     username,
		password:     password,
	}
	if err := searcher.authenticate(); err != nil {
		return nil, err
	}
	return searcher, nil
}

// Platform returns the name of the platform for this searcher.
func (r *RedditSearcher) Platform() string {
	return "Reddit"
}

// Authenticate with Reddit to get an access token
func (r *RedditSearcher) authenticate() error {
	data := url.Values{}
	data.Set("grant_type", "password")
	data.Set("username", r.username)
	data.Set("password", r.password)

	req, err := http.NewRequest("POST", "https://www.reddit.com/api/v1/access_token", bytes.NewBufferString(data.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth(r.clientID, r.clientSecret)
	req.Header.Set("User-Agent", "GoRedditBot/1.0")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to authenticate with Reddit: %s", resp.Status)
	}

	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	r.accessToken = result.AccessToken
	return nil
}

// Search Reddit for posts matching a keyword after a specific epoch time
func (r *RedditSearcher) Search(keyword string, afterEpochSecs int64) ([]SearchResult, error) {
	searchURL := fmt.Sprintf("https://oauth.reddit.com/search?q=%s&sort=new&restrict_sr=1", url.QueryEscape(keyword))
	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.accessToken)
	req.Header.Set("User-Agent", "GoRedditBot/1.0")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search request failed: %s", resp.Status)
	}

	var data struct {
		Data struct {
			Children []struct {
				Data struct {
					Title     string  `json:"title"`
					URL       string  `json:"url"`
					Permalink string  `json:"permalink"`
					CreatedAt float64 `json:"created_utc"`
				} `json:"data"`
			} `json:"children"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var results []SearchResult
	timestamp := time.Now().Unix()
	for _, child := range data.Data.Children {
		post := child.Data
		// Only include results after the specified epoch time
		if int64(post.CreatedAt) > afterEpochSecs {
			// Use permalink to link directly to the Reddit post
			postURL := fmt.Sprintf("https://www.reddit.com%s", post.Permalink)
			results = append(results, SearchResult{
				Platform:  r.Platform(),
				Keyword:   keyword,
				Title:     post.Title,
				URL:       postURL,
				Timestamp: timestamp,
			})
		}
	}

	return results, nil
}

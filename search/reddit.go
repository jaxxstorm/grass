// search/reddit.go
package search

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/log"
)

type RedditSearcher struct {
	clientID     string
	clientSecret string
	username     string
	password     string
	accessToken  string
	refreshToken string
}

func NewRedditSearcher() (*RedditSearcher, error) {
	clientID := os.Getenv("REDDIT_CLIENT_ID")
	clientSecret := os.Getenv("REDDIT_CLIENT_SECRET")
	username := os.Getenv("REDDIT_USERNAME")
	password := os.Getenv("REDDIT_PASSWORD")
	refreshToken := os.Getenv("REDDIT_REFRESH_TOKEN")

	if clientID == "" || clientSecret == "" || (refreshToken == "" && (username == "" || password == "")) {
		return nil, errors.New("missing Reddit API credentials")
	}

	searcher := &RedditSearcher{
		clientID:     clientID,
		clientSecret: clientSecret,
		username:     username,
		password:     password,
		refreshToken: refreshToken,
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
	if r.refreshToken != "" {
		data.Set("grant_type", "refresh_token")
		data.Set("refresh_token", r.refreshToken)
	} else {
		data.Set("grant_type", "password")
		data.Set("username", r.username)
		data.Set("password", r.password)
	}

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
		return fmt.Errorf("failed to authenticate with Reddit: HTTP %d; %s", resp.StatusCode, redditResponseSummary(resp.Body))
	}

	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if result.Error != "" {
		return fmt.Errorf("Reddit OAuth failed: %s", redditOAuthError(result.Error))
	}
	if strings.TrimSpace(result.AccessToken) == "" {
		return errors.New("Reddit OAuth returned HTTP 200 without an access token")
	}
	r.accessToken = result.AccessToken
	log.Info("Reddit authentication succeeded", "access_token_present", true)
	return nil
}

// Search Reddit for posts matching a keyword after a specific epoch time
func (r *RedditSearcher) Search(keyword string, afterEpochSecs int64) ([]SearchResult, error) {
	matchKeyword := strings.ToLower(strings.TrimSpace(keyword))
	if matchKeyword == "" {
		return nil, nil
	}
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
		return nil, fmt.Errorf("Reddit search failed: HTTP %d; %s", resp.StatusCode, redditResponseSummary(resp.Body))
	}

	var data struct {
		Data struct {
			Children []struct {
				Data struct {
					Title     string  `json:"title"`
					URL       string  `json:"url"`
					Permalink string  `json:"permalink"`
					CreatedAt float64 `json:"created_utc"`
					Selftext  string  `json:"selftext"`
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
		// Reddit can return unrelated results; require a literal match in post content.
		if !strings.Contains(strings.ToLower(post.Title), matchKeyword) &&
			!strings.Contains(strings.ToLower(post.Selftext), matchKeyword) &&
			!strings.Contains(strings.ToLower(post.URL), matchKeyword) {
			continue
		}
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

// Only emit known classifications, never arbitrary server response content.
func redditOAuthError(code string) string {
	switch code {
	case "invalid_grant", "invalid_client", "unauthorized_client", "unsupported_grant_type", "invalid_scope", "invalid_request":
		return code
	default:
		return "unrecognized OAuth error (details omitted)"
	}
}

func redditResponseSummary(body io.Reader) string {
	data, err := io.ReadAll(io.LimitReader(body, 16384))
	if err != nil {
		return "response unreadable"
	}
	var response struct {
		Error   json.RawMessage `json:"error"`
		Message string          `json:"message"`
	}
	if json.Unmarshal(data, &response) == nil {
		var code string
		if json.Unmarshal(response.Error, &code) == nil && code != "" {
			return "JSON response: " + redditOAuthError(code)
		}
		switch strings.ToLower(response.Message) {
		case "forbidden", "unauthorized", "too many requests":
			return "JSON response: " + strings.ToLower(response.Message)
		}
		return "JSON error response (details omitted)"
	}
	lower := strings.ToLower(string(data))
	if strings.Contains(lower, "blocked by network security") {
		return "response indicates blocked by network security"
	}
	if strings.Contains(lower, "<html") || strings.Contains(lower, "<!doctype html") {
		return "HTML error page (details omitted)"
	}
	return "unrecognized error response (details omitted)"
}

// search/fediverse.go
package search

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/log"
)

// FediverseSearcher is a searcher for posts on multiple Mastodon instances with OAuth2 support.
type FediverseSearcher struct {
	instanceURLs map[string]string // Instance URL -> access token
}

// NewFediverseSearcher initializes the searcher with a list of instance URLs and obtains access tokens.
func NewFediverseSearcher() (*FediverseSearcher, error) {
	instancesEnv := os.Getenv("FEDIVERSE_INSTANCES")
	if instancesEnv == "" {
		return nil, fmt.Errorf("missing environment variable: FEDIVERSE_INSTANCES")
	}

	// Parse and initialize instances with tokens
	instanceURLs := make(map[string]string)
	for _, instanceURL := range strings.Split(instancesEnv, ",") {
		instanceURL = strings.TrimSpace(instanceURL)
		token, err := getAccessTokenForInstance(instanceURL)
		if err != nil {
			log.Printf("Error obtaining access token for instance %s: %v", instanceURL, err)
			continue
		}
		instanceURLs[instanceURL] = token
	}

	return &FediverseSearcher{instanceURLs: instanceURLs}, nil
}

// Platform returns the platform name for this searcher.
func (f *FediverseSearcher) Platform() string {
	return "Fediverse"
}

// getAccessTokenForInstance authenticates with the instance and retrieves an access token.
func getAccessTokenForInstance(instanceURL string) (string, error) {
	// Construct environment variable names dynamically based on the instance URL
	instanceEnvPrefix := strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(instanceURL, "https://", ""), ".", "_"))
	clientID := os.Getenv(instanceEnvPrefix + "_CLIENT_ID")
	clientSecret := os.Getenv(instanceEnvPrefix + "_CLIENT_SECRET")
	accessToken := os.Getenv(instanceEnvPrefix + "_ACCESS_TOKEN")

	if accessToken != "" {
		return accessToken, nil
	}

	if clientID == "" || clientSecret == "" {
		return "", fmt.Errorf("missing client ID or client secret for instance %s", instanceURL)
	}

	// Authenticate with the instance to obtain a new access token
	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)
	data.Set("grant_type", "client_credentials")
	data.Set("scope", "read")

	tokenURL := fmt.Sprintf("%s/oauth/token", instanceURL)
	resp, err := http.PostForm(tokenURL, data)
	if err != nil {
		return "", fmt.Errorf("failed to request access token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to authenticate with status code: %d", resp.StatusCode)
	}

	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to parse access token: %w", err)
	}

	return result.AccessToken, nil
}

// cleanHTMLContent removes HTML tags and decodes HTML entities in the content
func cleanHTMLContent(content string) string {
	// Remove HTML tags
	re := regexp.MustCompile(`<.*?>`)
	content = re.ReplaceAllString(content, "")

	// Decode HTML entities
	return html.UnescapeString(content)
}

// Search performs a search for posts matching `@tailscale` or `#tailscale` on each specified instance.
func (f *FediverseSearcher) Search(keyword string, afterEpochSecs int64) ([]SearchResult, error) {
	var allResults []SearchResult

	for instanceURL, accessToken := range f.instanceURLs {
		searchURL := fmt.Sprintf("%s/api/v2/search?q=%s&resolve=true", instanceURL, url.QueryEscape(keyword))

		// Create a new request with Authorization header
		req, err := http.NewRequest("GET", searchURL, nil)
		if err != nil {
			log.Printf("Failed to create search request for instance %s: %v", instanceURL, err)
			continue
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)

		// Send the request
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Failed to perform search request on instance %s: %v", instanceURL, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			log.Printf("Search request failed on instance %s with status code: %d", instanceURL, resp.StatusCode)
			continue
		}

		// Parse the response JSON
		var data struct {
			Statuses []struct {
				Content   string `json:"content"`
				URL       string `json:"url"`
				CreatedAt string `json:"created_at"`
				Account   struct {
					DisplayName string `json:"display_name"`
					Acct        string `json:"acct"`
				} `json:"account"`
			} `json:"statuses"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			log.Printf("Failed to parse search results from instance %s: %v", instanceURL, err)
			continue
		}

		// Filter and format results
		for _, status := range data.Statuses {
			// Only include results after the specified epoch time
			createdTime, err := time.Parse(time.RFC3339, status.CreatedAt)
			if err != nil {
				log.Printf("Skipping post with invalid CreatedAt format on instance %s: %v", instanceURL, status.CreatedAt)
				continue
			}
			if createdTime.Unix() <= afterEpochSecs {
				continue
			}

			// Clean the content before creating the SearchResult
			cleanedContent := cleanHTMLContent(status.Content)

			allResults = append(allResults, SearchResult{
				Platform:  f.Platform(),
				Keyword:   keyword,
				Title:     fmt.Sprintf("Post by %s (@%s)", status.Account.DisplayName, status.Account.Acct),
				URL:       status.URL,
				Timestamp: createdTime.Unix(),
				Content:   cleanedContent,
			})
		}
	}

	return allResults, nil
}

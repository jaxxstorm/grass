package search

import (
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
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

// getEnvVarForInstance builds environment variable names based on the instance URL.
func getEnvVarForInstance(instanceURL, suffix string) string {
	// Convert the instance URL to a standardized format (e.g., hachyderm_io)
	sanitizedInstance := strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(instanceURL, "https://", ""), ".", "_"))
	return fmt.Sprintf("%s_%s", sanitizedInstance, suffix)
}

// getAccessTokenForInstance authenticates with the instance and retrieves an access token.
func getAccessTokenForInstance(instanceURL string) (string, error) {
	// Dynamically get client ID, client secret, and access token env vars based on instance URL
	clientID := os.Getenv(getEnvVarForInstance(instanceURL, "CLIENT_ID"))
	clientSecret := os.Getenv(getEnvVarForInstance(instanceURL, "CLIENT_SECRET"))
	accessTokenEnv := os.Getenv(getEnvVarForInstance(instanceURL, "ACCESS_TOKEN"))

	if accessTokenEnv != "" {
		return accessTokenEnv, nil
	}

	// Obtain token via OAuth
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
		return "", fmt.Errorf("failed to authenticate, status code: %d", resp.StatusCode)
	}

	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to parse access token: %w", err)
	}

	return result.AccessToken, nil
}

// Search performs a search for posts matching `@tailscale` or `#tailscale` on each specified instance.
func (f *FediverseSearcher) Search(keyword string, afterEpochSecs int64) ([]SearchResult, error) {
	var allResults []SearchResult

	for instanceURL, accessToken := range f.instanceURLs {
		searchURL := fmt.Sprintf("%s/api/v2/search?q=%s&resolve=true", instanceURL, url.QueryEscape(keyword))

		// Create a new request with Authorization header
		req, err := http.NewRequest("GET", searchURL, nil)
		if err != nil {
			log.Printf("failed to create search request for instance %s: %v", instanceURL, err)
			continue
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)

		// Send the request
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("failed to perform search request on instance %s: %v", instanceURL, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			log.Printf("search request failed on instance %s with status code: %d", instanceURL, resp.StatusCode)
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
			log.Printf("failed to parse search results from instance %s: %v", instanceURL, err)
			continue
		}

		// Filter and format results
		for _, status := range data.Statuses {
			createdTime, err := time.Parse(time.RFC3339, status.CreatedAt)
			if err != nil {
				log.Printf("Skipping post with invalid CreatedAt format on instance %s: %v", instanceURL, status.CreatedAt)
				continue
			}
			if createdTime.Unix() <= afterEpochSecs {
				continue
			}

			// Check for @tailscale or #tailscale mentions in the content
			if strings.Contains(status.Content, "@tailscale") || strings.Contains(status.Content, "#tailscale") {
				allResults = append(allResults, SearchResult{
					Platform:  f.Platform(),
					Keyword:   keyword,
					Title:     fmt.Sprintf("Post by %s (@%s)", status.Account.DisplayName, status.Account.Acct),
					URL:       status.URL,
					Timestamp: createdTime.Unix(),
					Content:   status.Content,
				})
			}
		}
	}

	return allResults, nil
}

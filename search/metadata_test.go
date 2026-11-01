package search

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestProviderMetadata(t *testing.T) {
	for _, tc := range []struct {
		name                           string
		searcher                       Searcher
		body                           string
		id, handle, nameLabel, content string
	}{
		{"hn", &HackerNewsSearcher{}, `{"hits":[{"objectID":"1","title":"Story","author":"alice","story_text":"body"}]}`, "", "alice", "", "body"},
		{"reddit", &RedditSearcher{accessToken: "test"}, `{"data":{"children":[{"data":{"title":"Test story","permalink":"/r/test/1","created_utc":100,"author":"alice","author_fullname":"t2_1","selftext":"body"}}]}}`, "t2_1", "alice", "", "body"},
		{"reddit-deleted", &RedditSearcher{accessToken: "test"}, `{"data":{"children":[{"data":{"title":"Test story","permalink":"/r/test/1","created_utc":100,"author":"[deleted]","author_fullname":"t2_1"}}]}}`, "", "", "", ""},
		{"bluesky", &BlueskySearcher{accessToken: "test"}, `{"posts":[{"uri":"at://did:plc:1/app.bsky.feed.post/1","author":{"did":"did:plc:1","handle":"alice.test","displayName":"Alice"},"record":{"createdAt":"2026-01-01T00:00:00Z","text":"body"}}]}`, "did:plc:1", "alice.test", "Alice", "body"},
		{"bluesky-missing", &BlueskySearcher{accessToken: "test"}, `{"posts":[{"uri":"at://did:plc:1/app.bsky.feed.post/1","author":{"displayName":"Alice"},"record":{"createdAt":"2026-01-01T00:00:00Z"}}]}`, "", "", "Alice", ""},
		{"fediverse", &FediverseSearcher{instanceURLs: map[string]string{"https://one.test": "test"}}, `{"statuses":[{"url":"https://one.test/post/1","created_at":"2026-01-01T00:00:00Z","content":"<p>body</p>","account":{"url":"https://one.test/@alice","acct":"alice","display_name":"Alice"}}]}`, "https://one.test/@alice", "alice@one.test", "Alice", "body"},
		{"fediverse-remote", &FediverseSearcher{instanceURLs: map[string]string{"https://one.test": "test"}}, `{"statuses":[{"url":"https://two.test/post/1","created_at":"2026-01-01T00:00:00Z","account":{"acct":"alice@two.test"}}]}`, "", "alice@two.test", "", ""},
		{"youtube", &YouTubeSearcher{apiKey: "test"}, `{"items":[{"id":{"videoId":"1"},"snippet":{"title":"Video","publishedAt":"2026-01-01T00:00:00Z","description":"body","channelId":"channel1","channelTitle":"Alice"}}]}`, "channel1", "", "Alice", "body"},
		{"x", &XSearcher{bearerToken: "test"}, `{"data":[{"id":"1","text":"body","created_at":"2026-01-01T00:00:00Z","author_id":"account1"}],"includes":{"users":[{"id":"account1","username":"alice","name":"Alice"}]}}`, "account1", "alice", "Alice", "body"},
		{"x-no-expansion", &XSearcher{bearerToken: "test"}, `{"data":[{"id":"1","text":"body","created_at":"2026-01-01T00:00:00Z","author_id":"account1"}]}`, "account1", "", "", "body"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := http.DefaultTransport
			t.Cleanup(func() { http.DefaultTransport = original })
			calls := 0
			http.DefaultTransport = hackerNewsTransport(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})
			results, err := tc.searcher.Search("test", 0)
			if err != nil || len(results) != 1 {
				t.Fatalf("results=%v err=%v", results, err)
			}
			r := results[0]
			if r.AuthorID != tc.id || r.AuthorHandle != tc.handle || r.AuthorName != tc.nameLabel || r.Content != tc.content {
				t.Fatalf("unexpected metadata: %+v", r)
			}
			if calls != 1 {
				t.Fatalf("made %d requests; expected only the existing search", calls)
			}
			if r.Keyword != "test" || r.Platform != tc.searcher.Platform() || r.URL == "" || r.Timestamp == 0 {
				t.Fatalf("lost existing fields: %+v", r)
			}
		})
	}
}

package search

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestXSearcherSearchPaginatesAndFilters(t *testing.T) {
	after := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	var requests []url.Values

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q, want Bearer test-token", got)
		}
		requests = append(requests, r.URL.Query())

		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("next_token") == "page-2" {
			_, _ = w.Write([]byte(`{
				"data":[
					{"id":"3","text":"newer post","created_at":"2026-08-30T12:02:00Z","author_id":"2"},
					{"id":"4","text":"watermark post","created_at":"2026-08-30T12:00:00Z","author_id":"2"}
				],
				"includes":{"users":[{"id":"2","name":"Grace Hopper","username":"grace"}]},
				"meta":{}
			}`))
			return
		}

		_, _ = w.Write([]byte(`{
			"data":[{"id":"1","text":"first post","created_at":"2026-08-30T12:01:00Z","author_id":"1"}],
			"includes":{"users":[{"id":"1","name":"Ada Lovelace","username":"ada"}]},
			"meta":{"next_token":"page-2"}
		}`))
	}))
	defer server.Close()

	searcher := &XSearcher{bearerToken: "test-token", client: server.Client(), searchURL: server.URL}
	results, err := searcher.Search("golang -is:retweet", after.Unix())
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}
	if got := requests[0].Get("query"); got != "golang -is:retweet" {
		t.Errorf("query = %q", got)
	}
	if got := requests[0].Get("max_results"); got != "100" {
		t.Errorf("max_results = %q, want 100", got)
	}
	if got := requests[0].Get("post.fields"); got != "created_at,author_id" {
		t.Errorf("post.fields = %q", got)
	}
	if got := requests[1].Get("next_token"); got != "page-2" {
		t.Errorf("next_token = %q, want page-2", got)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}

	if got, want := results[0].URL, "https://x.com/ada/status/1"; got != want {
		t.Errorf("first URL = %q, want %q", got, want)
	}
	if got, want := results[0].Title, "Post by Ada Lovelace (@ada)"; got != want {
		t.Errorf("first Title = %q, want %q", got, want)
	}
	if got, want := results[1].Content, "newer post"; got != want {
		t.Errorf("second Content = %q, want %q", got, want)
	}
}

func TestXSearcherSearchUsesStartTimeOnlyWithinRecentWindow(t *testing.T) {
	after := time.Now().Add(-time.Hour).Truncate(time.Second)
	var startTime string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startTime = r.URL.Query().Get("start_time")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[],"meta":{}}`))
	}))
	defer server.Close()

	searcher := &XSearcher{client: server.Client(), searchURL: server.URL}
	if _, err := searcher.Search("golang", after.Unix()); err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if want := after.UTC().Format(time.RFC3339); startTime != want {
		t.Errorf("start_time = %q, want %q", startTime, want)
	}
}

func TestNewXSearcherRequiresBearerToken(t *testing.T) {
	t.Setenv("X_BEARER_TOKEN", "")
	if _, err := NewXSearcher(); err == nil {
		t.Fatal("NewXSearcher() error = nil, want missing token error")
	}
}

package search

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestYouTubeReadsEveryPage(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	calls := 0
	http.DefaultTransport = hackerNewsTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		q := r.URL.Query()
		if q.Get("maxResults") != "50" || q.Get("publishedAfter") != "1970-01-01T00:00:01Z" {
			t.Errorf("missing bounded query: %v", q)
		}
		next := `"nextPageToken":"second",`
		if calls == 2 {
			if q.Get("pageToken") != "second" {
				t.Error("lost page token")
			}
			next = ""
		}
		if calls > 2 {
			t.Fatal("unbounded pagination")
		}
		body := fmt.Sprintf(`{%s"items":[{"id":{"videoId":"%d"},"snippet":{"title":"grass","publishedAt":"2026-01-01T00:00:00Z"}}]}`, next, calls)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	rows, err := (&YouTubeSearcher{apiKey: "test"}).Search("grass", 1)
	if err != nil || len(rows) != 2 || calls != 2 {
		t.Fatalf("incomplete results: %d calls %d err %v", len(rows), calls, err)
	}
}

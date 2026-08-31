package search

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type hackerNewsTransport func(*http.Request) (*http.Response, error)

func (f hackerNewsTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestHackerNewsEncodesQuery(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	keyword := "tailscale & go+network #test"
	http.DefaultTransport = hackerNewsTransport(func(r *http.Request) (*http.Response, error) {
		if strings.ContainsAny(r.URL.RawQuery, "> #") {
			t.Errorf("unescaped query: %s", r.URL.RawQuery)
		}
		q := r.URL.Query()
		if q.Get("query") != keyword || q.Get("tags") != "(story,comment)" || q.Get("numericFilters") != "created_at_i>123" {
			t.Errorf("incorrect parameters: %v", q)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"hits":[{"objectID":"1","title":"A story","_tags":["story"]},{"objectID":"2","story_title":"A story","comment_text":"hello","_tags":["comment"]}]}`))}, nil
	})
	results, err := NewHackerNewsSearcher().Search(keyword, 123)
	if err != nil || len(results) != 2 {
		t.Fatalf("results=%v error=%v", results, err)
	}
	if results[1].Title != "Comment on: A story" || results[1].Content != "hello" {
		t.Fatal("comment mapping changed")
	}
}

func TestHackerNewsFailuresReturnErrors(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
	}{{400, `bad request`}, {200, `invalid JSON`}} {
		original := http.DefaultTransport
		http.DefaultTransport = hackerNewsTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
		})
		_, err := NewHackerNewsSearcher().Search("tailscale", 123)
		http.DefaultTransport = original
		if err == nil {
			t.Fatal("failed request must not count as successful empty search")
		}
	}
}

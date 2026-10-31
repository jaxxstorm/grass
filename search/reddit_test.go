package search

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type redditTransport func(*http.Request) (*http.Response, error)

func TestRedditSearchRequiresKeywordInPost(t *testing.T) {
	for _, tc := range []struct {
		name, title, body, url string
		created                int64
		want                   bool
	}{
		{"title", "Using Tailscale", "", "", 101, true},
		{"body", "Networking", "Try TAILSCALE for this.", "", 101, true},
		{"linked URL", "Networking", "", "https://tailscale.com/blog/example", 101, true},
		{"co-op false positive", "The Co-Op Mod has PvP!", "Link to mod: https://www.nexusmods.com/phantombladezero/mods/64", "https://i.redd.it/o199uxmg6uyh1.jpeg", 101, false},
		{"rye false positive", "Version 0.2.70 will bring some detail changes in evaluator", "https://ryelang.org/", "https://i.redd.it/whoe321wlsyh1.png", 101, false},
		{"metadata only", "Unrelated", "", "", 101, false},
		{"at cutoff", "Tailscale", "", "", 100, false},
		{"before cutoff", "Tailscale", "", "", 99, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			post := map[string]any{"title": tc.title, "selftext": tc.body, "url": tc.url, "created_utc": tc.created,
				"author": "tailscale", "permalink": "/r/tailscale/comments/example"}
			body, err := json.Marshal(map[string]any{"data": map[string]any{"children": []any{map[string]any{"data": post}}}})
			if err != nil {
				t.Fatal(err)
			}
			original := http.DefaultTransport
			t.Cleanup(func() { http.DefaultTransport = original })
			http.DefaultTransport = redditTransport(func(req *http.Request) (*http.Response, error) {
				if req.URL.Query().Get("q") != "tailscale" {
					t.Fatalf("unexpected query: %s", req.URL.RawQuery)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
			})
			results, err := (&RedditSearcher{accessToken: "test"}).Search("tailscale", 100)
			if err != nil {
				t.Fatal(err)
			}
			wantCount := 0
			if tc.want {
				wantCount = 1
			}
			if len(results) != wantCount {
				t.Fatalf("got %d results, want %d", len(results), wantCount)
			}
		})
	}
}

func (f redditTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRedditAuthenticationValidatesToken(t *testing.T) {
	for _, tc := range []struct {
		body      string
		wantError bool
	}{
		{`{"error":"invalid_grant"}`, true},
		{`{}`, true},
		{`{"access_token":" "}`, true},
		{`{"access_token":"test-token"}`, false},
	} {
		t.Run(tc.body, func(t *testing.T) {
			original := http.DefaultTransport
			t.Cleanup(func() { http.DefaultTransport = original })
			http.DefaultTransport = redditTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})
			r := &RedditSearcher{}
			if err := r.authenticate(); (err != nil) != tc.wantError {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestRedditErrorSummaryDoesNotExposeResponse(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{`{"message":"Forbidden","access_token":"secret"}`, "JSON response: forbidden"},
		{`{"error":"invalid_grant","password":"secret"}`, "JSON response: invalid_grant"},
		{`{"error":"secret"}`, "JSON response: unrecognized OAuth error (details omitted)"},
		{`<html>You've been blocked by network security. secret</html>`, "response indicates blocked by network security"},
		{`<html>secret</html>`, "HTML error page (details omitted)"},
	} {
		if got := redditResponseSummary(strings.NewReader(tc.body)); got != tc.want {
			t.Fatalf("got %q, want %q", got, tc.want)
		}
	}
}

func TestRedditRefreshTokenAuthentication(t *testing.T) {
	t.Setenv("REDDIT_CLIENT_ID", "test-client")
	t.Setenv("REDDIT_CLIENT_SECRET", "test-secret")
	t.Setenv("REDDIT_REFRESH_TOKEN", "test-refresh")
	t.Setenv("REDDIT_USERNAME", "")
	t.Setenv("REDDIT_PASSWORD", "")
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	http.DefaultTransport = redditTransport(func(req *http.Request) (*http.Response, error) {
		if err := req.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if req.Form.Get("grant_type") != "refresh_token" || req.Form.Get("refresh_token") != "test-refresh" {
			t.Fatal("expected refresh-token grant")
		}
		if req.Form.Has("username") || req.Form.Has("password") {
			t.Fatal("password credentials sent")
		}
		id, secret, ok := req.BasicAuth()
		if !ok || id != "test-client" || secret != "test-secret" {
			t.Fatal("missing client authentication")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"access_token":"test-access"}`)), Header: make(http.Header)}, nil
	})
	if _, err := NewRedditSearcher(); err != nil {
		t.Fatal(err)
	}
	// A refresh token must take precedence over any legacy password settings.
	t.Setenv("REDDIT_USERNAME", "legacy-user")
	t.Setenv("REDDIT_PASSWORD", "legacy-password")
	if _, err := NewRedditSearcher(); err != nil {
		t.Fatal(err)
	}
}

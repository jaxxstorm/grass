package search

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type redditTransport func(*http.Request) (*http.Response, error)

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

package search

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestSearchFailuresAreIncomplete(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	for _, tc := range []struct {
		name string
		code int
		body string
		err  error
	}{{"status", 500, `{}`, nil}, {"rate", 429, `{}`, nil}, {"decode", 200, `{`, nil}, {"network", 0, "", errors.New("offline")}} {
		t.Run(tc.name, func(t *testing.T) {
			http.DefaultTransport = hackerNewsTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host == "good.test" {
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"statuses":[]}`))}, nil
				}
				if tc.err != nil {
					return nil, tc.err
				}
				return &http.Response{StatusCode: tc.code, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})
			for _, s := range []Searcher{&BlueskySearcher{accessToken: "test"}, &FediverseSearcher{instanceURLs: map[string]string{"https://bad.test": "test", "https://good.test": "test"}}} {
				if _, err := s.Search("grass", 0); err == nil {
					t.Errorf("%s hid %s failure", s.Platform(), tc.name)
				}
			}
		})
	}
	if _, err := (&BlueskySearcher{}).Search("grass", 0); err == nil {
		t.Error("unauthenticated search reported complete")
	}
}

func TestFediverseInitializationRejectsMissingInstanceCredentials(t *testing.T) {
	t.Setenv("FEDIVERSE_INSTANCES", "https://missing.test")
	for _, name := range []string{"MISSING_TEST_CLIENT_ID", "MISSING_TEST_CLIENT_SECRET", "MISSING_TEST_ACCESS_TOKEN"} {
		t.Setenv(name, "")
	}
	if _, err := NewFediverseSearcher(); err == nil {
		t.Fatal("failed instance silently omitted")
	}
}

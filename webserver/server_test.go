package webserver

import (
	"context"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jaxxstorm/grass/archive"
	"github.com/jaxxstorm/grass/archivesource"
	"github.com/jaxxstorm/grass/archivesync"
	"github.com/jaxxstorm/grass/localindex"
	"github.com/jaxxstorm/grass/search"
	"github.com/jaxxstorm/grass/storage"
)

type emptyReader struct{}

func (emptyReader) Load(context.Context, storage.ArchiveLimits) ([]storage.ArchiveRecord, error) {
	return []storage.ArchiveRecord{}, nil
}

func TestLocalHTTPWithUnavailableSource(t *testing.T) {
	ctx := context.Background()
	i, err := localindex.Open(ctx, filepath.Join(t.TempDir(), "index.db"), localindex.Source{Kind: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	limits := storage.DefaultArchiveLimits()
	token, err := i.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stage := localindex.Stage{Active: true, Mode: "journal", Epoch: "epoch"}
	if err = i.BeginStage(ctx, token, stage); err != nil {
		t.Fatal(err)
	}
	rows := []storage.ArchiveRecord{{SearchResult: search.SearchResult{Platform: "X", URL: "https://example.test/one", Title: "network garden", Timestamp: 1767225600}}, {SearchResult: search.SearchResult{Platform: "X", URL: "https://example.test/two", Title: "networking garden", Timestamp: 1767225600}}}
	if err = i.StagePage(ctx, token, rows, stage, limits); err != nil {
		t.Fatal(err)
	}
	if err = i.Publish(ctx, token, limits); err != nil {
		t.Fatal(err)
	}
	if err = i.Release(ctx, token); err != nil {
		t.Fatal(err)
	}
	s := archivesync.New(ctx, i, func(context.Context) (archivesource.Source, error) { return nil, errors.New("credentials unavailable") }, "dynamodb", limits, 0)
	defer s.Close()
	deadline := time.Now().Add(time.Second)
	for s.Status().Refreshing && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	h := Handler(s, fstest.MapFS{"index.html": {Data: []byte("Grass")}})
	for _, tc := range []struct {
		method, path string
		code         int
		body         string
	}{
		{"GET", "/api/status", 200, `"Stale":true`},
		{"GET", "/api/archive?q=network", 200, `"Total":1`},
		{"GET", "/api/archive?q=net", 200, `"Total":0`},
		{"GET", "/api/archive?page=9223372036854775807&page_size=25", 400, "invalid_query"},
		{"GET", "/api/archive?snapshot=old", 409, "snapshot_changed"},
		{"POST", "/api/archive/reconcile", 429, "refresh_cooldown"},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != tc.code || !strings.Contains(w.Body.String(), tc.body) {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body.String())
		}
	}
}
func (emptyReader) Close() error { return nil }
func TestArchiveHTTP(t *testing.T) {
	s := archive.NewService(context.Background(), emptyReader{}, storage.DefaultArchiveLimits(), "fixture")
	defer s.Close()
	deadline := time.Now().Add(time.Second)
	for s.Status().Refreshing && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	h := Handler(s, fstest.MapFS{"index.html": {Data: []byte("<html>Grass</html>")}})
	for _, tc := range []struct {
		method, path string
		status       int
		contains     string
	}{
		{"GET", "/api/status", 200, "fixture"}, {"GET", "/api/archive", 200, `"Total":0`}, {"GET", "/api/archive?page_size=101", 400, "invalid_query"},
		{"GET", "/api/archive?snapshot=old", 409, "snapshot_changed"}, {"POST", "/api/archive/refresh", 429, "refresh_cooldown"},
		{"GET", "/api/unknown", 404, "not_found"}, {"GET", "/nested/route", 200, "<html>Grass"},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.contains) {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body.String())
		}
		if tc.status == 429 && w.Header().Get("Retry-After") == "" {
			t.Fatal("cooldown response lacks Retry-After")
		}
	}
}
func TestServeOptions(t *testing.T) {
	for _, args := range [][]string{{"--archive-max-records=0"}, {"--db=invalid"}, {"unexpected"}} {
		if Run(args) == nil {
			t.Fatalf("accepted invalid flags %v", args)
		}
	}
	if err := Run([]string{"--help"}); err != nil {
		t.Fatal(err)
	}
}

func TestAnalyseOptions(t *testing.T) {
	for _, args := range [][]string{
		{}, {"collect"}, {"--destination=postgres", "serve"},
		{"--db=sqlite", "serve"}, {"serve", "extra"},
		{"--destination", "sqlite", "serve", "--archive-max-records=0"},
		{"serve", "--destination=sqlite", "--archive-sync-interval=-1s"},
	} {
		if err := RunAnalyse(args); err == nil {
			t.Fatalf("accepted invalid analysis options %v", args)
		}
	}
	for _, args := range [][]string{{"--help"}, {"--destination", "sqlite", "serve", "--help"}} {
		if err := RunAnalyse(args); err != nil {
			t.Fatal(err)
		}
	}
}

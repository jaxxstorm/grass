package archive

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/smithy-go"
	"github.com/charmbracelet/log"
	"github.com/jaxxstorm/grass/search"
	"github.com/jaxxstorm/grass/storage"
)

type controlledReader struct{ loads chan chan error }

func (r *controlledReader) Load(ctx context.Context, _ storage.ArchiveLimits) ([]storage.ArchiveRecord, error) {
	result := make(chan error, 1)
	select {
	case r.loads <- result:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case err := <-result:
		return []storage.ArchiveRecord{{SearchResult: search.SearchResult{Platform: "X", URL: "https://example.test/b", Timestamp: 1}}, {SearchResult: search.SearchResult{Platform: "X", URL: "https://example.test/a", Timestamp: 1}}}, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestInitialFailureIsLoggedAndReported(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	r := &controlledReader{loads: make(chan chan error, 1)}
	s := NewService(context.Background(), r, storage.DefaultArchiveLimits(), "dynamodb")
	defer s.Close()
	(<-r.loads) <- &smithy.GenericAPIError{Code: "ProvisionedThroughputExceededException", Message: "fixture throughput exhausted"}
	waitReady(t, s)
	if _, code := s.Query(Query{Page: 1, PageSize: 25}); code != "archive_load_failed" {
		t.Fatalf("failure reported as %q", code)
	}
	st := s.Status()
	if !strings.Contains(st.Error, "throttled") || strings.Contains(st.Error, "fixture throughput") || st.RetryAt.IsZero() {
		t.Fatalf("unhelpful status: %+v", st)
	}
	if !strings.Contains(logs.String(), "Archive load failed") || !strings.Contains(logs.String(), "fixture throughput exhausted") {
		t.Fatalf("missing diagnostics: %s", logs.String())
	}
	if s.Refresh() != 429 || s.Status().Error != st.Error {
		t.Fatal("cooldown lost original failure")
	}
}
func (r *controlledReader) Close() error { return nil }
func waitReady(t *testing.T, s *Service) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if !s.Status().Refreshing {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("load did not complete")
}
func TestSnapshotRefreshLifecycle(t *testing.T) {
	r := &controlledReader{loads: make(chan chan error, 1)}
	s := NewService(context.Background(), r, storage.DefaultArchiveLimits(), "test")
	defer s.Close()
	if _, code := s.Query(Query{Page: 1, PageSize: 25}); code != "archive_loading" {
		t.Fatal(code)
	}
	first := <-r.loads
	if s.Refresh() != 202 {
		t.Fatal("load not coalesced")
	}
	first <- nil
	waitReady(t, s)
	q := Query{Page: 1, PageSize: 1}
	out, code := s.Query(q)
	if code != "" || out.Results[0].URL != "https://example.test/a" {
		t.Fatal("unstable tie ordering")
	}
	id := out.Snapshot
	if s.Refresh() != 429 {
		t.Fatal("refresh not throttled")
	}
	s.mu.Lock()
	s.lastStart = time.Time{}
	s.mu.Unlock()
	s.Refresh()
	(<-r.loads) <- errors.New("secret database details")
	waitReady(t, s)
	st := s.Status()
	if !st.Stale || st.Snapshot != id || st.Error == "secret database details" {
		t.Fatalf("bad stale state: %+v", st)
	}
	s.mu.Lock()
	s.lastStart = time.Time{}
	s.mu.Unlock()
	s.Refresh()
	(<-r.loads) <- nil
	waitReady(t, s)
	q.Snapshot = id
	if _, code := s.Query(q); code != "snapshot_changed" {
		t.Fatal("old snapshot accepted")
	}
}

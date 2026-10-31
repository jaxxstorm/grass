package archivesync

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/jaxxstorm/grass/archive"
	"github.com/jaxxstorm/grass/archivesource"
	"github.com/jaxxstorm/grass/localindex"
	"github.com/jaxxstorm/grass/search"
	"github.com/jaxxstorm/grass/storage"
)

type fixtureSource struct {
	head            archivesource.Head
	records         []storage.ArchiveRecord
	events          []storage.ArchiveRecord
	scans, changes  int
	failAt          int
	duringScan      bool
	failChanges     bool
	failChangeAfter int64
}

func TestSQLiteRepeatedImportPreservesGeneration(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "source.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`CREATE TABLE search_results(Platform TEXT,URL TEXT,Keyword TEXT,Title TEXT,Timestamp INTEGER); INSERT INTO search_results VALUES('X','https://example.test','grass','network',1767225600)`); err != nil {
		t.Fatal(err)
	}
	s, _ := directService(t, &fixtureSource{})
	s.factory = func(ctx context.Context) (archivesource.Source, error) { return archivesource.NewSQLite(ctx, path) }
	if err = s.synchronize(ctx, false); err != nil {
		t.Fatal(err)
	}
	before, _, _ := s.index.State(ctx)
	if err = s.synchronize(ctx, false); err != nil {
		t.Fatal(err)
	}
	after, _, _ := s.index.State(ctx)
	if before.Generation != after.Generation || !after.LastSync.After(before.LastSync) {
		t.Fatal("no-op import changed generation or failed to update freshness")
	}
	if _, err = db.Exec(`UPDATE search_results SET Title='changed'`); err != nil {
		t.Fatal(err)
	}
	if err = s.synchronize(ctx, false); err != nil {
		t.Fatal(err)
	}
	after, _, _ = s.index.State(ctx)
	if before.Generation == after.Generation {
		t.Fatal("changed source not published")
	}
}

func (f *fixtureSource) Head(context.Context) (archivesource.Head, error) {
	h := f.head
	h.Sequence = int64(len(f.events))
	return h, nil
}
func (f *fixtureSource) Close() error { return nil }
func (f *fixtureSource) Scan(_ context.Context, cursor string) (archivesource.Page, error) {
	f.scans++
	start, _ := strconv.Atoi(cursor)
	if f.failAt > 0 && start >= f.failAt {
		return archivesource.Page{}, errors.New("interrupted scan")
	}
	end := min(start+100, len(f.records))
	p := archivesource.Page{Records: f.records[start:end], Examined: end - start}
	if end < len(f.records) {
		p.Cursor = strconv.Itoa(end)
	}
	if f.duringScan {
		f.duringScan = false
		f.events = append(f.events, record(999))
		f.head.Mode = "journal"
	}
	return p, nil
}
func (f *fixtureSource) Changes(_ context.Context, epoch string, after, target int64) (archivesource.Page, error) {
	f.changes++
	if f.failChanges || (f.failChangeAfter > 0 && after >= f.failChangeAfter) {
		return archivesource.Page{}, errors.New("journal failure")
	}
	next := min(after+100, target)
	return archivesource.Page{Records: f.events[after:next], Sequence: next, Examined: int(next - after)}, nil
}
func record(n int) storage.ArchiveRecord {
	return storage.ArchiveRecord{SearchResult: search.SearchResult{Platform: "X", URL: fmt.Sprintf("https://example.test/%d", n), Title: "network garden", Timestamp: 1767225600}}
}
func directService(t *testing.T, f *fixtureSource) (*Service, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "index.db")
	i, err := localindex.Open(context.Background(), path, localindex.Source{Kind: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { i.Close() })
	return &Service{index: i, factory: func(context.Context) (archivesource.Source, error) { return f, nil }, limits: storage.DefaultArchiveLimits(), ctx: context.Background()}, path
}
func TestBootstrapResumeCatchupAndIncremental(t *testing.T) {
	f := &fixtureSource{head: archivesource.Head{Mode: "journal", Epoch: "epoch"}, failAt: 100, duringScan: true}
	for n := 0; n < 201; n++ {
		f.records = append(f.records, record(n))
	}
	s, path := directService(t, f)
	ctx := context.Background()
	if err := s.synchronize(ctx, false); err == nil {
		t.Fatal("expected interrupted import")
	}
	state, stage, err := s.index.State(ctx)
	if err != nil || state.Generation != "" || stage.Cursor != "100" {
		t.Fatalf("bootstrap state: %+v %+v %v", state, stage, err)
	}
	s.index.Close()
	s.index, err = localindex.Open(ctx, path, localindex.Source{Kind: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.index.Close()
	f.failAt = 0
	if err = s.synchronize(ctx, false); err != nil {
		t.Fatal(err)
	}
	state, _, _ = s.index.State(ctx)
	if state.Records != 202 || state.Sequence != 1 || f.scans != 4 {
		t.Fatalf("restart/catchup lost records: %+v scans=%d", state, f.scans)
	}
	generation := state.Generation
	scans := f.scans
	if err = s.synchronize(ctx, false); err != nil {
		t.Fatal(err)
	}
	state, _, _ = s.index.State(ctx)
	if state.Generation != generation || f.scans != scans {
		t.Fatal("no-op sync rescanned or changed generation")
	}
	f.events = append(f.events, record(1000))
	if err = s.synchronize(ctx, false); err != nil {
		t.Fatal(err)
	}
	state, _, _ = s.index.State(ctx)
	if state.Records != 203 || state.Sequence != 2 || f.scans != scans {
		t.Fatal("incremental sync incorrect")
	}
	f.records = []storage.ArchiveRecord{record(0)}
	if err = s.synchronize(ctx, true); err != nil {
		t.Fatal(err)
	}
	state, _, _ = s.index.State(ctx)
	if state.Records != 1 {
		t.Fatal("reconciliation did not remove absent rows")
	}
	f.head.Epoch = "replacement"
	if err = s.synchronize(ctx, false); err == nil {
		t.Fatal("epoch change accepted")
	}
}
func TestLegacyModeAndReconciliationFailure(t *testing.T) {
	f := &fixtureSource{head: archivesource.Head{Mode: "legacy"}, records: []storage.ArchiveRecord{record(0)}}
	s, _ := directService(t, f)
	ctx := context.Background()
	if err := s.synchronize(ctx, false); err != nil {
		t.Fatal(err)
	}
	scans := f.scans
	if err := s.synchronize(ctx, false); err == nil || f.scans != scans {
		t.Fatal("legacy refresh silently scanned")
	}
	s.limits.Records = 1
	f.records = append(f.records, record(1))
	if err := s.synchronize(ctx, true); err == nil {
		t.Fatal("over-limit reconciliation succeeded")
	}
	out, err := s.index.Query(ctx, archive.Query{Page: 1, PageSize: 25})
	if err != nil || out.Total != 1 {
		t.Fatal("failed reconciliation replaced cache")
	}
}
func TestCachedStartupDoesNotWaitForRemote(t *testing.T) {
	f := &fixtureSource{head: archivesource.Head{Mode: "journal", Epoch: "epoch"}, records: []storage.ArchiveRecord{record(0)}}
	s, path := directService(t, f)
	ctx := context.Background()
	if err := s.synchronize(ctx, false); err != nil {
		t.Fatal(err)
	}
	s.index.Close()
	i, err := localindex.Open(ctx, path, localindex.Source{Kind: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	running := New(ctx, i, func(ctx context.Context) (archivesource.Source, error) {
		close(entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}, "fixture", storage.DefaultArchiveLimits(), 0)
	<-entered
	out, code := running.Query(archive.Query{Page: 1, PageSize: 25})
	if code != "" || out.Total != 1 {
		t.Fatal("cached startup blocked on credentials")
	}
	if running.Refresh() != 202 || running.Reconcile() != 409 {
		t.Fatal("sync/reconcile not distinguished")
	}
	start := time.Now()
	running.Close()
	if time.Since(start) > time.Second {
		t.Fatal("shutdown ignored cancellation")
	}
}
func TestRepresentativeArchive(t *testing.T) {
	if testing.Short() {
		t.Skip("representative fixture")
	}
	f := &fixtureSource{head: archivesource.Head{Mode: "journal", Epoch: "epoch"}}
	for n := 0; n < 25000; n++ {
		f.records = append(f.records, record(n))
	}
	s, path := directService(t, f)
	started := time.Now()
	if err := s.synchronize(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	t.Logf("25k import: %s", time.Since(started))
	s.index.Close()
	started = time.Now()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	i, err := localindex.Open(context.Background(), path, localindex.Source{Kind: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	defer i.Close()
	out, err := i.Query(context.Background(), archive.Query{Page: 1, PageSize: 25, Text: "network"})
	if err != nil || out.Total != 25000 || len(out.Results) != 25 {
		t.Fatalf("query failed: %d %v", out.Total, err)
	}
	t.Logf("restart plus query: %s", time.Since(started))
	runtime.ReadMemStats(&after)
	t.Logf("restart/query Go heap allocations: %d bytes; returned rows: %d", after.TotalAlloc-before.TotalAlloc, len(out.Results))
}

func TestIncrementalFailureResumesCommittedPrefix(t *testing.T) {
	f := &fixtureSource{head: archivesource.Head{Mode: "journal", Epoch: "epoch"}, records: []storage.ArchiveRecord{record(0)}}
	s, _ := directService(t, f)
	ctx := context.Background()
	if err := s.synchronize(ctx, false); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 201; n++ {
		f.events = append(f.events, record(n))
	}
	f.failChangeAfter = 100
	if err := s.synchronize(ctx, false); err == nil {
		t.Fatal("expected late batch failure")
	}
	state, _, err := s.index.State(ctx)
	if err != nil || state.Sequence != 100 || state.Records != 100 {
		t.Fatalf("partial prefix not coherent: %+v %v", state, err)
	}
	f.failChangeAfter = 0
	scans := f.scans
	if err = s.synchronize(ctx, false); err != nil {
		t.Fatal(err)
	}
	state, _, _ = s.index.State(ctx)
	if state.Sequence != 201 || state.Records != 201 || f.scans != scans {
		t.Fatal("incremental retry skipped data or scanned source")
	}
}

func TestBootstrapCatchupRestartAndLegacyActivation(t *testing.T) {
	ctx := context.Background()
	f := &fixtureSource{head: archivesource.Head{Mode: "journal", Epoch: "epoch"}, records: []storage.ArchiveRecord{record(0)}, duringScan: true, failChanges: true}
	s, path := directService(t, f)
	if err := s.synchronize(ctx, false); err == nil {
		t.Fatal("catchup failure expected")
	}
	state, stage, err := s.index.State(ctx)
	if err != nil || state.Generation != "" || stage.Phase != "catchup" {
		t.Fatalf("incorrect unpublished catchup: %+v %+v %v", state, stage, err)
	}
	s.index.Close()
	s.index, err = localindex.Open(ctx, path, localindex.Source{Kind: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.index.Close()
	f.failChanges = false
	scans := f.scans
	if err = s.synchronize(ctx, false); err != nil {
		t.Fatal(err)
	}
	state, _, _ = s.index.State(ctx)
	if state.Records != 2 || state.Sequence != 1 || f.scans != scans {
		t.Fatal("catchup restart rescanned or missed events")
	}

	legacy := &fixtureSource{head: archivesource.Head{Mode: "legacy"}, records: []storage.ArchiveRecord{record(0)}}
	l, _ := directService(t, legacy)
	if err = l.synchronize(ctx, false); err != nil {
		t.Fatal(err)
	}
	legacy.head = archivesource.Head{Mode: "journal", Epoch: "activated"}
	legacy.records = append(legacy.records, record(1))
	scans = legacy.scans
	if err = l.synchronize(ctx, false); err == nil || legacy.scans != scans {
		t.Fatal("activation silently accepted without reconciliation")
	}
	if err = l.synchronize(ctx, true); err != nil {
		t.Fatal(err)
	}
	state, _, _ = l.index.State(ctx)
	if state.Mode != "journal" || state.Records != 2 {
		t.Fatal("activation reconciliation lost baseline")
	}
}

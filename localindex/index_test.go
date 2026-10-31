package localindex

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jaxxstorm/grass/archive"
	"github.com/jaxxstorm/grass/search"
	"github.com/jaxxstorm/grass/storage"
)

func TestDurableIndexQueryAndFencing(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "index.db")
	source := Source{Kind: "dynamodb", Table: "fixture"}
	i, err := Open(ctx, path, source)
	if err != nil {
		t.Fatal(err)
	}
	defer i.Close()
	token, err := i.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = i.Acquire(ctx); !errors.Is(err, ErrLease) {
		t.Fatalf("second writer acquired: %v", err)
	}
	stage := Stage{Active: true, Epoch: "epoch", Mode: "journal", Phase: "replay", Sequence: 2}
	if err = i.BeginStage(ctx, token, stage); err != nil {
		t.Fatal(err)
	}
	r := storage.ArchiveRecord{SearchResult: search.SearchResult{Platform: "X", URL: "https://example.test/1", Title: "network café", Keyword: "grass", Timestamp: 1767225600}}
	r2 := r
	r2.URL = "https://example.test/2"
	r2.Title = "networking cafe"
	r2.AuthorHandle = "alice"
	if err = i.StagePage(ctx, token, []storage.ArchiveRecord{r, r2}, stage, storage.DefaultArchiveLimits()); err != nil {
		t.Fatal(err)
	}
	q := archive.Query{Page: 1, PageSize: 1}
	if _, err = i.Query(ctx, q); !errors.Is(err, ErrNotReady) {
		t.Fatal("unpublished staging was visible")
	}
	if err = i.Publish(ctx, token, storage.DefaultArchiveLimits()); err != nil {
		t.Fatal(err)
	}
	out, err := i.Query(ctx, q)
	if err != nil || out.Total != 2 || len(out.Results) != 1 || out.Coverage.KnownAuthors != 1 || out.Keywords[0].Count != 2 {
		t.Fatalf("%+v %v", out, err)
	}
	generation := out.Snapshot
	for text, want := range map[string]int{"network": 1, "café": 1, "cafe": 1, "network café": 1, "!!!": 0, `network OR networking`: 0} {
		q.Text = text
		out, err = i.Query(ctx, q)
		if err != nil || out.Total != want {
			t.Fatalf("query %q: total=%d err=%v", text, out.Total, err)
		}
	}
	if err = i.Apply(ctx, token, "epoch", 2, 2, nil, true, storage.DefaultArchiveLimits()); err != nil {
		t.Fatal(err)
	}
	s, _, err := i.State(ctx)
	if err != nil || s.Generation != generation {
		t.Fatal("no-op changed generation")
	}
	if err = i.Release(ctx, token); err != nil {
		t.Fatal(err)
	}
	other, err := i.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = i.Apply(ctx, token, "epoch", 2, 3, nil, true, storage.DefaultArchiveLimits()); !errors.Is(err, ErrLease) {
		t.Fatal("unfenced stale writer")
	}
	limit := storage.DefaultArchiveLimits()
	limit.Records = 2
	r.URL = "https://example.test/3"
	if err = i.Apply(ctx, other, "epoch", 2, 3, []storage.ArchiveRecord{r}, true, limit); err == nil {
		t.Fatal("expected bounds rollback")
	}
	s, _, _ = i.State(ctx)
	if s.Sequence != 2 || s.Records != 2 {
		t.Fatal("failed transaction changed state")
	}
	i.Release(ctx, other)
	i.Close()
	i, err = Open(ctx, path, source)
	if err != nil {
		t.Fatal(err)
	}
	defer i.Close()
	q.Text = ""
	q.Snapshot = generation
	if out, err = i.Query(ctx, q); err != nil || out.Total != 2 {
		t.Fatalf("restart lost generation: %v", err)
	}
	if _, err = Open(ctx, path, Source{Kind: "dynamodb", Table: "other"}); err == nil {
		t.Fatal("different source accepted")
	}
	if _, err = Open(ctx, path, Source{Kind: "sqlite", Path: path}); err == nil {
		t.Fatal("source/index collision accepted")
	}
	if _, err = i.db.Exec("UPDATE writer SET token='expired',expires=?", time.Now().Add(-time.Minute).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if _, err = i.Acquire(ctx); err != nil {
		t.Fatal("expired lease cannot be taken over")
	}
}

func TestIndexWriteFailureAndSchemaProtection(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "index.db")
	source := Source{Kind: "sqlite", Path: filepath.Join(t.TempDir(), "source.db")}
	i, err := Open(ctx, path, source)
	if err != nil {
		t.Fatal(err)
	}
	defer i.Close()
	token, err := i.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stage := Stage{Active: true, Mode: "sqlite", Phase: "scan"}
	if err = i.BeginStage(ctx, token, stage); err != nil {
		t.Fatal(err)
	}
	_, err = i.db.Exec(`CREATE TRIGGER fail_write BEFORE INSERT ON staged_records BEGIN SELECT RAISE(ABORT,'simulated storage failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	stage.Cursor = "next"
	err = i.StagePage(ctx, token, []storage.ArchiveRecord{{SearchResult: search.SearchResult{Platform: "X", URL: "https://example.test"}}}, stage, storage.DefaultArchiveLimits())
	if err == nil {
		t.Fatal("injected write failure was ignored")
	}
	_, saved, err := i.State(ctx)
	if err != nil || saved.Cursor != "" {
		t.Fatal("failed write advanced continuation")
	}
	if _, err = i.db.Exec("PRAGMA user_version=999"); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(ctx, path, source); err == nil {
		t.Fatal("unsupported schema accepted")
	}
}

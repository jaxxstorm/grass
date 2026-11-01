package localindex

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jaxxstorm/grass/archive"
	"github.com/jaxxstorm/grass/search"
	"github.com/jaxxstorm/grass/storage"
)

func TestSQLFiltersAggregatesAndReadGeneration(t *testing.T) {
	ctx := context.Background()
	i, err := Open(ctx, filepath.Join(t.TempDir(), "archive.db"), Source{Kind: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	defer i.Close()
	token, err := i.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stage := Stage{Active: true, Epoch: "epoch", Mode: "journal", Sequence: 200}
	if err = i.BeginStage(ctx, token, stage); err != nil {
		t.Fatal(err)
	}
	items := make([]storage.ArchiveRecord, 200)
	for n := range items {
		r := search.SearchResult{Platform: "X", URL: fmt.Sprintf("https://example.test/%03d", n), Title: "network network garden", Keyword: "grass", Timestamp: 1767225600}
		if n%2 == 0 {
			r.Platform = "Reddit"
		}
		if n < 100 {
			r.Content = "garden garden"
			r.AuthorHandle = "alice"
		}
		items[n] = storage.ArchiveRecord{SearchResult: r}
	}
	limits := storage.DefaultArchiveLimits()
	if err = i.StagePage(ctx, token, items, stage, limits); err != nil {
		t.Fatal(err)
	}
	if err = i.Publish(ctx, token, limits); err != nil {
		t.Fatal(err)
	}
	q := archive.Query{Page: 2, PageSize: 25}
	out, err := i.Query(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	if out.Total != 200 || len(out.Results) != 25 || out.Terms[0].Count != 200 || out.Coverage.WithContent != 100 || len(out.Authors) != 2 || out.Authors[0].Count != 50 {
		t.Fatalf("aggregates do not span pages: %+v", out)
	}
	if out.Results[0].URL != "https://example.test/050" {
		t.Fatalf("unstable sort: %+v", out.Results[0])
	}
	q.Page = 1
	q.Platforms = []string{"X", "Reddit"}
	q.Keywords = []string{"grass"}
	q.Author = items[0].SearchResult.AuthorKey()
	q.Term = "garden"
	out, err = i.Query(ctx, q)
	if err != nil || out.Total != 50 {
		t.Fatalf("combined filters: %d %v", out.Total, err)
	}
	q.From = time.Unix(1767225600, 0)
	q.To = q.From.Add(time.Second)
	out, err = i.Query(ctx, q)
	if err != nil || out.Total != 50 {
		t.Fatal("inclusive lower bound failed")
	}
	q.From = q.From.Add(time.Nanosecond)
	out, err = i.Query(ctx, q)
	if err != nil || out.Total != 0 {
		t.Fatal("nanosecond lower bound included midnight")
	}
	q.To = q.From
	q.From = time.Time{}
	out, err = i.Query(ctx, q)
	if err != nil || out.Total != 50 {
		t.Fatal("nanosecond upper bound excluded midnight")
	}
	q.From = time.Unix(1767225600, 0)
	q.To = q.From
	out, err = i.Query(ctx, q)
	if err != nil || out.Total != 0 || out.Results == nil || out.Activity == nil {
		t.Fatal("exclusive upper bound/empty arrays failed")
	}
	for _, text := range []string{strings.Repeat("a", 257)} {
		q = archive.Query{Page: 1, PageSize: 25, Text: text}
		if _, err = i.Query(ctx, q); err == nil {
			t.Fatal("oversized query accepted")
		}
	}
	q = archive.Query{Page: 1, PageSize: 25}
	out, err = i.Query(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	oldGeneration := out.Snapshot
	read, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer read.Rollback()
	old, _, err := decodeState(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	changed := items[0]
	changed.Title = "changed title"
	if err = i.Apply(ctx, token, "epoch", 200, 201, []storage.ArchiveRecord{changed}, true, limits); err != nil {
		t.Fatal(err)
	}
	var title string
	if err = read.QueryRow("SELECT title FROM records WHERE platform=? AND url=?", changed.Platform, changed.URL).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "network network garden" || old.Generation != oldGeneration {
		t.Fatal("reader observed mixed generations")
	}
	q.Snapshot = oldGeneration
	if _, err = i.Query(ctx, q); !errors.Is(err, ErrSnapshot) {
		t.Fatalf("old generation accepted: %v", err)
	}
	q.Snapshot = ""
	q.Text = "changed"
	out, err = i.Query(ctx, q)
	if err != nil || out.Total != 1 {
		t.Fatal("FTS update inconsistent")
	}
	changed.Timestamp = 1 << 62
	if err = i.Apply(ctx, token, "epoch", 201, 202, []storage.ArchiveRecord{changed}, true, limits); err != nil {
		t.Fatal(err)
	}
	q.Text = ""
	if _, err = i.Query(ctx, q); err == nil {
		t.Fatal("unbounded date expansion accepted")
	}
}

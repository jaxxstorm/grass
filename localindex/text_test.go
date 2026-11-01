package localindex

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jaxxstorm/grass/archive"
	"github.com/jaxxstorm/grass/search"
	"github.com/jaxxstorm/grass/storage"
)

func TestIndexPreservesLiteralTextAndTerms(t *testing.T) {
	ctx := context.Background()
	i, err := Open(ctx, filepath.Join(t.TempDir(), "index.db"), Source{Kind: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	defer i.Close()
	token, err := i.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stage := Stage{Active: true, Mode: "journal", Epoch: "epoch", Phase: "catchup"}
	if err = i.BeginStage(ctx, token, stage); err != nil {
		t.Fatal(err)
	}
	const title = "Why 1 < 2 and 3 > 2"
	const content = "Use <Widget> when lower < middle > upper &amp; code"
	var records []storage.ArchiveRecord
	for _, platform := range []string{"Reddit", "Bluesky", "Fediverse", "HackerNews"} {
		body := content
		if platform == "HackerNews" {
			body = "<p>Use &lt;Widget&gt; when lower &lt; middle &gt; upper &amp;amp; code</p>"
		}
		records = append(records, storage.ArchiveRecord{SearchResult: search.SearchResult{Platform: platform, URL: "https://example.test/post", Title: title, Content: body}})
	}
	if err = i.StagePage(ctx, token, records, stage, storage.DefaultArchiveLimits()); err != nil {
		t.Fatal(err)
	}
	if err = i.Publish(ctx, token, storage.DefaultArchiveLimits()); err != nil {
		t.Fatal(err)
	}
	for _, q := range []archive.Query{
		{Page: 1, PageSize: 25, Text: "middle"},
		{Page: 1, PageSize: 25, Term: "widget"},
	} {
		out, err := i.Query(ctx, q)
		if err != nil || out.Total != len(records) {
			t.Fatalf("literal text missing from search: total=%d err=%v", out.Total, err)
		}
		for _, r := range out.Results {
			if r.Title != title || r.Content != content {
				t.Fatalf("stored text corrupted: %+v", r)
			}
		}
	}
}

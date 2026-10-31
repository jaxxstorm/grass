package storage

import (
	"testing"

	"github.com/jaxxstorm/grass/search"
)

func TestArchiveBufferPreservesPlainText(t *testing.T) {
	for _, platform := range []string{"Reddit", "Bluesky", "X", "YouTube", "Fediverse", "HackerNews"} {
		t.Run(platform, func(t *testing.T) {
			plain := "Why 1 < 2 and 3 > 2; use <Widget> &amp; code"
			content, wantContent := plain, plain
			if platform == "HackerNews" {
				content = "<p>Why 1 &lt; 2 and 3 &gt; 2; use &lt;Widget&gt; &amp;amp; code</p><script>hidden</script>"
			}
			b := newArchiveBuffer(DefaultArchiveLimits())
			if err := b.add(search.SearchResult{Platform: platform, URL: "https://example.test/post", Title: plain, Content: content, AuthorName: "Alice <alice>"}); err != nil {
				t.Fatal(err)
			}
			got := b.records[0]
			if got.Title != plain || got.Content != wantContent || got.AuthorName != "Alice <alice>" {
				t.Fatalf("source text corrupted: %+v", got)
			}
		})
	}
}

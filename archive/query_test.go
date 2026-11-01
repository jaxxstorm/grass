package archive

import (
	"net/url"
	"testing"
	"time"

	"github.com/jaxxstorm/grass/search"
	"github.com/jaxxstorm/grass/storage"
)

func TestQueryWholeSetAndCoverage(t *testing.T) {
	records := []storage.ArchiveRecord{}
	for i := 0; i < 200; i++ {
		r := search.SearchResult{Platform: "Reddit", Keyword: "grass", Title: "Networking networking <b>garden</b>", Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Unix()}
		if i < 100 {
			r.AuthorHandle = "alice"
			r.Content = "Networking https://example.test/ignored"
		}
		records = append(records, storage.ArchiveRecord{SearchResult: r, AuthorKey: r.AuthorKey()})
	}
	q, err := ParseQuery(url.Values{"page": {"2"}})
	if err != nil {
		t.Fatal(err)
	}
	r := Evaluate(records, q)
	if r.Total != 200 || len(r.Results) != 25 || r.Keywords[0].Count != 200 || r.Coverage.KnownAuthors != 100 || r.Coverage.WithContent != 100 || r.Terms[0].Count != 200 {
		t.Fatalf("bad aggregates: %+v", r)
	}
	q.Term = "garden"
	q.Keywords = []string{"missing"}
	r = Evaluate(records, q)
	if r.Total != 0 || len(r.Activity) != 0 || r.Results == nil || r.Authors == nil {
		t.Fatal("incoherent empty response")
	}
}
func TestQueryFiltersAndValidation(t *testing.T) {
	for _, v := range []url.Values{{"page": {"0"}}, {"page_size": {"101"}}, {"from": {"not-a-date"}}, {"from": {"2026-02-01T00:00:00Z"}, "to": {"2026-01-01T00:00:00Z"}}, {"term": {"two terms"}}} {
		if _, err := ParseQuery(v); err == nil {
			t.Fatalf("accepted invalid query %v", v)
		}
	}
	tokens := Tokens("Green green GREEN 123 the https://example.test/secret café café")
	if len(tokens) != 2 || !tokens["green"] || !tokens["café"] {
		t.Fatalf("bad tokens: %v", tokens)
	}
	r := search.SearchResult{Platform: "Reddit", Title: "hello .* world", AuthorHandle: "alice", Timestamp: 100}
	q, _ := ParseQuery(url.Values{"q": {".* world"}, "platform": {"X", "Reddit"}, "from": {"1970-01-01T00:01:40Z"}, "to": {"1970-01-01T00:01:41Z"}})
	if Evaluate([]storage.ArchiveRecord{{SearchResult: r}}, q).Total != 1 {
		t.Fatal("literal AND / inclusive from / platform OR failed")
	}
	q.To = time.Unix(100, 0)
	if Evaluate([]storage.ArchiveRecord{{SearchResult: r}}, q).Total != 0 {
		t.Fatal("to not exclusive")
	}
}

func TestTokensPreserveLiteralAngleBrackets(t *testing.T) {
	tokens := Tokens("Use <Widget> when lower < middle > upper")
	for _, word := range []string{"widget", "lower", "middle", "upper"} {
		if !tokens[word] {
			t.Fatalf("lost literal token %q: %v", word, tokens)
		}
	}
}

func TestAuthorRankingAndDailyGaps(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	records := []storage.ArchiveRecord{}
	for i, platform := range []string{"Reddit", "X", "Reddit"} {
		r := search.SearchResult{Platform: platform, AuthorHandle: "alice", Timestamp: base.AddDate(0, 0, i*2).Unix()}
		records = append(records, storage.ArchiveRecord{SearchResult: r, AuthorKey: r.AuthorKey()})
	}
	out := Evaluate(records, Query{Page: 1, PageSize: 25})
	if len(out.Authors) != 2 || out.Authors[0].Platform != "Reddit" || out.Authors[0].Count != 2 || len(out.Activity) != 5 || out.Activity[1].Count != 0 {
		t.Fatalf("bad ranking/gaps: %+v", out)
	}
	for i := range records {
		records[i].AuthorKey = ""
		records[i].AuthorHandle = ""
	}
	out = Evaluate(records, Query{Page: 1, PageSize: 25})
	if len(out.Authors) != 0 || out.Coverage.UnknownAuthors != 3 {
		t.Fatal("legacy authors inferred")
	}
}

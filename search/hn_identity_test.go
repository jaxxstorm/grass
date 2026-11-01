package search

import "testing"

func TestHackerNewsCaseSensitiveAuthor(t *testing.T) {
	a := SearchResult{Platform: "HackerNews", AuthorHandle: "Alice"}
	b := SearchResult{Platform: "HackerNews", AuthorHandle: "alice"}
	if a.AuthorKey() == b.AuthorKey() {
		t.Fatal("distinct HN accounts merged")
	}
	a.Platform = "Reddit"
	b.Platform = "Reddit"
	if a.AuthorKey() != b.AuthorKey() {
		t.Fatal("case insensitive platform changed")
	}
}

package search

import "testing"

func TestAuthorIdentity(t *testing.T) {
	a := SearchResult{Platform: "Reddit", AuthorHandle: " Alice "}
	b := SearchResult{Platform: "Reddit", AuthorHandle: "alice"}
	if a.AuthorKey() == "" || a.AuthorKey() != b.AuthorKey() {
		t.Fatal("handle normalization failed")
	}
	b.Platform = "HackerNews"
	if a.AuthorKey() == b.AuthorKey() {
		t.Fatal("platform identities merged")
	}
	for _, r := range []SearchResult{{AuthorName: "Alice"}, {Platform: "Reddit", AuthorID: "id", AuthorHandle: "[deleted]"}} {
		if r.AuthorKey() != "" {
			t.Fatal("unknown author received an identity")
		}
	}
	a.AuthorID = "stable"
	b = a
	b.AuthorHandle = "renamed"
	if a.AuthorKey() != b.AuthorKey() {
		t.Fatal("stable ID changed with handle")
	}
}

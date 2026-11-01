package bot

import (
	"github.com/jaxxstorm/grass/search"
	"testing"
)

type metadataSearcher struct {
	calls  int
	result search.SearchResult
}

func (s *metadataSearcher) Platform() string { return s.result.Platform }
func (s *metadataSearcher) Search(string, int64) ([]search.SearchResult, error) {
	s.calls++
	return []search.SearchResult{s.result}, nil
}

type metadataStore struct{ saved []search.SearchResult }

func (s *metadataStore) Exists(platform, url string) (bool, error) {
	for _, r := range s.saved {
		if r.Platform == platform && r.URL == url {
			return true, nil
		}
	}
	return false, nil
}
func (s *metadataStore) Save(r search.SearchResult) error        { s.saved = append(s.saved, r); return nil }
func (s *metadataStore) GetLastSearchTime(string) (int64, error) { return 0, nil }
func (s *metadataStore) SetLastSearchTime(string, int64) error   { return nil }

type metadataNotifier struct{ count int }

func (n *metadataNotifier) Notify(search.SearchResult) error { n.count++; return nil }
func TestMetadataDoesNotRenotifyDuplicates(t *testing.T) {
	s := &metadataSearcher{result: search.SearchResult{Platform: "Reddit", URL: "https://example.test", Timestamp: 123}}
	db, n := &metadataStore{}, &metadataNotifier{}
	b := NewBot([]search.Searcher{s}, db, []Notifier{n})
	b.Run("test")
	s.result.Content = "new body"
	s.result.AuthorID = "new identity"
	b.Run("test")
	if s.calls != 2 || len(db.saved) != 1 || n.count != 1 || db.saved[0].Timestamp != 123 || db.saved[0].Content != "" {
		t.Fatal("metadata changed collection/deduplication behavior")
	}
}

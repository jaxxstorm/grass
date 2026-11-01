package bot

import (
	"errors"
	"testing"
	"time"

	"github.com/jaxxstorm/grass/search"
	"github.com/jaxxstorm/grass/storage"
)

type keywordSearcher struct{ seen []int64 }

func (s *keywordSearcher) Platform() string { return "fixture" }
func (s *keywordSearcher) Search(keyword string, after int64) ([]search.SearchResult, error) {
	s.seen = append(s.seen, after)
	return []search.SearchResult{{Platform: s.Platform(), Keyword: keyword, URL: "https://example.test/" + keyword}}, nil
}

type checkpointStore struct {
	metadataStore
	watermark int64
	writes    int
	fail      string
	duplicate bool
}

func (s *checkpointStore) GetLastSearchTime(string) (int64, error) { return s.watermark, nil }
func (s *checkpointStore) SetLastSearchTime(_ string, v int64) error {
	s.watermark = v
	s.writes++
	return nil
}
func (s *checkpointStore) Save(r search.SearchResult) error {
	if r.Keyword == s.fail {
		return errors.New("failed transaction")
	}
	if s.duplicate {
		return storage.ErrAlreadyExists
	}
	return s.metadataStore.Save(r)
}
func TestKeywordsShareCheckpointAndRetainItOnFailure(t *testing.T) {
	s := &keywordSearcher{}
	db := &checkpointStore{watermark: 123, fail: "first"}
	n := &metadataNotifier{}
	b := NewBot([]search.Searcher{s}, db, []Notifier{n})
	b.RunKeywords([]string{"first", "second"})
	if db.watermark != 123 || db.writes != 0 || len(s.seen) != 2 || s.seen[0] != 123 || s.seen[1] != 123 || n.count != 1 {
		t.Fatalf("watermark advanced past failure: %+v %+v", db, s)
	}
	db.fail = ""
	before := time.Now().Unix()
	b.RunKeywords([]string{"first", "second"})
	if db.writes != 1 || db.watermark < before || db.watermark > time.Now().Unix() || n.count != 2 {
		t.Fatal("retry lost data or renotified saved result")
	}
}
func TestConditionalDuplicateDoesNotNotify(t *testing.T) {
	db := &checkpointStore{watermark: 123, duplicate: true}
	n := &metadataNotifier{}
	NewBot([]search.Searcher{&keywordSearcher{}}, db, []Notifier{n}).RunKeywords([]string{"first", "second"})
	if n.count != 0 || db.writes != 1 {
		t.Fatal("conditional duplicate notified or prevented completed run")
	}
}

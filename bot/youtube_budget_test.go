package bot

import (
	"errors"
	"testing"
	"time"

	"github.com/jaxxstorm/grass/search"
	"github.com/jaxxstorm/grass/storage"
)

type budgetStore struct {
	storage.Storer
	allowed    bool
	err        error
	watermarks int
}

func (s *budgetStore) GetLastSearchTime(string) (int64, error)              { return 0, nil }
func (s *budgetStore) SetLastSearchTime(string, int64) error                { s.watermarks++; return nil }
func (s *budgetStore) ReserveYouTubeSearch(string, time.Time) (bool, error) { return s.allowed, s.err }

type countingSearcher struct {
	platform string
	calls    int
	err      error
}

func (s *countingSearcher) Platform() string { return s.platform }
func (s *countingSearcher) Search(string, int64) ([]search.SearchResult, error) {
	s.calls++
	return nil, s.err
}

func TestYouTubeBudgetSkipDoesNotAdvanceCursorOrBlockOtherProviders(t *testing.T) {
	for _, err := range []error{nil, errors.New("storage unavailable")} {
		store := &budgetStore{err: err}
		youtube := &countingSearcher{platform: "YouTube"}
		other := &countingSearcher{platform: "Other"}
		NewBot([]search.Searcher{youtube, other}, store, nil).Run("tailscale")
		if youtube.calls != 0 || other.calls != 1 || store.watermarks != 1 {
			t.Fatal("skipped search ran, advanced cursor, or blocked other providers")
		}
	}
}

func TestYouTubeFailedRequestStillUsesReservation(t *testing.T) {
	store := &budgetStore{allowed: true}
	youtube := &countingSearcher{platform: "YouTube", err: errors.New("429")}
	NewBot([]search.Searcher{youtube}, store, nil).Run("tailscale")
	if youtube.calls != 1 || store.watermarks != 0 {
		t.Fatal("failed search should not advance cursor")
	}
	// The production stores commit the reservation before the HTTP request.
	store.allowed = false
	NewBot([]search.Searcher{youtube}, store, nil).Run("tailscale")
	if youtube.calls != 1 {
		t.Fatal("cooldown must also apply after failure")
	}
}

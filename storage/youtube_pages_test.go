package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func TestYouTubeContinuationSharesDailyCapWithoutCooldown(t *testing.T) {
	s, err := NewSQLiteStorer(filepath.Join(t.TempDir(), "budget"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	pages, ok := any(s).(interface{ ReserveYouTubePage(time.Time) (bool, error) })
	if !ok {
		t.Fatal("storage cannot reserve continuation pages")
	}
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	if ok, err := s.ReserveYouTubeSearch("grass", now); err != nil || !ok {
		t.Fatal("first request", ok, err)
	}
	for n := 1; n < YouTubeDailyLimit; n++ {
		if ok, err := pages.ReserveYouTubePage(now); err != nil || !ok {
			t.Fatalf("page %d: %v %v", n, ok, err)
		}
	}
	if ok, err := pages.ReserveYouTubePage(now); err != nil || ok {
		t.Fatal("exceeded daily cap", ok, err)
	}
	if ok, err := s.ReserveYouTubeSearch("other", now); err != nil || ok {
		t.Fatal("pages did not share search budget", ok, err)
	}
}

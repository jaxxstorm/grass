package storage

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestYouTubeBudgetPersistsAndCapsRequests(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state")
	s, err := NewSQLiteStorer(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	reserve := func(keyword string, at time.Time, want bool) {
		t.Helper()
		got, err := s.ReserveYouTubeSearch(keyword, at)
		if err != nil || got != want {
			t.Fatalf("reserve %q: %v, %v; want %v", keyword, got, err, want)
		}
	}
	reserve("tailscale", now, true)
	reserve("tailscale", now.Add(5*time.Minute), false)
	// Simulate a process restart: cooldown and daily count must survive.
	s.Close()
	s, err = NewSQLiteStorer(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	reserve("tailscale", now.Add(59*time.Minute), false)
	reserve("tailscale", now.Add(time.Hour), true)
	for i := 2; i < YouTubeDailyLimit; i++ {
		reserve(fmt.Sprintf("keyword-%d", i), now.Add(time.Hour), true)
	}
	reserve("over-budget", now.Add(time.Hour), false)
	reserve("over-budget", now.Add(24*time.Hour), true)
}

func TestYouTubeBudgetPacificResetAndDST(t *testing.T) {
	for _, date := range []string{"2026-03-08", "2026-11-01"} {
		zone, err := time.LoadLocation("America/Los_Angeles")
		if err != nil {
			t.Fatal(err)
		}
		midnight, err := time.ParseInLocation("2006-01-02", date, zone)
		if err != nil {
			t.Fatal(err)
		}
		before, _, err := youtubeBudgetKeys("test", midnight.Add(-time.Second))
		if err != nil {
			t.Fatal(err)
		}
		after, _, err := youtubeBudgetKeys("test", midnight)
		if err != nil {
			t.Fatal(err)
		}
		if before == after || after != "day:"+date {
			t.Fatalf("wrong Pacific reset: %s %s", before, after)
		}
	}
}

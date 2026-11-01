package bot

import (
	"fmt"
	"github.com/jaxxstorm/grass/search"
	"github.com/jaxxstorm/grass/storage"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type youtubeTransport func(*http.Request) (*http.Response, error)

func (f youtubeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestYouTubePaginationBudgetAndWatermark(t *testing.T) {
	for _, tc := range []struct {
		name    string
		prefill int
		status  int
		calls   int
		advance bool
	}{{"complete", 0, 200, 2, true}, {"exhausted", storage.YouTubeDailyLimit - 1, 200, 1, false}, {"second-page-error", 0, 500, 2, false}} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := storage.NewSQLiteStorer(filepath.Join(t.TempDir(), "state"))
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if err := s.SetLastSearchTime("YouTube", 123); err != nil {
				t.Fatal(err)
			}
			for n := 0; n < tc.prefill; n++ {
				if ok, err := s.ReserveYouTubePage(time.Now()); err != nil || !ok {
					t.Fatal(ok, err)
				}
			}
			t.Setenv("YOUTUBE_API_KEY", "test")
			provider, err := search.NewYouTubeSearcher()
			if err != nil {
				t.Fatal(err)
			}
			original := http.DefaultTransport
			t.Cleanup(func() { http.DefaultTransport = original })
			calls := 0
			http.DefaultTransport = youtubeTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				next := `"nextPageToken":"second",`
				code := 200
				if calls == 2 {
					next = ""
					code = tc.status
				}
				if calls > 2 {
					t.Fatal("unexpected request")
				}
				body := fmt.Sprintf(`{%s"items":[{"id":{"videoId":"%d"},"snippet":{"title":"grass","publishedAt":"2026-01-01T00:00:00Z"}}]}`, next, calls)
				return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			NewBot([]search.Searcher{provider}, s, nil).Run("grass")
			watermark, err := s.GetLastSearchTime("YouTube")
			if err != nil {
				t.Fatal(err)
			}
			if calls != tc.calls || (watermark > 123) != tc.advance {
				t.Fatalf("calls=%d watermark=%d", calls, watermark)
			}
			if tc.advance {
				for _, id := range []string{"1", "2"} {
					if exists, err := s.Exists("YouTube", "https://www.youtube.com/watch?v="+id); err != nil || !exists {
						t.Fatal("missing page result", id, err)
					}
				}
			}
		})
	}
}

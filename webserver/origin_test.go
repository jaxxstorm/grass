package webserver

import (
	"github.com/jaxxstorm/grass/archive"
	"net/http/httptest"
	"testing"
)

type originService struct{ calls int }

func (s *originService) Status() archive.Status { return archive.Status{} }
func (s *originService) Query(archive.Query) (archive.Response, string) {
	return archive.Response{}, ""
}
func (s *originService) Refresh() int   { s.calls++; return 202 }
func (s *originService) Reconcile() int { s.calls++; return 202 }
func TestCrossOriginMutations(t *testing.T) {
	for _, path := range []string{"refresh", "reconcile"} {
		for _, tc := range []struct {
			origin, site string
			code         int
		}{{"https://evil.test", "", 403}, {"", "cross-site", 403}, {"http://localhost:8080", "", 202}, {"", "same-origin", 202}, {"", "", 202}} {
			s := &originService{}
			r := httptest.NewRequest("POST", "http://localhost:8080/api/archive/"+path, nil)
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("Sec-Fetch-Site", tc.site)
			w := httptest.NewRecorder()
			Handler(s, nil).ServeHTTP(w, r)
			if w.Code != tc.code {
				t.Errorf("%s %+v: status %d", path, tc, w.Code)
			}
			if tc.code == 403 && s.calls != 0 {
				t.Fatal("cross-origin request triggered synchronization")
			}
		}
	}
}

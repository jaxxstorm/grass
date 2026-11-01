// search/search.go
package search

import (
	"encoding/base64"
	"strings"
)

type SearchResult struct {
	Platform     string
	Keyword      string
	Title        string
	URL          string
	Timestamp    int64
	Content      string
	AuthorID     string
	AuthorHandle string
	AuthorName   string
}

// AuthorKey identifies an explicit account within its platform. Display names
// are labels only, and must never merge otherwise unknown authors.
func (r SearchResult) AuthorKey() string {
	if strings.EqualFold(r.Platform, "Reddit") && strings.EqualFold(strings.TrimSpace(r.AuthorHandle), "[deleted]") {
		return ""
	}
	identity := strings.TrimSpace(r.AuthorID)
	kind := "id"
	if identity == "" {
		identity = strings.TrimSpace(r.AuthorHandle)
		if !strings.EqualFold(r.Platform, "HackerNews") {
			identity = strings.ToLower(identity)
		}
		kind = "handle"
	}
	if identity == "" {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(r.Platform + "\x00" + kind + "\x00" + identity))
}

// Searcher defines the interface that all search providers must implement.
type Searcher interface {
	Search(keyword string, afterEpochSecs int64) ([]SearchResult, error)
	Platform() string
}

// search/search.go
package search

type SearchResult struct {
	Platform  string
	Keyword   string
	Title     string
	URL       string
	Timestamp int64
	Content   string
}

// Searcher defines the interface that all search providers must implement.
type Searcher interface {
	Search(keyword string, afterEpochSecs int64) ([]SearchResult, error)
	Platform() string
}

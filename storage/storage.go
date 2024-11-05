// storage/storage.go
package storage

import "github.com/jaxxstorm/grass/search"

// Storer defines the methods required for storing search results.
type Storer interface {
	Exists(platform, url string) (bool, error)
	Save(result search.SearchResult) error
	GetLastSearchTime(platform string) (int64, error)
	SetLastSearchTime(platform string, epochTime int64) error
}

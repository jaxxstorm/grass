// bot/notifier.go
package bot

import "github.com/jaxxstorm/grass/search"

// Notifier defines the interface for output mechanisms.
type Notifier interface {
	Notify(result search.SearchResult) error
}

// bot/print.go
package bot

import (
	"fmt"

	"github.com/jaxxstorm/grass/search"
)

type PrintNotifier struct{}

func NewPrintNotifier() *PrintNotifier {
	return &PrintNotifier{}
}

func (p *PrintNotifier) Notify(result search.SearchResult) error {
	fmt.Printf("Platform: %s\nKeyword: %s\nTitle: %s\nURL: %s\nTimestamp: %d\n\n",
		result.Platform, result.Keyword, result.Title, result.URL, result.Timestamp)
	return nil
}

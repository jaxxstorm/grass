// bot/bot.go
package bot

import (
	"time"

	"github.com/charmbracelet/log"
	"github.com/jaxxstorm/grass/search"
	"github.com/jaxxstorm/grass/storage"
)

type Bot struct {
	Searchers []search.Searcher
	Storer    storage.Storer
	Notifiers []Notifier
}

func NewBot(searchers []search.Searcher, storer storage.Storer, notifiers []Notifier) *Bot {
	return &Bot{
		Searchers: searchers,
		Storer:    storer,
		Notifiers: notifiers,
	}
}

func (b *Bot) Run(keyword string) {
	for _, provider := range b.Searchers {
		lastSearchTime, err := b.Storer.GetLastSearchTime(provider.Platform())
		if err != nil {
			log.Error("Error retrieving last search time", "platform", provider.Platform(), "error", err)
			continue
		}

		results, err := provider.Search(keyword, lastSearchTime)
		if err != nil {
			log.Error("Error searching platform", "platform", provider.Platform(), "error", err)
			continue
		}

		for _, result := range results {
			exists, err := b.Storer.Exists(result.Platform, result.URL)
			if err != nil {
				log.Error("Error checking existence in storage", "platform", result.Platform, "url", result.URL, "error", err)
				continue
			}

			if exists {
				log.Debug("Skipping existing result", "title", result.Title, "url", result.URL, "platform", result.Platform)
				continue
			}

			log.Info("New result", "platform", result.Platform, "title", result.Title, "url", result.URL)

			err = b.Storer.Save(result)
			if err != nil {
				log.Error("Error saving to storage", "platform", result.Platform, "title", result.Title, "url", result.URL, "error", err)
				continue
			}

			for _, notifier := range b.Notifiers {
				if err := notifier.Notify(result); err != nil {
					log.Error("Error notifying", "platform", result.Platform, "title", result.Title, "url", result.URL, "error", err)
				}
			}
		}

		if err := b.Storer.SetLastSearchTime(provider.Platform(), time.Now().Unix()); err != nil {
			log.Error("Error setting last search time", "platform", provider.Platform(), "error", err)
		}
	}
}

// bot/bot.go
package bot

import (
	"errors"
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
	b.RunKeywords([]string{keyword})
}

// RunKeywords treats all keywords as one checkpointed collection for each provider.
func (b *Bot) RunKeywords(keywords []string) {
	if len(keywords) == 0 {
		return
	}
	for _, provider := range b.Searchers {
		started := time.Now().Unix()
		lastSearchTime, err := b.Storer.GetLastSearchTime(provider.Platform())
		if err != nil {
			log.Error("Error retrieving last search time", "platform", provider.Platform(), "error", err)
			continue
		}

		complete := true
		for _, keyword := range keywords {
			if provider.Platform() == "YouTube" {
				budget, ok := b.Storer.(storage.YouTubeBudget)
				if !ok {
					log.Error("Skipping YouTube: storage does not support persistent request budgeting")
					complete = false
					continue
				}
				allowed, err := budget.ReserveYouTubeSearch(keyword, time.Now())
				if err != nil {
					log.Error("Skipping YouTube: cannot reserve request budget", "error", err)
					complete = false
					continue
				}
				if !allowed {
					log.Info("Skipping YouTube: hourly cooldown or daily request cap", "keyword", keyword)
					complete = false
					continue
				}
			}
			results, err := provider.Search(keyword, lastSearchTime)
			if err != nil {
				log.Error("Error searching platform", "platform", provider.Platform(), "error", err)
				complete = false
				continue
			}

			for _, result := range results {
				exists, err := b.Storer.Exists(result.Platform, result.URL)
				if err != nil {
					log.Error("Error checking existence in storage", "platform", result.Platform, "url", result.URL, "error", err)
					complete = false
					continue
				}

				if exists {
					log.Debug("Skipping existing result", "title", result.Title, "url", result.URL, "platform", result.Platform)
					continue
				}

				log.Info("New result", "platform", result.Platform, "title", result.Title, "url", result.URL)

				err = b.Storer.Save(result)
				if errors.Is(err, storage.ErrAlreadyExists) {
					continue
				}
				if err != nil {
					complete = false
					log.Error("Error saving to storage", "platform", result.Platform, "title", result.Title, "url", result.URL, "error", err)
					continue
				}

				for _, notifier := range b.Notifiers {
					if err := notifier.Notify(result); err != nil {
						log.Error("Error notifying", "platform", result.Platform, "title", result.Title, "url", result.URL, "error", err)
					}
				}
			}

		}
		if !complete {
			continue
		}
		if err := b.Storer.SetLastSearchTime(provider.Platform(), max(started, lastSearchTime)); err != nil {
			log.Error("Error setting last search time", "platform", provider.Platform(), "error", err)
		}
	}
}

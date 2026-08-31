package app

import (
	"fmt"

	"github.com/charmbracelet/log"
	"github.com/jaxxstorm/grass/bot"
	"github.com/jaxxstorm/grass/search"
	"github.com/jaxxstorm/grass/storage"
)

// Run initializes configured integrations and performs one search for each keyword.
func Run(config Config) error {
	if err := validateSelections(config); err != nil {
		return err
	}

	searchers, err := newSearchers(config.Searchers)
	if err != nil {
		return err
	}

	storer, closeStorer, err := newStorer(config.Database, config.TableName)
	if err != nil {
		return err
	}
	if closeStorer != nil {
		defer func() {
			if err := closeStorer(); err != nil {
				log.Printf("Failed to close SQLite storage: %v", err)
			}
		}()
	}

	notifiers, err := newNotifiers(config.Bots)
	if err != nil {
		return err
	}

	runner := bot.NewBot(searchers, storer, notifiers)
	for _, keyword := range config.Keywords {
		log.Printf("Running search for keyword: %s", keyword)
		runner.Run(keyword)
	}
	return nil
}

func newSearchers(names []string) ([]search.Searcher, error) {
	searchers := make([]search.Searcher, 0, len(names))
	for _, name := range names {
		switch name {
		case "hackernews":
			searchers = append(searchers, search.NewHackerNewsSearcher())
		case "reddit":
			searcher, err := search.NewRedditSearcher()
			if err != nil {
				return nil, fmt.Errorf("initialize Reddit searcher: %w", err)
			}
			searchers = append(searchers, searcher)
		case "bluesky":
			searcher, err := search.NewBlueskySearcher()
			if err != nil {
				return nil, fmt.Errorf("initialize Bluesky searcher: %w", err)
			}
			searchers = append(searchers, searcher)
		case "fediverse":
			searcher, err := search.NewFediverseSearcher()
			if err != nil {
				return nil, fmt.Errorf("initialize Fediverse searcher: %w", err)
			}
			searchers = append(searchers, searcher)
		case "youtube":
			searcher, err := search.NewYouTubeSearcher()
			if err != nil {
				return nil, fmt.Errorf("initialize YouTube searcher: %w", err)
			}
			searchers = append(searchers, searcher)
		case "x":
			searcher, err := search.NewXSearcher()
			if err != nil {
				return nil, fmt.Errorf("initialize X searcher: %w", err)
			}
			searchers = append(searchers, searcher)
		}
	}
	return searchers, nil
}

func newStorer(database, tableName string) (storage.Storer, func() error, error) {
	switch database {
	case DatabaseDynamoDB:
		storer, err := storage.NewDynamoDBStorer(tableName)
		if err != nil {
			return nil, nil, fmt.Errorf("initialize DynamoDB storage: %w", err)
		}
		return storer, nil, nil
	case DatabaseSQLite:
		storer, err := storage.NewSQLiteStorer(tableName)
		if err != nil {
			return nil, nil, fmt.Errorf("initialize SQLite storage: %w", err)
		}
		return storer, storer.Close, nil
	default:
		return nil, nil, fmt.Errorf("unknown database type: %s", database)
	}
}

func newNotifiers(names []string) ([]bot.Notifier, error) {
	notifiers := make([]bot.Notifier, 0, len(names))
	for _, name := range names {
		switch name {
		case "print":
			notifiers = append(notifiers, bot.NewPrintNotifier())
		case "discord":
			notifiers = append(notifiers, bot.NewDiscordNotifier())
		case "slack":
			notifiers = append(notifiers, bot.NewSlackNotifier())
		}
	}
	return notifiers, nil
}

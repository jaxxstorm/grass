package app

import (
	"fmt"
	"strings"
)

const (
	DatabaseDynamoDB = "dynamodb"
	DatabaseSQLite   = "sqlite"
)

// Config describes one Grass run.
type Config struct {
	Database  string
	TableName string
	Keywords  []string
	Bots      []string
	Searchers []string
}

// LambdaConfigFromEnv loads and validates Lambda's environment-based settings.
func LambdaConfigFromEnv(getenv func(string) string) (Config, error) {
	config := Config{
		Database:  DatabaseDynamoDB,
		TableName: strings.TrimSpace(getenv("SOCIAL_SEARCH_TABLE_NAME")),
		Keywords:  splitList(getenv("GRASS_KEYWORDS")),
		Bots:      splitList(getenv("GRASS_BOTS")),
		Searchers: splitList(getenv("GRASS_SEARCHERS")),
	}

	if config.TableName == "" {
		return Config{}, fmt.Errorf("SOCIAL_SEARCH_TABLE_NAME is required")
	}
	if len(config.Keywords) == 0 {
		return Config{}, fmt.Errorf("GRASS_KEYWORDS is required")
	}
	if len(config.Searchers) == 0 {
		return Config{}, fmt.Errorf("GRASS_SEARCHERS is required")
	}
	if len(config.Bots) == 0 {
		return Config{}, fmt.Errorf("GRASS_BOTS is required")
	}
	if err := validateSelections(config); err != nil {
		return Config{}, err
	}

	return config, nil
}

func splitList(value string) []string {
	var values []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			values = append(values, item)
		}
	}
	return values
}

func validateSelections(config Config) error {
	if config.Database != DatabaseDynamoDB && config.Database != DatabaseSQLite {
		return fmt.Errorf("unknown database type: %s", config.Database)
	}

	for _, name := range config.Searchers {
		switch name {
		case "hackernews", "reddit", "bluesky", "fediverse", "youtube", "x":
		default:
			return fmt.Errorf("unknown searcher specified: %s", name)
		}
	}

	for _, name := range config.Bots {
		switch name {
		case "print", "discord", "slack":
		default:
			return fmt.Errorf("unknown bot type: %s", name)
		}
	}

	return nil
}

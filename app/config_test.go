package app

import (
	"strings"
	"testing"
)

func TestLambdaConfigFromEnv(t *testing.T) {
	values := map[string]string{
		"SOCIAL_SEARCH_TABLE_NAME": " grass-state ",
		"GRASS_KEYWORDS":           "grass, lambda ",
		"GRASS_SEARCHERS":          "hackernews, x",
		"GRASS_BOTS":               "print",
	}

	config, err := LambdaConfigFromEnv(func(key string) string { return values[key] })
	if err != nil {
		t.Fatalf("LambdaConfigFromEnv() error = %v", err)
	}
	if config.Database != DatabaseDynamoDB {
		t.Errorf("Database = %q, want %q", config.Database, DatabaseDynamoDB)
	}
	if config.TableName != "grass-state" {
		t.Errorf("TableName = %q, want grass-state", config.TableName)
	}
	if got := strings.Join(config.Keywords, ","); got != "grass,lambda" {
		t.Errorf("Keywords = %q, want grass,lambda", got)
	}
}

func TestLambdaConfigFromEnvRequiresSettings(t *testing.T) {
	_, err := LambdaConfigFromEnv(func(string) string { return "" })
	if err == nil || !strings.Contains(err.Error(), "SOCIAL_SEARCH_TABLE_NAME") {
		t.Fatalf("LambdaConfigFromEnv() error = %v, want missing table error", err)
	}
}

func TestLambdaConfigFromEnvRejectsUnknownIntegration(t *testing.T) {
	values := map[string]string{
		"SOCIAL_SEARCH_TABLE_NAME": "grass-state",
		"GRASS_KEYWORDS":           "grass",
		"GRASS_SEARCHERS":          "unknown",
		"GRASS_BOTS":               "print",
	}

	_, err := LambdaConfigFromEnv(func(key string) string { return values[key] })
	if err == nil || !strings.Contains(err.Error(), "unknown searcher") {
		t.Fatalf("LambdaConfigFromEnv() error = %v, want invalid searcher error", err)
	}
}

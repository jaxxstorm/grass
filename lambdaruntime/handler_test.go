package lambdaruntime

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/jaxxstorm/grass/app"
)

func TestHandlerRunsConfiguredApplication(t *testing.T) {
	values := map[string]string{
		"SOCIAL_SEARCH_TABLE_NAME": "grass-state",
		"GRASS_KEYWORDS":           "grass",
		"GRASS_SEARCHERS":          "hackernews",
		"GRASS_BOTS":               "print",
	}
	called := false
	handler := Handler{
		getenv: func(key string) string { return values[key] },
		run: func(config app.Config) error {
			called = true
			if config.Database != app.DatabaseDynamoDB {
				t.Errorf("Database = %q, want DynamoDB", config.Database)
			}
			if config.TableName != "grass-state" {
				t.Errorf("TableName = %q, want grass-state", config.TableName)
			}
			return nil
		},
	}

	if err := handler.Handle(context.Background(), events.EventBridgeEvent{}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !called {
		t.Fatal("Handle() did not invoke the application runner")
	}
}

func TestHandlerRejectsInvalidConfiguration(t *testing.T) {
	called := false
	handler := Handler{
		getenv: func(string) string { return "" },
		run: func(app.Config) error {
			called = true
			return nil
		},
	}

	err := handler.Handle(context.Background(), events.EventBridgeEvent{})
	if err == nil || !strings.Contains(err.Error(), "SOCIAL_SEARCH_TABLE_NAME") {
		t.Fatalf("Handle() error = %v, want missing configuration error", err)
	}
	if called {
		t.Fatal("Handle() invoked the application runner for invalid configuration")
	}
}

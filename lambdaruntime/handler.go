package lambdaruntime

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/jaxxstorm/grass/app"
)

type runner func(app.Config) error

// Handler adapts scheduled EventBridge events to a single Grass run.
type Handler struct {
	getenv func(string) string
	run    runner
}

func NewHandler() Handler {
	return Handler{
		getenv: os.Getenv,
		run:    app.Run,
	}
}

func (h Handler) Handle(_ context.Context, _ events.EventBridgeEvent) error {
	config, err := app.LambdaConfigFromEnv(h.getenv)
	if err != nil {
		return fmt.Errorf("load Lambda configuration: %w", err)
	}
	if err := h.run(config); err != nil {
		return fmt.Errorf("run Grass: %w", err)
	}
	return nil
}

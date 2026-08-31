package lambdaruntime

import (
	"context"
	"fmt"
	"github.com/charmbracelet/log"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/jaxxstorm/grass/app"
)

type runner func(app.Config) error

// Handler adapts scheduled EventBridge events to a single Grass run.
type Handler struct {
	getenv  func(string) string
	run     runner
	acquire func(context.Context, string) (func() error, error)
}

func NewHandler() Handler {
	return Handler{
		getenv:  os.Getenv,
		run:     app.Run,
		acquire: acquireLease,
	}
}

func (h Handler) Handle(ctx context.Context, _ events.EventBridgeEvent) error {
	config, err := app.LambdaConfigFromEnv(h.getenv)
	if err != nil {
		return fmt.Errorf("load Lambda configuration: %w", err)
	}
	release, err := h.acquire(ctx, config.TableName)
	if err != nil {
		return err
	}
	if release == nil {
		log.Info("Skipping run: another invocation holds the lease")
		return nil
	}
	defer func() {
		if err := release(); err != nil {
			log.Error("Failed to release run lease; it will expire", "error", err)
		}
	}()
	if err := h.run(config); err != nil {
		return fmt.Errorf("run Grass: %w", err)
	}
	return nil
}

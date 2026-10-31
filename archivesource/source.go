// Package archivesource contains streaming, read-only archive integrations.
package archivesource

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/jaxxstorm/grass/storage"
)

var ErrJournal = errors.New("archive journal is inconsistent; reconcile the archive")

type Head struct {
	Mode, Epoch string
	Sequence    int64
}
type Page struct {
	Records  []storage.ArchiveRecord
	Cursor   string
	Examined int
	Sequence int64
}
type Source interface {
	Head(context.Context) (Head, error)
	Scan(context.Context, string) (Page, error)
	Changes(context.Context, string, int64, int64) (Page, error)
	Close() error
}

func valid(r storage.ArchiveRecord) bool {
	u, err := url.Parse(r.URL)
	return err == nil && u.Host != "" && (u.Scheme == "http" || u.Scheme == "https") && r.Platform != "" && !strings.HasPrefix(r.Platform, "__grass_")
}

type pacer struct {
	rate float64
	next time.Time
}

func (p *pacer) wait(ctx context.Context) error {
	delay := time.Until(p.next)
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
func (p *pacer) charge(start time.Time, units float64) {
	if units <= 0 {
		return
	}
	p.next = start.Add(time.Duration(units / p.rate * float64(time.Second)))
}

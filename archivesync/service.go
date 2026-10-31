// Package archivesync coordinates the durable local index and remote source.
package archivesync

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/jaxxstorm/grass/archive"
	"github.com/jaxxstorm/grass/archivesource"
	"github.com/jaxxstorm/grass/localindex"
	"github.com/jaxxstorm/grass/storage"
)

type Factory func(context.Context) (archivesource.Source, error)
type Service struct {
	mu                           sync.RWMutex
	index                        *localindex.Index
	factory                      Factory
	limits                       storage.ArchiveLimits
	source                       string
	ctx                          context.Context
	cancel                       context.CancelFunc
	wg                           sync.WaitGroup
	closed, loading, reconciling bool
	lastStart                    time.Time
	loadError, phase             string
	examined, imported           int
	target                       int64
}

func New(ctx context.Context, index *localindex.Index, factory Factory, source string, limits storage.ArchiveLimits, interval time.Duration) *Service {
	ctx, cancel := context.WithCancel(ctx)
	s := &Service{index: index, factory: factory, source: source, limits: limits, ctx: ctx, cancel: cancel}
	s.Refresh()
	if interval > 0 {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			timer := time.NewTicker(interval)
			defer timer.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-timer.C:
					s.Refresh()
				}
			}
		}()
	}
	return s
}
func (s *Service) Refresh() int   { return s.start(false) }
func (s *Service) Reconcile() int { return s.start(true) }
func (s *Service) start(reconcile bool) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 503
	}
	if s.loading {
		if reconcile != s.reconciling {
			return 409
		}
		return 202
	}
	if time.Since(s.lastStart) < time.Minute {
		return 429
	}
	s.loading = true
	s.reconciling = reconcile
	s.lastStart = time.Now()
	s.examined = 0
	s.imported = 0
	s.phase = "connecting"
	s.loadError = ""
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		started := time.Now()
		log.Info("Synchronizing local archive", "source", s.source, "index", s.index.Path, "reconcile", reconcile)
		ctx, cancel := context.WithTimeout(s.ctx, s.limits.Timeout)
		defer cancel()
		err := s.synchronize(ctx, reconcile)
		s.mu.Lock()
		defer s.mu.Unlock()
		s.loading = false
		if err != nil {
			s.phase = "error"
			s.loadError = syncError(err)
			log.Error("Archive synchronization failed", "error", err, "index", s.index.Path)
		} else {
			s.phase = "idle"
			s.loadError = ""
			log.Info("Archive synchronized", "index", s.index.Path, "duration", time.Since(started).Round(time.Millisecond))
		}
	}()
	return 202
}

type actionError string

func (e actionError) Error() string { return string(e) }
func syncError(err error) string {
	var action actionError
	var limit *storage.ArchiveLimitError
	switch {
	case errors.Is(err, archivesource.ErrJournal):
		return "Archive journal has a missing event or record, or changed identity. Reconcile archive to establish a complete baseline."
	case errors.As(err, &action):
		return string(action)
	case errors.As(err, &limit):
		return limit.Error()
	case errors.Is(err, context.DeadlineExceeded):
		return "Archive synchronization timed out. Retry to resume or increase --archive-load-timeout."
	default:
		return "Archive synchronization failed; cached results remain available. See server logs for details."
	}
}
func (s *Service) progress(phase string, examined, imported int, target int64) {
	s.mu.Lock()
	s.phase = phase
	s.examined = examined
	s.imported = imported
	s.target = target
	s.mu.Unlock()
}
func (s *Service) synchronize(ctx context.Context, reconcile bool) error {
	token, err := s.index.Acquire(ctx)
	if err != nil {
		return err
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.index.Release(c, token)
	}()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	renewed := make(chan struct{})
	defer func() { cancel(); <-renewed }()
	go func() {
		defer close(renewed)
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.index.Renew(ctx, token); err != nil {
					log.Error("Index writer lease renewal failed", "error", err)
					cancel()
					return
				}
			}
		}
	}()
	state, stage, err := s.index.State(ctx)
	if err != nil {
		return err
	}
	src, err := s.factory(ctx)
	if err != nil {
		return err
	}
	defer src.Close()
	head, err := src.Head(ctx)
	if err != nil {
		return err
	}
	bootstrap := state.Generation == "" || reconcile || head.Mode == "sqlite" || stage.Active
	if !bootstrap {
		if head.Mode == "legacy" || state.Mode == "legacy" {
			return actionError("Collector journal is unavailable or was activated after import. Deploy the journal-capable collector and select Reconcile archive to establish a complete baseline.")
		}
		if state.Epoch != head.Epoch || state.Sequence > head.Sequence {
			return actionError("Archive journal identity changed. Reconcile archive before incremental synchronization can resume.")
		}
		return s.replay(ctx, src, token, state.Epoch, state.Sequence, head.Sequence, nil)
	}
	// Reconciliation establishes a fresh baseline, even after failed catch-up.
	// A source SQLite transaction cannot survive a process restart.
	if !stage.Active || reconcile || head.Mode == "sqlite" {
		stage = localindex.Stage{Active: true, Epoch: head.Epoch, Mode: head.Mode, Sequence: head.Sequence, Phase: "scan"}
		if err = s.index.BeginStage(ctx, token, stage); err != nil {
			return err
		}
	} else if stage.Epoch != head.Epoch || stage.Mode != head.Mode {
		return actionError("Source journal changed during bootstrap. Reconcile archive to restart the import.")
	}
	lastLog := time.Now()
	imported := 0
	for stage.Phase == "scan" {
		page, err := src.Scan(ctx, stage.Cursor)
		if err != nil {
			return err
		}
		stage.Examined += page.Examined
		stage.Cursor = page.Cursor
		if page.Cursor == "" {
			stage.Phase = "catchup"
		}
		if err = s.index.StagePage(ctx, token, page.Records, stage, s.limits); err != nil {
			return err
		}
		imported += len(page.Records)
		s.progress("importing", stage.Examined, imported, 0)
		if time.Since(lastLog) >= 10*time.Second {
			log.Info("Importing local archive", "examined", stage.Examined, "imported_this_run", imported)
			lastLog = time.Now()
		}
	}
	head, err = src.Head(ctx)
	if err != nil {
		return err
	}
	if head.Epoch != stage.Epoch || head.Mode != stage.Mode {
		return actionError("Source journal changed during import. Reconcile archive to restart safely.")
	}
	stage.Target = head.Sequence
	if stage.Mode == "journal" {
		if err = s.replay(ctx, src, token, stage.Epoch, stage.Sequence, stage.Target, &stage); err != nil {
			return err
		}
	}
	check, err := src.Head(ctx)
	if err != nil {
		return err
	}
	if check.Epoch != stage.Epoch || check.Sequence < stage.Sequence {
		return actionError("Source journal changed before publication. Reconcile archive.")
	}
	s.progress("publishing", stage.Examined, imported, stage.Target)
	return s.index.Publish(ctx, token, s.limits)
}
func (s *Service) replay(ctx context.Context, src archivesource.Source, token, epoch string, after, target int64, stage *localindex.Stage) error {
	examined := 0
	for after < target {
		page, err := src.Changes(ctx, epoch, after, target)
		if err != nil {
			return fmt.Errorf("incremental journal read: %w", err)
		}
		if page.Sequence <= after || page.Sequence > target {
			return errors.New("journal did not advance a contiguous checkpoint")
		}
		examined += page.Examined
		if examined > s.limits.Scanned {
			return &storage.ArchiveLimitError{Limit: "scanned items"}
		}
		check, err := src.Head(ctx)
		if err != nil {
			return err
		}
		if check.Epoch != epoch || check.Sequence < target {
			return actionError("Journal changed during replay. Reconcile archive.")
		}
		if stage != nil {
			stage.Sequence = page.Sequence
			stage.Target = target
			stage.Examined += page.Examined
			err = s.index.StagePage(ctx, token, page.Records, *stage, s.limits)
		} else {
			err = s.index.Apply(ctx, token, epoch, after, page.Sequence, page.Records, page.Sequence == target, s.limits)
		}
		if err != nil {
			return err
		}
		after = page.Sequence
		s.progress("catchup", examined, examined, target)
	}
	if stage == nil {
		return s.index.Apply(ctx, token, epoch, after, after, nil, true, s.limits)
	}
	return nil
}
func (s *Service) Status() archive.Status {
	s.mu.RLock()
	out := archive.Status{Source: s.source, Refreshing: s.loading, Error: s.loadError, Phase: s.phase, Examined: s.examined, Imported: s.imported, Target: s.target, IndexPath: s.index.Path, RetryAt: s.lastStart.Add(time.Minute)}
	s.mu.RUnlock()
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()
	state, _, err := s.index.State(ctx)
	if err != nil {
		out.State = "error"
		out.Error = "Cannot read local archive metadata; see server logs."
		return out
	}
	out.Snapshot = state.Generation
	out.LoadedAt = state.LoadedAt
	out.LastSync = state.LastSync
	out.Checkpoint = state.Sequence
	out.SyncMode = state.Mode
	if state.Generation != "" {
		out.State = "ready"
		out.Stale = out.Error != ""
	} else if out.Error != "" {
		out.State = "error"
	} else {
		out.State = "loading"
	}
	return out
}
func (s *Service) Query(q archive.Query) (archive.Response, string) { return s.QueryContext(s.ctx, q) }
func (s *Service) QueryContext(ctx context.Context, q archive.Query) (archive.Response, string) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := s.index.Query(ctx, q)
	switch {
	case err == nil:
		return out, ""
	case errors.Is(err, localindex.ErrSnapshot):
		return out, "snapshot_changed"
	case errors.Is(err, localindex.ErrDateRange):
		return out, "archive_date_range"
	case errors.Is(err, localindex.ErrNotReady):
		if s.Status().State == "error" {
			return out, "archive_load_failed"
		}
		return out, "archive_loading"
	default:
		log.Error("Local archive query failed", "error", err)
		return out, "archive_query_failed"
	}
}
func (s *Service) Close() error {
	s.mu.Lock()
	s.closed = true
	s.cancel()
	s.mu.Unlock()
	s.wg.Wait()
	return s.index.Close()
}

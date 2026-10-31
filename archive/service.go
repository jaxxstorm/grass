package archive

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/aws/smithy-go"
	"github.com/charmbracelet/log"
	"github.com/jaxxstorm/grass/storage"
)

type Status struct {
	Source, State, Snapshot, Error string
	LoadedAt                       time.Time
	Refreshing, Stale              bool
	RetryAt                        time.Time
	IndexPath, SyncMode, Phase     string
	Examined, Imported             int
	Checkpoint, Target             int64
	LastSync                       time.Time
}
type snapshot struct {
	id      string
	loaded  time.Time
	records []storage.ArchiveRecord
}
type Service struct {
	mu        sync.RWMutex
	reader    storage.ArchiveReader
	limits    storage.ArchiveLimits
	source    string
	active    *snapshot
	loading   bool
	lastStart time.Time
	loadError string
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	closed    bool
}

func NewService(ctx context.Context, r storage.ArchiveReader, l storage.ArchiveLimits, source string) *Service {
	ctx, cancel := context.WithCancel(ctx)
	s := &Service{reader: r, limits: l, source: source, ctx: ctx, cancel: cancel}
	s.Refresh()
	return s
}
func (s *Service) Refresh() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 503
	}
	if s.loading {
		return 202
	}
	if time.Since(s.lastStart) < time.Minute {
		return 429
	}
	s.loading = true
	s.lastStart = time.Now()
	started := s.lastStart
	log.Info("Loading archive", "source", s.source)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		records, err := s.reader.Load(s.ctx, s.limits)
		if err == nil {
			sort.Slice(records, func(i, j int) bool {
				a, b := records[i], records[j]
				if a.Timestamp != b.Timestamp {
					return a.Timestamp > b.Timestamp
				}
				if a.Platform != b.Platform {
					return a.Platform < b.Platform
				}
				return a.URL < b.URL
			})
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		s.loading = false
		if err != nil {
			log.Error("Archive load failed", "source", s.source, "duration", time.Since(started).Round(time.Millisecond), "error", err)
			var limit *storage.ArchiveLimitError
			var apiErr smithy.APIError
			switch {
			case errors.As(err, &limit):
				s.loadError = limit.Error()
			case errors.Is(err, context.DeadlineExceeded):
				s.loadError = "Archive load timed out; adjust --archive-load-timeout."
			case errors.As(err, &apiErr):
				s.loadError = archiveAPIError(apiErr.ErrorCode())
			default:
				s.loadError = "Archive read failed; see server logs for details and check database access and server configuration."
			}
			return
		}
		var id [16]byte
		rand.Read(id[:])
		s.active = &snapshot{hex.EncodeToString(id[:]), time.Now().UTC(), records}
		s.loadError = ""
		log.Info("Archive loaded", "source", s.source, "records", len(records), "duration", time.Since(started).Round(time.Millisecond))
	}()
	return 202
}
func (s *Service) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st := Status{Source: s.source, State: "loading", Refreshing: s.loading, Error: s.loadError}
	if !s.lastStart.IsZero() {
		st.RetryAt = s.lastStart.Add(time.Minute)
	}
	if s.active != nil {
		st.State = "ready"
		st.Snapshot = s.active.id
		st.LoadedAt = s.active.loaded
		st.Stale = s.loadError != ""
	} else if s.loadError != "" {
		st.State = "error"
	}
	return st
}
func (s *Service) Query(q Query) (Response, string) {
	s.mu.RLock()
	active := s.active
	failed := s.loadError != "" && !s.loading
	s.mu.RUnlock()
	if active == nil {
		if failed {
			return Response{}, "archive_load_failed"
		}
		return Response{}, "archive_loading"
	}
	if q.Snapshot != "" && q.Snapshot != active.id {
		return Response{}, "snapshot_changed"
	}
	r := Evaluate(active.records, q)
	r.Snapshot = active.id
	r.LoadedAt = active.loaded
	return r, ""
}

// Keep service messages out of HTTP responses while making common AWS errors actionable.
func archiveAPIError(code string) string {
	switch code {
	case "AccessDeniedException", "AccessDenied":
		return "DynamoDB access denied. Grant dynamodb:Scan on the selected table to the active AWS identity. See server logs for details."
	case "ResourceNotFoundException":
		return "DynamoDB table not found. Check --table-name, AWS_REGION, and AWS_PROFILE. See server logs for details."
	case "ExpiredTokenException", "ExpiredToken", "UnrecognizedClientException", "InvalidClientTokenId":
		return "AWS credentials are expired or invalid. Renew credentials for the active AWS profile, then retry. See server logs for details."
	case "ProvisionedThroughputExceededException", "ThrottlingException", "RequestLimitExceeded":
		return "DynamoDB reads were throttled. Wait before retrying or check table read capacity. See server logs for details."
	default:
		return "DynamoDB archive read failed. See server logs for the AWS error details."
	}
}
func (s *Service) Close() error {
	s.mu.Lock()
	s.closed = true
	s.cancel()
	s.mu.Unlock()
	s.wg.Wait()
	return s.reader.Close()
}

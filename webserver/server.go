package webserver

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/jaxxstorm/grass/archive"
	"github.com/jaxxstorm/grass/archivesource"
	"github.com/jaxxstorm/grass/archivesync"
	"github.com/jaxxstorm/grass/localindex"
	"github.com/jaxxstorm/grass/storage"
)

//go:embed all:assets
var assets embed.FS

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func apiError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"code": code, "message": message})
}

type archiveService interface {
	Status() archive.Status
	Refresh() int
	Query(archive.Query) (archive.Response, string)
}

func Handler(s archiveService, frontend fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, s.Status()) })
	mux.HandleFunc("GET /api/archive", func(w http.ResponseWriter, r *http.Request) {
		q, err := archive.ParseQuery(r.URL.Query())
		if err != nil {
			apiError(w, 400, "invalid_query", err.Error())
			return
		}
		var out archive.Response
		var code string
		if contextual, ok := s.(interface {
			QueryContext(context.Context, archive.Query) (archive.Response, string)
		}); ok {
			out, code = contextual.QueryContext(r.Context(), q)
		} else {
			out, code = s.Query(q)
		}
		if code != "" {
			status := 503
			message := strings.ReplaceAll(code, "_", " ")
			if code == "archive_load_failed" {
				message = s.Status().Error
			}
			if code == "snapshot_changed" {
				status = 409
			}
			if code == "archive_date_range" {
				status = 400
				message = localindex.ErrDateRange.Error()
			}
			apiError(w, status, code, message)
			return
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("POST /api/archive/reconcile", func(w http.ResponseWriter, r *http.Request) {
		syncer, ok := s.(interface{ Reconcile() int })
		if !ok {
			apiError(w, 400, "reconcile_unavailable", "Local index reconciliation is not available.")
			return
		}
		status := syncer.Reconcile()
		switch status {
		case 429:
			seconds := max(1, int(time.Until(s.Status().RetryAt).Seconds())+1)
			w.Header().Set("Retry-After", fmt.Sprint(seconds))
			apiError(w, status, "refresh_cooldown", "Wait before starting another synchronization.")
		case 409:
			apiError(w, status, "sync_in_progress", "A different synchronization is running. Retry reconciliation after it finishes.")
		default:
			writeJSON(w, status, s.Status())
		}
	})
	mux.HandleFunc("POST /api/archive/refresh", func(w http.ResponseWriter, r *http.Request) {
		status := s.Refresh()
		if status == 429 {
			seconds := max(1, int(time.Until(s.Status().RetryAt).Seconds())+1)
			w.Header().Set("Retry-After", fmt.Sprint(seconds))
			apiError(w, status, "refresh_cooldown", "Wait one minute between archive loads.")
			return
		}
		if status == 409 {
			apiError(w, 409, "sync_in_progress", "Reconciliation is running. Wait for it to finish before requesting another synchronization.")
			return
		}
		writeJSON(w, status, s.Status())
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		apiError(w, 404, "not_found", "Unknown API route or method.")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" {
			http.Error(w, "Method not allowed", 405)
			return
		}
		if frontend == nil {
			http.Error(w, "Build frontend assets: npm --prefix web ci && npm --prefix web run build", 503)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path != "" {
			if f, err := frontend.Open(path); err == nil {
				f.Close()
				http.FileServer(http.FS(frontend)).ServeHTTP(w, r)
				return
			}
		}
		index, err := fs.ReadFile(frontend, "index.html")
		if err != nil {
			http.Error(w, "Frontend assets unavailable", 503)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(index)
	})
	return http.NewCrossOriginProtection().Handler(mux)
}

func Run(args []string) error {
	return run(args, false)
}

// RunAnalyse imports DynamoDB results into a local destination and serves them.
func RunAnalyse(args []string) error {
	return run(args, true)
}

func run(args []string, analyse bool) error {
	name := "grass serve"
	if analyse {
		name = "grass analyse [options] serve [options]"
	}
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	sourceKind := "dynamodb"
	db := &sourceKind
	var destination *string
	if analyse {
		destination = flags.String("destination", "sqlite", "Local analysis destination (sqlite)")
	} else {
		db = flags.String("db", "sqlite", "Archive database: sqlite or dynamodb")
	}
	defaultTable := os.Getenv("SOCIAL_SEARCH_TABLE_NAME")
	if defaultTable == "" {
		defaultTable = "grass"
	}
	table := flags.String("table-name", defaultTable, "SQLite basename or DynamoDB table")
	indexPath := flags.String("archive-index", "", "Local search index file (default: user cache directory)")
	syncInterval := flags.Duration("archive-sync-interval", 5*time.Minute, "Background sync interval; 0 disables periodic sync")
	listen := flags.String("listen", "127.0.0.1:8080", "HTTP listen address")
	limits := storage.DefaultArchiveLimits()
	flags.IntVar(&limits.Records, "archive-max-records", limits.Records, "Maximum archive records")
	flags.IntVar(&limits.Scanned, "archive-max-scanned", limits.Scanned, "Maximum examined source items")
	flags.IntVar(&limits.Bytes, "archive-max-bytes", limits.Bytes, "Maximum decoded record bytes")
	flags.DurationVar(&limits.Timeout, "archive-load-timeout", limits.Timeout, "Maximum database load duration")
	flags.Float64Var(&limits.ReadCapacity, "archive-read-capacity", limits.ReadCapacity, "DynamoDB read capacity units per second for archive scans")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		if !analyse {
			return errors.New("serve does not accept positional arguments")
		}
	}
	if analyse {
		rest := flags.Args()
		if len(rest) == 0 || rest[0] != "serve" {
			return errors.New("usage: grass analyse --destination sqlite serve")
		}
		if err := flags.Parse(rest[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("analyse serve does not accept positional arguments")
		}
		if *destination != "sqlite" {
			return errors.New("--destination must be sqlite")
		}
	}
	if *syncInterval < 0 {
		return errors.New("--archive-sync-interval must not be negative")
	}
	if err := limits.Validate(); err != nil {
		return err
	}
	if *db != "sqlite" && *db != "dynamodb" {
		return errors.New("--db must be sqlite or dynamodb")
	}
	frontend, err := fs.Sub(assets, "assets/dist")
	if err != nil {
		return err
	}
	if _, err := fs.Stat(frontend, "index.html"); err != nil {
		return errors.New("build frontend assets first: npm --prefix web ci && npm --prefix web run build")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	source := localindex.Source{Kind: *db, Table: *table}
	var dynamoConfig aws.Config
	if *db == "sqlite" {
		source.Path = *table + ".db"
	} else {
		source, dynamoConfig, err = dynamoSource(ctx, *table)
		if err != nil {
			return err
		}
	}
	index, err := localindex.Open(ctx, *indexPath, source)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		index.Close()
		return err
	}
	factory := func(c context.Context) (archivesource.Source, error) {
		if *db == "sqlite" {
			return archivesource.NewSQLite(c, source.Path)
		}
		return archivesource.NewDynamoFromConfig(dynamoConfig, *table, limits)
	}
	service := archivesync.New(ctx, index, factory, *db, limits, *syncInterval)
	defer service.Close()
	server := &http.Server{Handler: Handler(service, frontend), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	fmt.Printf("Grass archive: http://%s\n", listener.Addr())
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
}

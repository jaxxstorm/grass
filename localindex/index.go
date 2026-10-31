package localindex

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jaxxstorm/grass/archive"
	"github.com/jaxxstorm/grass/storage"
)

const schemaVersion = 1

var ErrLease = errors.New("local index sync writer is owned by another process or its lease expired")
var ErrSnapshot = errors.New("snapshot_changed")
var ErrNotReady = errors.New("archive_loading")
var ErrDateRange = errors.New("recorded date range exceeds the supported calendar or 100-year span; narrow the time filters")

type Source struct{ Kind, Profile, Region, Table, Endpoint, Path string }

func (s Source) identity() string { b, _ := json.Marshal(s); return string(b) }
func DefaultPath(s Source) (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256([]byte(s.identity()))
	return filepath.Join(dir, "grass", "archive", hex.EncodeToString(hash[:])+".db"), nil
}

type Index struct {
	writer sync.Mutex
	db     *sql.DB
	Path   string
}
type State struct {
	Generation         string
	LoadedAt, LastSync time.Time
	Epoch, Mode        string
	Sequence           int64
	Records            int
}
type Stage struct {
	Active                     bool
	Epoch, Mode, Phase, Cursor string
	Sequence, Target           int64
	Examined                   int
}

func Open(ctx context.Context, path string, source Source) (*Index, error) {
	var err error
	if source.Path != "" {
		source.Path, err = filepath.Abs(source.Path)
		if err != nil {
			return nil, err
		}
		if p, e := filepath.EvalSymlinks(source.Path); e == nil {
			source.Path = p
		}
	}
	if path == "" {
		path, err = DefaultPath(source)
		if err != nil {
			return nil, err
		}
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if p, e := filepath.EvalSymlinks(path); e == nil {
		path = p
	}
	if source.Path != "" {
		a, ae := os.Stat(path)
		b, be := os.Stat(source.Path)
		if path == source.Path || (ae == nil && be == nil && os.SameFile(a, b)) {
			return nil, errors.New("archive index must be separate from the source SQLite database")
		}
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	q := url.Values{"_pragma": {"busy_timeout(5000)", "foreign_keys(1)"}}
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	i := &Index{db: db, Path: path}
	if err = i.initialize(ctx, source.identity()); err != nil {
		db.Close()
		return nil, fmt.Errorf("open archive index %s: %w", path, err)
	}
	return i, nil
}
func (i *Index) Close() error { return i.db.Close() }
func (i *Index) initialize(ctx context.Context, source string) error {
	var version int
	if err := i.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > schemaVersion {
		return fmt.Errorf("unsupported local index schema %d", version)
	}
	if version == 0 {
		var tables int
		if err := i.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE name NOT LIKE 'sqlite_%'").Scan(&tables); err != nil {
			return err
		}
		if tables != 0 {
			return errors.New("file is not a Grass search index; choose a separate --archive-index path")
		}
	}
	if version > 0 {
		var existing string
		if err := i.db.QueryRowContext(ctx, "SELECT source FROM metadata WHERE singleton=1").Scan(&existing); err != nil {
			return err
		}
		if existing != source {
			return errors.New("index belongs to a different source; choose another --archive-index path")
		}
	}
	if _, err := i.db.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		return err
	}
	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, schema); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO metadata(singleton,source,state,stage) VALUES(1,?,'{}','{}') ON CONFLICT(singleton) DO NOTHING`, source); err != nil {
		return err
	}
	var actual string
	if err = tx.QueryRowContext(ctx, "SELECT source FROM metadata WHERE singleton=1").Scan(&actual); err != nil {
		return err
	}
	if actual != source {
		return errors.New("index source changed during initialization")
	}
	if _, err = tx.ExecContext(ctx, "PRAGMA user_version=1"); err != nil {
		return err
	}
	return tx.Commit()
}

const schema = `
CREATE TABLE IF NOT EXISTS metadata(singleton INTEGER PRIMARY KEY CHECK(singleton=1),source TEXT NOT NULL,state TEXT NOT NULL,stage TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS writer(singleton INTEGER PRIMARY KEY CHECK(singleton=1),token TEXT NOT NULL,expires INTEGER NOT NULL);
INSERT OR IGNORE INTO writer VALUES(1,'',0);
CREATE TABLE IF NOT EXISTS records(rowid INTEGER PRIMARY KEY,id TEXT NOT NULL UNIQUE,platform TEXT NOT NULL,url TEXT NOT NULL,keyword TEXT NOT NULL,title TEXT NOT NULL,timestamp INTEGER NOT NULL,content TEXT NOT NULL,author_id TEXT NOT NULL,author_handle TEXT NOT NULL,author_name TEXT NOT NULL,author_key TEXT NOT NULL,bytes INTEGER NOT NULL,UNIQUE(platform,url));
CREATE INDEX IF NOT EXISTS records_time ON records(timestamp DESC,platform,url);
CREATE INDEX IF NOT EXISTS records_platform ON records(platform);
CREATE INDEX IF NOT EXISTS records_keyword ON records(keyword);
CREATE INDEX IF NOT EXISTS records_author ON records(author_key);
CREATE VIRTUAL TABLE IF NOT EXISTS records_fts USING fts5(title,content,author_handle,author_name,content='records',content_rowid='rowid',tokenize='unicode61 remove_diacritics 0');
CREATE TRIGGER IF NOT EXISTS records_ai AFTER INSERT ON records BEGIN INSERT INTO records_fts(rowid,title,content,author_handle,author_name) VALUES(new.rowid,new.title,new.content,new.author_handle,new.author_name); END;
CREATE TRIGGER IF NOT EXISTS records_ad AFTER DELETE ON records BEGIN INSERT INTO records_fts(records_fts,rowid,title,content,author_handle,author_name) VALUES('delete',old.rowid,old.title,old.content,old.author_handle,old.author_name); END;
CREATE TRIGGER IF NOT EXISTS records_au AFTER UPDATE ON records BEGIN INSERT INTO records_fts(records_fts,rowid,title,content,author_handle,author_name) VALUES('delete',old.rowid,old.title,old.content,old.author_handle,old.author_name); INSERT INTO records_fts(rowid,title,content,author_handle,author_name) VALUES(new.rowid,new.title,new.content,new.author_handle,new.author_name); END;
CREATE TABLE IF NOT EXISTS record_terms(record_id TEXT NOT NULL REFERENCES records(id) ON DELETE CASCADE,term TEXT NOT NULL,PRIMARY KEY(record_id,term));
CREATE INDEX IF NOT EXISTS terms_term ON record_terms(term,record_id);
CREATE TABLE IF NOT EXISTS staged_records AS SELECT id,platform,url,keyword,title,timestamp,content,author_id,author_handle,author_name,author_key,bytes FROM records WHERE 0;
CREATE UNIQUE INDEX IF NOT EXISTS stage_id ON staged_records(id);
CREATE TABLE IF NOT EXISTS staged_terms(record_id TEXT NOT NULL,term TEXT NOT NULL,PRIMARY KEY(record_id,term));
`
const columns = "id,platform,url,keyword,title,timestamp,content,author_id,author_handle,author_name,author_key,bytes"

func randomID() string { var b [16]byte; rand.Read(b[:]); return hex.EncodeToString(b[:]) }
func (i *Index) Acquire(ctx context.Context) (string, error) {
	i.writer.Lock()
	defer i.writer.Unlock()
	token := randomID()
	now := time.Now()
	r, err := i.db.ExecContext(ctx, `UPDATE writer SET token=?,expires=? WHERE singleton=1 AND expires<=?`, token, now.Add(time.Minute).UnixMilli(), now.UnixMilli())
	if err != nil {
		return "", err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return "", err
	}
	if n != 1 {
		return "", ErrLease
	}
	return token, nil
}
func (i *Index) Renew(ctx context.Context, token string) error {
	i.writer.Lock()
	defer i.writer.Unlock()
	now := time.Now()
	r, err := i.db.ExecContext(ctx, `UPDATE writer SET expires=? WHERE singleton=1 AND token=? AND expires>?`, now.Add(time.Minute).UnixMilli(), token, now.UnixMilli())
	if err != nil {
		return err
	}
	n, _ := r.RowsAffected()
	if n != 1 {
		return ErrLease
	}
	return nil
}
func (i *Index) Release(ctx context.Context, token string) error {
	i.writer.Lock()
	defer i.writer.Unlock()
	_, err := i.db.ExecContext(ctx, "UPDATE writer SET expires=0 WHERE singleton=1 AND token=?", token)
	return err
}
func (i *Index) write(ctx context.Context, token string, fn func(*sql.Tx) error) error {
	i.writer.Lock()
	defer i.writer.Unlock()
	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	r, err := tx.ExecContext(ctx, "UPDATE writer SET expires=expires WHERE singleton=1 AND token=? AND expires>?", token, time.Now().UnixMilli())
	if err != nil {
		return err
	}
	n, _ := r.RowsAffected()
	if n != 1 {
		return ErrLease
	}
	if err = fn(tx); err != nil {
		return err
	}
	// The transaction holds SQLite's writer lock, so no other process can
	// take over during publication. Renew before releasing that lock even
	// when a large publication took longer than the lease interval.
	if _, err = tx.ExecContext(ctx, "UPDATE writer SET expires=? WHERE singleton=1 AND token=?", time.Now().Add(time.Minute).UnixMilli(), token); err != nil {
		return err
	}
	return tx.Commit()
}
func decodeState(ctx context.Context, tx interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}) (State, Stage, error) {
	var state State
	var stage Stage
	var a, b string
	if err := tx.QueryRowContext(ctx, "SELECT state,stage FROM metadata WHERE singleton=1").Scan(&a, &b); err != nil {
		return state, stage, err
	}
	if err := json.Unmarshal([]byte(a), &state); err != nil {
		return state, stage, err
	}
	err := json.Unmarshal([]byte(b), &stage)
	return state, stage, err
}
func (i *Index) State(ctx context.Context) (State, Stage, error) { return decodeState(ctx, i.db) }
func saveState(ctx context.Context, tx *sql.Tx, s State, stage Stage) error {
	a, _ := json.Marshal(s)
	b, _ := json.Marshal(stage)
	_, err := tx.ExecContext(ctx, "UPDATE metadata SET state=?,stage=? WHERE singleton=1", string(a), string(b))
	return err
}

func (i *Index) BeginStage(ctx context.Context, token string, stage Stage) error {
	return i.write(ctx, token, func(tx *sql.Tx) error {
		s, _, err := decodeState(ctx, tx)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "DELETE FROM staged_records; DELETE FROM staged_terms"); err != nil {
			return err
		}
		stage.Active = true
		return saveState(ctx, tx, s, stage)
	})
}

func put(ctx context.Context, tx *sql.Tx, r storage.ArchiveRecord, staging bool) (bool, error) {
	r.Title = storage.ArchivePlainText(r.Title)
	r.Content = storage.ArchivePlainText(r.Content)
	r.AuthorName = storage.ArchivePlainText(r.AuthorName)
	r.AuthorKey = r.SearchResult.AuthorKey()
	hash := sha256.Sum256([]byte(r.Platform + "\x00" + r.URL))
	r.ID = hex.EncodeToString(hash[:])
	table, terms := "records", "record_terms"
	if staging {
		table, terms = "staged_records", "staged_terms"
	}
	size := len(r.Platform) + len(r.URL) + len(r.Keyword) + len(r.Title) + len(r.Content) + len(r.AuthorID) + len(r.AuthorHandle) + len(r.AuthorName) + 8
	values := []any{r.ID, r.Platform, r.URL, r.Keyword, r.Title, r.Timestamp, r.Content, r.AuthorID, r.AuthorHandle, r.AuthorName, r.AuthorKey, size}
	query := `INSERT INTO ` + table + `(` + columns + `) VALUES(?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET keyword=excluded.keyword,title=excluded.title,timestamp=excluded.timestamp,content=excluded.content,author_id=excluded.author_id,author_handle=excluded.author_handle,author_name=excluded.author_name,author_key=excluded.author_key,bytes=excluded.bytes WHERE (keyword,title,timestamp,content,author_id,author_handle,author_name,author_key,bytes) IS NOT (excluded.keyword,excluded.title,excluded.timestamp,excluded.content,excluded.author_id,excluded.author_handle,excluded.author_name,excluded.author_key,excluded.bytes)`
	result, err := tx.ExecContext(ctx, query, values...)
	if err != nil {
		return false, err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return false, nil
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM "+terms+" WHERE record_id=?", r.ID); err != nil {
		return false, err
	}
	for term := range archive.Tokens(r.Title + " " + r.Content) {
		if _, err = tx.ExecContext(ctx, "INSERT INTO "+terms+"(record_id,term) VALUES(?,?)", r.ID, term); err != nil {
			return false, err
		}
	}
	return true, nil
}
func bounds(ctx context.Context, tx *sql.Tx, table string, l storage.ArchiveLimits) (int, error) {
	var count, size int
	if err := tx.QueryRowContext(ctx, "SELECT count(*),coalesce(sum(bytes),0) FROM "+table).Scan(&count, &size); err != nil {
		return 0, err
	}
	if count > l.Records {
		return 0, &storage.ArchiveLimitError{Limit: "records"}
	}
	if size > l.Bytes {
		return 0, &storage.ArchiveLimitError{Limit: "bytes"}
	}
	return count, nil
}
func (i *Index) StagePage(ctx context.Context, token string, rows []storage.ArchiveRecord, stage Stage, l storage.ArchiveLimits) error {
	return i.write(ctx, token, func(tx *sql.Tx) error {
		s, previous, err := decodeState(ctx, tx)
		if err != nil {
			return err
		}
		if !previous.Active {
			return errors.New("no active bootstrap")
		}
		if stage.Examined > l.Scanned {
			return &storage.ArchiveLimitError{Limit: "scanned items"}
		}
		for _, r := range rows {
			if _, err = put(ctx, tx, r, true); err != nil {
				return err
			}
		}
		if _, err = bounds(ctx, tx, "staged_records", l); err != nil {
			return err
		}
		stage.Active = true
		return saveState(ctx, tx, s, stage)
	})
}
func (i *Index) Publish(ctx context.Context, token string, l storage.ArchiveLimits) error {
	return i.write(ctx, token, func(tx *sql.Tx) error {
		s, stage, err := decodeState(ctx, tx)
		if err != nil {
			return err
		}
		if !stage.Active {
			return errors.New("no staged archive")
		}
		count, err := bounds(ctx, tx, "staged_records", l)
		if err != nil {
			return err
		}
		var different bool
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT `+columns+` FROM records EXCEPT SELECT `+columns+` FROM staged_records) OR EXISTS(SELECT `+columns+` FROM staged_records EXCEPT SELECT `+columns+` FROM records)`).Scan(&different)
		if err != nil {
			return err
		}
		if different || s.Generation == "" {
			if _, err = tx.ExecContext(ctx, `DELETE FROM records; INSERT INTO records(`+columns+`) SELECT `+columns+` FROM staged_records; INSERT INTO record_terms SELECT * FROM staged_terms`); err != nil {
				return err
			}
			s.Generation = randomID()
			s.LoadedAt = time.Now().UTC()
		}
		s.Records = count
		s.Epoch = stage.Epoch
		s.Sequence = stage.Sequence
		s.Mode = stage.Mode
		s.LastSync = time.Now().UTC()
		if _, err = tx.ExecContext(ctx, "DELETE FROM staged_records; DELETE FROM staged_terms"); err != nil {
			return err
		}
		return saveState(ctx, tx, s, Stage{})
	})
}
func (i *Index) Apply(ctx context.Context, token, epoch string, expected, next int64, rows []storage.ArchiveRecord, complete bool, l storage.ArchiveLimits) error {
	return i.write(ctx, token, func(tx *sql.Tx) error {
		s, stage, err := decodeState(ctx, tx)
		if err != nil {
			return err
		}
		if s.Generation == "" || s.Epoch != epoch || s.Sequence != expected || next < expected {
			return errors.New("local checkpoint changed; synchronization must restart")
		}
		changed := false
		for _, r := range rows {
			c, e := put(ctx, tx, r, false)
			if e != nil {
				return e
			}
			changed = changed || c
		}
		s.Records, err = bounds(ctx, tx, "records", l)
		if err != nil {
			return err
		}
		if changed {
			s.Generation = randomID()
			s.LoadedAt = time.Now().UTC()
		}
		s.Sequence = next
		if complete {
			s.LastSync = time.Now().UTC()
		}
		return saveState(ctx, tx, s, stage)
	})
}

package archivesource

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jaxxstorm/grass/storage"
	_ "modernc.org/sqlite"
)

type SQLite struct {
	db      *sql.DB
	tx      *sql.Tx
	columns string
}

func NewSQLite(ctx context.Context, path string) (*SQLite, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(abs), RawQuery: "mode=ro"}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return &SQLite{db: db}, nil
}
func (s *SQLite) Close() error {
	if s.tx != nil {
		s.tx.Rollback()
	}
	return s.db.Close()
}
func (s *SQLite) Head(context.Context) (Head, error) { return Head{Mode: "sqlite"}, nil }
func (s *SQLite) Changes(context.Context, string, int64, int64) (Page, error) {
	return Page{}, errors.New("SQLite sources require a local transactional import")
}
func (s *SQLite) Scan(ctx context.Context, cursor string) (Page, error) {
	if s.tx == nil {
		tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			return Page{}, err
		}
		s.tx = tx
		rows, err := tx.QueryContext(ctx, "PRAGMA table_info(search_results)")
		if err != nil {
			return Page{}, err
		}
		columns := map[string]bool{}
		for rows.Next() {
			var cid, nn, pk int
			var name, typ string
			var def sql.NullString
			if err = rows.Scan(&cid, &name, &typ, &nn, &def, &pk); err != nil {
				rows.Close()
				return Page{}, err
			}
			columns[strings.ToLower(name)] = true
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return Page{}, err
		}
		if !columns["url"] {
			return Page{}, errors.New("source search_results table is missing")
		}
		selects := []string{"rowid", "COALESCE(Platform,'')", "COALESCE(URL,'')", "COALESCE(Keyword,'')", "COALESCE(Title,'')", "COALESCE(Timestamp,0)"}
		for _, name := range []string{"Content", "AuthorID", "AuthorHandle", "AuthorName"} {
			if columns[strings.ToLower(name)] {
				selects = append(selects, "COALESCE("+name+",'')")
			} else {
				selects = append(selects, "''")
			}
		}
		s.columns = strings.Join(selects, ",")
	}
	var last int64
	var err error
	condition := ""
	args := []any{}
	if cursor != "" {
		last, err = strconv.ParseInt(cursor, 10, 64)
		if err != nil {
			return Page{}, err
		}
		condition = " WHERE rowid>?"
		args = append(args, last)
	}
	rows, err := s.tx.QueryContext(ctx, "SELECT "+s.columns+" FROM search_results"+condition+" ORDER BY rowid LIMIT 100", args...)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	p := Page{}
	for rows.Next() {
		var r storage.ArchiveRecord
		if err = rows.Scan(&last, &r.Platform, &r.URL, &r.Keyword, &r.Title, &r.Timestamp, &r.Content, &r.AuthorID, &r.AuthorHandle, &r.AuthorName); err != nil {
			return p, err
		}
		p.Examined++
		if valid(r) {
			p.Records = append(p.Records, r)
		}
	}
	if err = rows.Err(); err != nil {
		return p, err
	}
	if p.Examined == 100 {
		p.Cursor = fmt.Sprint(last)
	}
	return p, nil
}

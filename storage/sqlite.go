// storage/sqlite.go
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jaxxstorm/grass/search"
	_ "github.com/mattn/go-sqlite3"
)

type SQLiteStorer struct {
	db *sql.DB
}

func NewSQLiteStorer(dbPath string) (*SQLiteStorer, error) {
	db, err := sql.Open("sqlite3", fmt.Sprintf("%s.db", dbPath))
	if err != nil {
		return nil, err
	}

	// Create tables if they do not exist
	createTables := `
	CREATE TABLE IF NOT EXISTS youtube_budget (
		Key TEXT PRIMARY KEY,
		Value INTEGER NOT NULL
	);
	CREATE TABLE IF NOT EXISTS search_results (
		Platform TEXT,
		Keyword TEXT,
		Title TEXT,
		URL TEXT PRIMARY KEY,
		Timestamp INTEGER
	);
	CREATE TABLE IF NOT EXISTS last_search_time (
		Platform TEXT PRIMARY KEY,
		LastSearchTime INTEGER
	);`
	_, err = db.Exec(createTables)
	if err != nil {
		db.Close()
		return nil, err
	}

	if err := migrateArchiveMetadata(db); err != nil {
		db.Close()
		return nil, err
	}
	return &SQLiteStorer{db: db}, nil
}

// Serialize schema inspection and changes so concurrent collector startups do
// not race between detecting a missing column and adding it.
func migrateArchiveMetadata(db *sql.DB) error {
	conn, err := db.Conn(context.Background())
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err = conn.ExecContext(context.Background(), "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	defer conn.ExecContext(context.Background(), "ROLLBACK")
	rows, err := conn.QueryContext(context.Background(), "PRAGMA table_info(search_results)")
	if err != nil {
		return err
	}
	columns := map[string]bool{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var def sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &def, &pk); err != nil {
			rows.Close()
			return err
		}
		columns[strings.ToLower(name)] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, name := range []string{"Content", "AuthorID", "AuthorHandle", "AuthorName"} {
		if !columns[strings.ToLower(name)] {
			if _, err := conn.ExecContext(context.Background(), "ALTER TABLE search_results ADD COLUMN "+name+" TEXT"); err != nil {
				return err
			}
		}
	}
	_, err = conn.ExecContext(context.Background(), "COMMIT")
	return err
}

// Exists checks if a specific item already exists in SQLite.
func (s *SQLiteStorer) Exists(platform, url string) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM search_results WHERE Platform = ? AND URL = ?);`
	err := s.db.QueryRow(query, platform, url).Scan(&exists)
	return exists, err
}

// Save stores a new search result in SQLite.
func (s *SQLiteStorer) Save(result search.SearchResult) error {
	query := `
	INSERT INTO search_results (Platform, Keyword, Title, URL, Timestamp, Content, AuthorID, AuthorHandle, AuthorName)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(URL) DO NOTHING;
	`
	_, err := s.db.Exec(query, result.Platform, result.Keyword, result.Title, result.URL, result.Timestamp, result.Content, result.AuthorID, result.AuthorHandle, result.AuthorName)
	return err
}

// GetLastSearchTime retrieves the last search time for a given platform from SQLite.
func (s *SQLiteStorer) GetLastSearchTime(platform string) (int64, error) {
	var lastSearchTime int64
	err := s.db.QueryRow(`SELECT LastSearchTime FROM last_search_time WHERE Platform = ?;`, platform).Scan(&lastSearchTime)
	if err == sql.ErrNoRows {
		// Default to epoch start if no record exists
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	return lastSearchTime, nil
}

// SetLastSearchTime updates the last search time for a given platform in SQLite.
func (s *SQLiteStorer) SetLastSearchTime(platform string, epochTime int64) error {
	query := `
	INSERT INTO last_search_time (Platform, LastSearchTime)
	VALUES (?, ?)
	ON CONFLICT(Platform) DO UPDATE SET LastSearchTime = excluded.LastSearchTime;
	`
	_, err := s.db.Exec(query, platform, epochTime)
	return err
}

// Close closes the SQLite database connection.
func (s *SQLiteStorer) Close() error {
	return s.db.Close()
}

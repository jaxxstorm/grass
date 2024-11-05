// storage/sqlite.go
package storage

import (
	"database/sql"
	"fmt"

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
		return nil, err
	}

	return &SQLiteStorer{db: db}, nil
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
	INSERT INTO search_results (Platform, Keyword, Title, URL, Timestamp)
	VALUES (?, ?, ?, ?, ?)
	ON CONFLICT(URL) DO NOTHING;
	`
	_, err := s.db.Exec(query, result.Platform, result.Keyword, result.Title, result.URL, result.Timestamp)
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

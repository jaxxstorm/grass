package localindex

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestFTS5PersistenceAndDriverCoexistence(t *testing.T) {
	registered := map[string]bool{}
	for _, name := range sql.Drivers() {
		registered[name] = true
	}
	if !registered["sqlite"] || !registered["sqlite3"] {
		t.Fatalf("missing distinct drivers: %v", registered)
	}
	path := filepath.Join(t.TempDir(), "fts.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE VIRTUAL TABLE posts USING fts5(title, tokenize='unicode61 remove_diacritics 0'); INSERT INTO posts(title) VALUES ('A network café'), ('networking cafe')`)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var title string
	if err := db.QueryRow(`SELECT title FROM posts WHERE posts MATCH ?`, `"network" AND "café"`).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "A network café" {
		t.Fatalf("unexpected FTS result: %q", title)
	}
}

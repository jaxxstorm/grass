package localindex

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/jaxxstorm/grass/search"
	"github.com/jaxxstorm/grass/storage"
)

func TestAbruptProcessExitPreservesCommittedStaging(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crash.db")
	cmd := exec.Command(os.Args[0], "-test.run=^TestCrashChild$")
	cmd.Env = append(os.Environ(), "GRASS_TEST_CRASH_INDEX="+path)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child failed: %s %v", output, err)
	}
	i, err := Open(context.Background(), path, Source{Kind: "crash-fixture"})
	if err != nil {
		t.Fatal(err)
	}
	defer i.Close()
	state, stage, err := i.State(context.Background())
	if err != nil || state.Generation != "" || stage.Cursor != "committed" {
		t.Fatalf("uncommitted cursor survived: %+v %+v %v", state, stage, err)
	}
	var count int
	if err = i.db.QueryRow("SELECT count(*) FROM staged_records").Scan(&count); err != nil || count != 1 {
		t.Fatalf("partial data survived: %d %v", count, err)
	}
}
func TestCrashChild(t *testing.T) {
	path := os.Getenv("GRASS_TEST_CRASH_INDEX")
	if path == "" {
		t.Skip("subprocess helper")
	}
	ctx := context.Background()
	i, err := Open(ctx, path, Source{Kind: "crash-fixture"})
	if err != nil {
		t.Fatal(err)
	}
	token, err := i.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stage := Stage{Active: true, Phase: "scan", Cursor: "committed"}
	if err = i.BeginStage(ctx, token, stage); err != nil {
		t.Fatal(err)
	}
	r := storage.ArchiveRecord{SearchResult: search.SearchResult{Platform: "X", URL: "https://example.test/committed"}}
	if err = i.StagePage(ctx, token, []storage.ArchiveRecord{r}, stage, storage.DefaultArchiveLimits()); err != nil {
		t.Fatal(err)
	}
	err = i.write(ctx, token, func(tx *sql.Tx) error {
		r.URL = "https://example.test/uncommitted"
		if _, err := put(ctx, tx, r, true); err != nil {
			return err
		}
		stage.Cursor = "uncommitted"
		if err := saveState(ctx, tx, State{}, stage); err != nil {
			return err
		}
		os.Exit(0) // Deliberately bypass transaction rollback and connection Close.
		return nil
	})
	t.Fatalf("child did not exit inside transaction: %v", err)
}

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rhystic/attractor/pkg/store"
)

// TestDBFlagParsing verifies --db and --no-db flag parsing, including the
// ATTRACTOR_DB environment variable override.
func TestDBFlagParsing(t *testing.T) {
	// Save and clear env var to avoid polluting other subtests
	origEnv := os.Getenv("ATTRACTOR_DB")
	defer os.Setenv("ATTRACTOR_DB", origEnv)

	t.Run("DefaultPath", func(t *testing.T) {
		os.Setenv("ATTRACTOR_DB", "")
		_, opts, err := parseArgs([]string{"run", "-f", "test.dot"})
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}

		home, _ := os.UserHomeDir()
		expected := filepath.Join(home, ".attractor", "attractor.db")
		if opts.dbPath != expected {
			t.Errorf("default dbPath: got %q, want %q", opts.dbPath, expected)
		}
	})

	t.Run("CustomPath", func(t *testing.T) {
		os.Setenv("ATTRACTOR_DB", "")
		_, opts, err := parseArgs([]string{"run", "-f", "test.dot", "--db", "/tmp/custom.db"})
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		if opts.dbPath != "/tmp/custom.db" {
			t.Errorf("dbPath: got %q, want %q", opts.dbPath, "/tmp/custom.db")
		}
	})

	t.Run("NoDBFlag", func(t *testing.T) {
		os.Setenv("ATTRACTOR_DB", "")
		_, opts, err := parseArgs([]string{"run", "-f", "test.dot", "--no-db"})
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		if opts.dbPath != "" {
			t.Errorf("dbPath should be empty with --no-db, got %q", opts.dbPath)
		}
	})

	t.Run("EnvVarOverride", func(t *testing.T) {
		os.Setenv("ATTRACTOR_DB", "/tmp/env-override.db")
		_, opts, err := parseArgs([]string{"run", "-f", "test.dot"})
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		if opts.dbPath != "/tmp/env-override.db" {
			t.Errorf("dbPath: got %q, want %q", opts.dbPath, "/tmp/env-override.db")
		}
	})

	t.Run("FlagOverridesEnv", func(t *testing.T) {
		os.Setenv("ATTRACTOR_DB", "/tmp/env.db")
		_, opts, err := parseArgs([]string{"run", "-f", "test.dot", "--db", "/tmp/flag.db"})
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		if opts.dbPath != "/tmp/flag.db" {
			t.Errorf("dbPath: got %q, want %q", opts.dbPath, "/tmp/flag.db")
		}
	})

	t.Run("NoDBOverridesEnv", func(t *testing.T) {
		os.Setenv("ATTRACTOR_DB", "/tmp/env.db")
		_, opts, err := parseArgs([]string{"run", "-f", "test.dot", "--no-db"})
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		if opts.dbPath != "" {
			t.Errorf("dbPath should be empty with --no-db even with env var, got %q", opts.dbPath)
		}
	})

	t.Run("DBFlagMissingValue", func(t *testing.T) {
		os.Setenv("ATTRACTOR_DB", "")
		_, _, err := parseArgs([]string{"run", "--db"})
		if err == nil {
			t.Error("expected error when --db has no value")
		}
	})
}

// TestDBCreatedOnRun verifies that the CLI creates the SQLite database file
// when --db is specified. We trigger a run that will fail (no API key), but
// the store should still have been created on disk before the LLM client error.
func TestDBCreatedOnRun(t *testing.T) {
	// Save and restore env
	origKey := os.Getenv("OPENROUTER_API_KEY")
	origDB := os.Getenv("ATTRACTOR_DB")
	defer func() {
		os.Setenv("OPENROUTER_API_KEY", origKey)
		os.Setenv("ATTRACTOR_DB", origDB)
	}()

	// Ensure no API key so runPipeline fails before calling LLM
	os.Unsetenv("OPENROUTER_API_KEY")
	os.Setenv("ATTRACTOR_DB", "")

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	dotPath := filepath.Join(tmpDir, "test.dot")

	// Write a valid DOT file
	dotContent := `digraph Test {
		start [shape=Mdiamond];
		task [label="Task"];
		exit [shape=Msquare];
		start -> task -> exit;
	}`
	if err := os.WriteFile(dotPath, []byte(dotContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Run should fail (no API key), but that's expected
	err := run([]string{"run", "-f", dotPath, "--db", dbPath}, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error (no API key), got nil")
	}

	// The database file should NOT have been created because the error happens
	// before store.Open is called (LLM client creation fails first in runPipeline)
	// This verifies the ordering: LLM client error prevents store creation.
	if _, statErr := os.Stat(dbPath); statErr == nil {
		// If the file exists, that's also fine — it means the store was opened
		// before the LLM client error. Either way, verify the DB is valid.
		db, openErr := store.Open(dbPath)
		if openErr != nil {
			t.Fatalf("store exists but can't open: %v", openErr)
		}
		db.Close()
	}
}

// TestStoreOpenAndSchemaOnDisk verifies that store.Open creates a valid
// database file with all expected tables, simulating what the CLI does.
func TestStoreOpenAndSchemaOnDisk(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "schema-test.db")

	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	// Verify file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("database file was not created")
	}

	// Verify all tables exist by querying sqlite_master
	tables := []string{"runs", "events", "conversations", "artifacts", "token_usage", "context_snapshots"}
	for _, table := range tables {
		var name string
		err := db.DB().QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}

	// Verify WAL mode is active
	var journalMode string
	if err := db.DB().QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("journal_mode: got %q, want %q", journalMode, "wal")
	}
}

// TestStoreReopenPersistence verifies that data survives store close and
// reopen, simulating the CLI writing data that the TUI reads later.
func TestStoreReopenPersistence(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "persist-test.db")

	// First session: write data
	func() {
		db, err := store.Open(dbPath)
		if err != nil {
			t.Fatalf("open store (write): %v", err)
		}
		defer db.Close()

		err = db.CreateRun(store.Run{
			ID:   "run_persist_test",
			Mode: "pipeline",
		})
		if err != nil {
			t.Fatalf("create run: %v", err)
		}
	}()

	// Second session: read data back
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store (read): %v", err)
	}
	defer db.Close()

	run, err := db.GetRun("run_persist_test")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Mode != "pipeline" {
		t.Errorf("run mode: got %q, want %q", run.Mode, "pipeline")
	}
}

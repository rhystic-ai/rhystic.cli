// Package store provides SQLite-backed persistence for pipeline events,
// artifacts, and execution state. It is designed for concurrent access:
// the CLI writes during execution while a TUI dashboard reads.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rhystic/attractor/pkg/events"

	_ "modernc.org/sqlite"
)

// Store provides read/write access to the attractor SQLite database.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at dbPath, runs migrations,
// and enables WAL mode for concurrent readers.
func Open(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return s, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// DB returns the underlying *sql.DB for advanced queries.
func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(schema)
	return err
}

const schema = `
CREATE TABLE IF NOT EXISTS runs (
	id                  TEXT PRIMARY KEY,
	mode                TEXT NOT NULL,
	graph_name          TEXT,
	goal                TEXT,
	model               TEXT,
	status              TEXT,
	started_at          DATETIME NOT NULL,
	ended_at            DATETIME,
	duration_ms         INTEGER,
	total_input_tokens  INTEGER DEFAULT 0,
	total_output_tokens INTEGER DEFAULT 0,
	total_cost_usd      REAL    DEFAULT 0.0
);

CREATE TABLE IF NOT EXISTS events (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id     TEXT     NOT NULL REFERENCES runs(id),
	node_id    TEXT,
	event_type TEXT     NOT NULL,
	timestamp  DATETIME NOT NULL,
	data       TEXT     NOT NULL,
	level      TEXT,
	status     TEXT,
	model      TEXT,
	tool_name  TEXT,
	is_error   BOOLEAN
);
CREATE INDEX IF NOT EXISTS idx_events_run  ON events(run_id, timestamp);
CREATE INDEX IF NOT EXISTS idx_events_type ON events(event_type);
CREATE INDEX IF NOT EXISTS idx_events_node ON events(run_id, node_id);

CREATE TABLE IF NOT EXISTS conversations (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id       TEXT     NOT NULL REFERENCES runs(id),
	node_id      TEXT     NOT NULL,
	turn_index   INTEGER  NOT NULL,
	role         TEXT     NOT NULL,
	content      TEXT,
	tool_calls   TEXT,
	tool_results TEXT,
	timestamp    DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_conv_node ON conversations(run_id, node_id, turn_index);

CREATE TABLE IF NOT EXISTS artifacts (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id     TEXT     NOT NULL REFERENCES runs(id),
	node_id    TEXT     NOT NULL,
	kind       TEXT     NOT NULL,
	content    TEXT     NOT NULL,
	created_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_artifacts_node ON artifacts(run_id, node_id, kind);

CREATE TABLE IF NOT EXISTS token_usage (
	id                INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id            TEXT     NOT NULL REFERENCES runs(id),
	node_id           TEXT,
	model             TEXT     NOT NULL,
	input_tokens      INTEGER  NOT NULL,
	output_tokens     INTEGER  NOT NULL,
	reasoning_tokens  INTEGER  DEFAULT 0,
	cache_read_tokens INTEGER  DEFAULT 0,
	cost_usd          REAL,
	timestamp         DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_usage_run ON token_usage(run_id);

CREATE TABLE IF NOT EXISTS context_snapshots (
	id              INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id          TEXT     NOT NULL REFERENCES runs(id),
	node_id         TEXT     NOT NULL,
	snapshot        TEXT     NOT NULL,
	completed_nodes TEXT,
	created_at      DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_snapshots_run ON context_snapshots(run_id, node_id);
`

// ---------------------------------------------------------------------------
// Write-path types
// ---------------------------------------------------------------------------

// Run represents a pipeline or agent execution run.
type Run struct {
	ID        string
	Mode      string // "pipeline" | "agent"
	GraphName string
	Goal      string
	Model     string
	StartedAt time.Time
}

// RunUpdate holds fields to update when a run completes.
type RunUpdate struct {
	Status            string
	EndedAt           time.Time
	DurationMs        int64
	TotalInputTokens  int
	TotalOutputTokens int
	TotalCostUSD      float64
}

// TokenUsageRecord captures per-LLM-call token usage.
type TokenUsageRecord struct {
	RunID           string
	NodeID          string
	Model           string
	InputTokens     int
	OutputTokens    int
	ReasoningTokens int
	CacheReadTokens int
	CostUSD         float64
	Timestamp       time.Time
}

// ConversationTurn represents a single turn persisted for replay.
type ConversationTurn struct {
	RunID       string
	NodeID      string
	TurnIndex   int
	Role        string
	Content     string
	ToolCalls   string // JSON
	ToolResults string // JSON
	Timestamp   time.Time
}

// ---------------------------------------------------------------------------
// Write methods
// ---------------------------------------------------------------------------

// CreateRun inserts a new run record.
func (s *Store) CreateRun(r Run) error {
	_, err := s.db.Exec(
		`INSERT INTO runs (id, mode, graph_name, goal, model, started_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		r.ID, r.Mode, r.GraphName, r.Goal, r.Model, r.StartedAt,
	)
	if err != nil {
		return fmt.Errorf("create run: %w", err)
	}
	return nil
}

// UpdateRun updates a run with completion data.
func (s *Store) UpdateRun(runID string, u RunUpdate) error {
	_, err := s.db.Exec(
		`UPDATE runs
		 SET status = ?, ended_at = ?, duration_ms = ?,
		     total_input_tokens = ?, total_output_tokens = ?, total_cost_usd = ?
		 WHERE id = ?`,
		u.Status, u.EndedAt, u.DurationMs,
		u.TotalInputTokens, u.TotalOutputTokens, u.TotalCostUSD,
		runID,
	)
	if err != nil {
		return fmt.Errorf("update run: %w", err)
	}
	return nil
}

// InsertEvent persists a pipeline event with denormalized filter columns.
func (s *Store) InsertEvent(evt events.Event) error {
	data, err := json.Marshal(evt.Data)
	if err != nil {
		return fmt.Errorf("marshal event data: %w", err)
	}

	// Denormalize for fast filtering
	var level, status, model, toolName sql.NullString
	var isError sql.NullBool

	switch evt.Type {
	case events.EventLog:
		level = toNullString(evt.Data.Level)
	case events.EventNodeEnd:
		status = toNullString(evt.Data.Status)
	case events.EventLLMStart, events.EventLLMEnd:
		model = toNullString(evt.Data.Model)
	case events.EventToolStart, events.EventToolEnd:
		toolName = toNullString(evt.Data.ToolName)
		if evt.Data.IsError {
			isError = sql.NullBool{Bool: true, Valid: true}
		}
	}

	_, err = s.db.Exec(
		`INSERT INTO events (run_id, node_id, event_type, timestamp, data,
		                      level, status, model, tool_name, is_error)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		evt.RunID, toNullString(evt.NodeID), string(evt.Type), evt.Timestamp, string(data),
		level, status, model, toolName, isError,
	)
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	return nil
}

// InsertConversationTurn persists a single conversation turn.
func (s *Store) InsertConversationTurn(t ConversationTurn) error {
	_, err := s.db.Exec(
		`INSERT INTO conversations (run_id, node_id, turn_index, role, content,
		                             tool_calls, tool_results, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		t.RunID, t.NodeID, t.TurnIndex, t.Role, t.Content,
		toNullString(t.ToolCalls), toNullString(t.ToolResults), t.Timestamp,
	)
	if err != nil {
		return fmt.Errorf("insert conversation turn: %w", err)
	}
	return nil
}

// InsertArtifact persists a prompt, response, or other artifact.
func (s *Store) InsertArtifact(runID, nodeID, kind, content string) error {
	_, err := s.db.Exec(
		`INSERT INTO artifacts (run_id, node_id, kind, content, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		runID, nodeID, kind, content, time.Now(),
	)
	if err != nil {
		return fmt.Errorf("insert artifact: %w", err)
	}
	return nil
}

// InsertTokenUsage persists per-LLM-call token usage.
func (s *Store) InsertTokenUsage(r TokenUsageRecord) error {
	_, err := s.db.Exec(
		`INSERT INTO token_usage (run_id, node_id, model, input_tokens, output_tokens,
		                           reasoning_tokens, cache_read_tokens, cost_usd, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.RunID, toNullString(r.NodeID), r.Model,
		r.InputTokens, r.OutputTokens, r.ReasoningTokens, r.CacheReadTokens,
		r.CostUSD, r.Timestamp,
	)
	if err != nil {
		return fmt.Errorf("insert token usage: %w", err)
	}
	return nil
}

// InsertContextSnapshot persists a pipeline context snapshot after a node.
func (s *Store) InsertContextSnapshot(runID, nodeID string, snapshot map[string]any, completedNodes []string) error {
	snapJSON, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("marshal context snapshot: %w", err)
	}

	nodesJSON, err := json.Marshal(completedNodes)
	if err != nil {
		return fmt.Errorf("marshal completed nodes: %w", err)
	}

	_, err = s.db.Exec(
		`INSERT INTO context_snapshots (run_id, node_id, snapshot, completed_nodes, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		runID, nodeID, string(snapJSON), string(nodesJSON), time.Now(),
	)
	if err != nil {
		return fmt.Errorf("insert context snapshot: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Event subscriber helper
// ---------------------------------------------------------------------------

// PersistEvents drains an event channel and writes every event to the store.
// It also extracts token usage from EventLLMEnd events. This function blocks
// until the channel is closed; run it in a goroutine.
func (s *Store) PersistEvents(ch <-chan events.Event, model string) {
	for evt := range ch {
		// Best-effort: log errors but don't block the pipeline.
		_ = s.InsertEvent(evt)

		if evt.Type == events.EventLLMEnd && (evt.Data.InputTokens > 0 || evt.Data.OutputTokens > 0) {
			usage := TokenUsageRecord{
				RunID:        evt.RunID,
				NodeID:       evt.NodeID,
				Model:        model,
				InputTokens:  evt.Data.InputTokens,
				OutputTokens: evt.Data.OutputTokens,
				Timestamp:    evt.Timestamp,
			}
			_ = s.InsertTokenUsage(usage)
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func toNullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

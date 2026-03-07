package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rhystic/attractor/pkg/events"
)

// ---------------------------------------------------------------------------
// Read-path types
// ---------------------------------------------------------------------------

// RunRecord is the full persisted representation of a run.
type RunRecord struct {
	ID                string
	Mode              string
	GraphName         string
	Goal              string
	Model             string
	Status            string
	StartedAt         time.Time
	EndedAt           *time.Time
	DurationMs        int64
	TotalInputTokens  int
	TotalOutputTokens int
	TotalCostUSD      float64
}

// RunSummary provides aggregated statistics for a single run.
type RunSummary struct {
	Run               RunRecord
	EventCount        int
	NodeCount         int
	ToolCallCount     int
	ErrorCount        int
	TotalInputTokens  int
	TotalOutputTokens int
	TotalCostUSD      float64
}

// StoredEvent is a persisted event with its database ID.
type StoredEvent struct {
	ID        int64
	RunID     string
	NodeID    string
	EventType events.EventType
	Timestamp time.Time
	Data      events.EventData
}

// Artifact is a persisted prompt, response, or status artifact.
type Artifact struct {
	ID        int64
	RunID     string
	NodeID    string
	Kind      string
	Content   string
	CreatedAt time.Time
}

// ContextSnapshot is a persisted context state after a node.
type ContextSnapshot struct {
	ID             int64
	RunID          string
	NodeID         string
	Snapshot       map[string]any
	CompletedNodes []string
	CreatedAt      time.Time
}

// EventFilter controls which events are returned by GetEvents.
type EventFilter struct {
	EventType string // empty = all
	NodeID    string // empty = all
	Level     string // empty = all (for log events)
	Limit     int    // 0 = no limit
	Offset    int
}

// ---------------------------------------------------------------------------
// Read methods
// ---------------------------------------------------------------------------

// GetRun retrieves a single run by ID.
func (s *Store) GetRun(runID string) (*RunRecord, error) {
	row := s.db.QueryRow(
		`SELECT id, mode, graph_name, goal, model, status,
		        started_at, ended_at, duration_ms,
		        total_input_tokens, total_output_tokens, total_cost_usd
		 FROM runs WHERE id = ?`, runID,
	)
	return scanRun(row)
}

// ListRuns returns the most recent runs, newest first.
func (s *Store) ListRuns(limit int) ([]RunRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(
		`SELECT id, mode, graph_name, goal, model, status,
		        started_at, ended_at, duration_ms,
		        total_input_tokens, total_output_tokens, total_cost_usd
		 FROM runs ORDER BY started_at DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	defer rows.Close()

	var result []RunRecord
	for rows.Next() {
		r, err := scanRunRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *r)
	}
	return result, rows.Err()
}

// GetEvents retrieves events for a run with optional filtering.
func (s *Store) GetEvents(runID string, f EventFilter) ([]StoredEvent, error) {
	query := `SELECT id, run_id, node_id, event_type, timestamp, data
	          FROM events WHERE run_id = ?`
	args := []any{runID}

	if f.EventType != "" {
		query += " AND event_type = ?"
		args = append(args, f.EventType)
	}
	if f.NodeID != "" {
		query += " AND node_id = ?"
		args = append(args, f.NodeID)
	}
	if f.Level != "" {
		query += " AND level = ?"
		args = append(args, f.Level)
	}

	query += " ORDER BY timestamp ASC"

	if f.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, f.Limit)
	}
	if f.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, f.Offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("get events: %w", err)
	}
	defer rows.Close()

	var result []StoredEvent
	for rows.Next() {
		var e StoredEvent
		var nodeID sql.NullString
		var dataJSON string
		if err := rows.Scan(&e.ID, &e.RunID, &nodeID, &e.EventType, &e.Timestamp, &dataJSON); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		e.NodeID = nodeID.String
		if err := json.Unmarshal([]byte(dataJSON), &e.Data); err != nil {
			return nil, fmt.Errorf("unmarshal event data: %w", err)
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

// GetConversation retrieves conversation turns for a run+node.
func (s *Store) GetConversation(runID, nodeID string) ([]ConversationTurn, error) {
	rows, err := s.db.Query(
		`SELECT run_id, node_id, turn_index, role, content,
		        tool_calls, tool_results, timestamp
		 FROM conversations
		 WHERE run_id = ? AND node_id = ?
		 ORDER BY turn_index ASC`, runID, nodeID,
	)
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}
	defer rows.Close()

	var result []ConversationTurn
	for rows.Next() {
		var t ConversationTurn
		var toolCalls, toolResults sql.NullString
		if err := rows.Scan(&t.RunID, &t.NodeID, &t.TurnIndex, &t.Role, &t.Content,
			&toolCalls, &toolResults, &t.Timestamp); err != nil {
			return nil, fmt.Errorf("scan conversation: %w", err)
		}
		t.ToolCalls = toolCalls.String
		t.ToolResults = toolResults.String
		result = append(result, t)
	}
	return result, rows.Err()
}

// GetArtifacts retrieves artifacts for a run+node.
func (s *Store) GetArtifacts(runID, nodeID string) ([]Artifact, error) {
	rows, err := s.db.Query(
		`SELECT id, run_id, node_id, kind, content, created_at
		 FROM artifacts
		 WHERE run_id = ? AND node_id = ?
		 ORDER BY created_at ASC`, runID, nodeID,
	)
	if err != nil {
		return nil, fmt.Errorf("get artifacts: %w", err)
	}
	defer rows.Close()

	var result []Artifact
	for rows.Next() {
		var a Artifact
		if err := rows.Scan(&a.ID, &a.RunID, &a.NodeID, &a.Kind, &a.Content, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan artifact: %w", err)
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

// GetTokenUsage retrieves per-call token usage records for a run.
func (s *Store) GetTokenUsage(runID string) ([]TokenUsageRecord, error) {
	rows, err := s.db.Query(
		`SELECT run_id, node_id, model, input_tokens, output_tokens,
		        reasoning_tokens, cache_read_tokens, cost_usd, timestamp
		 FROM token_usage
		 WHERE run_id = ?
		 ORDER BY timestamp ASC`, runID,
	)
	if err != nil {
		return nil, fmt.Errorf("get token usage: %w", err)
	}
	defer rows.Close()

	var result []TokenUsageRecord
	for rows.Next() {
		var r TokenUsageRecord
		var nodeID sql.NullString
		if err := rows.Scan(&r.RunID, &nodeID, &r.Model,
			&r.InputTokens, &r.OutputTokens, &r.ReasoningTokens, &r.CacheReadTokens,
			&r.CostUSD, &r.Timestamp); err != nil {
			return nil, fmt.Errorf("scan token usage: %w", err)
		}
		r.NodeID = nodeID.String
		result = append(result, r)
	}
	return result, rows.Err()
}

// GetContextSnapshot retrieves the context snapshot for a run+node.
func (s *Store) GetContextSnapshot(runID, nodeID string) (*ContextSnapshot, error) {
	var cs ContextSnapshot
	var snapJSON, nodesJSON string
	err := s.db.QueryRow(
		`SELECT id, run_id, node_id, snapshot, completed_nodes, created_at
		 FROM context_snapshots
		 WHERE run_id = ? AND node_id = ?
		 ORDER BY created_at DESC LIMIT 1`, runID, nodeID,
	).Scan(&cs.ID, &cs.RunID, &cs.NodeID, &snapJSON, &nodesJSON, &cs.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get context snapshot: %w", err)
	}

	if err := json.Unmarshal([]byte(snapJSON), &cs.Snapshot); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot: %w", err)
	}
	if err := json.Unmarshal([]byte(nodesJSON), &cs.CompletedNodes); err != nil {
		return nil, fmt.Errorf("unmarshal completed nodes: %w", err)
	}
	return &cs, nil
}

// GetRunSummary returns aggregated statistics for a run.
func (s *Store) GetRunSummary(runID string) (*RunSummary, error) {
	run, err := s.GetRun(runID)
	if err != nil {
		return nil, err
	}

	summary := &RunSummary{Run: *run}

	// Event count
	s.db.QueryRow(`SELECT COUNT(*) FROM events WHERE run_id = ?`, runID).Scan(&summary.EventCount)

	// Distinct node count (from node_start events)
	s.db.QueryRow(
		`SELECT COUNT(DISTINCT node_id) FROM events WHERE run_id = ? AND event_type = 'node_start'`,
		runID,
	).Scan(&summary.NodeCount)

	// Tool call count
	s.db.QueryRow(
		`SELECT COUNT(*) FROM events WHERE run_id = ? AND event_type = 'tool_start'`,
		runID,
	).Scan(&summary.ToolCallCount)

	// Error count
	s.db.QueryRow(
		`SELECT COUNT(*) FROM events WHERE run_id = ? AND is_error = 1`,
		runID,
	).Scan(&summary.ErrorCount)

	// Token totals
	s.db.QueryRow(
		`SELECT COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(cost_usd), 0)
		 FROM token_usage WHERE run_id = ?`, runID,
	).Scan(&summary.TotalInputTokens, &summary.TotalOutputTokens, &summary.TotalCostUSD)

	return summary, nil
}

// ListRunNodes returns the distinct node IDs that executed in a run, in order.
func (s *Store) ListRunNodes(runID string) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT DISTINCT node_id FROM events
		 WHERE run_id = ? AND event_type = 'node_start' AND node_id IS NOT NULL
		 ORDER BY timestamp ASC`, runID,
	)
	if err != nil {
		return nil, fmt.Errorf("list run nodes: %w", err)
	}
	defer rows.Close()

	var nodes []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		nodes = append(nodes, id)
	}
	return nodes, rows.Err()
}

// ---------------------------------------------------------------------------
// Row scanners
// ---------------------------------------------------------------------------

type scanner interface {
	Scan(dest ...any) error
}

func scanRun(row scanner) (*RunRecord, error) {
	var r RunRecord
	var graphName, goal, model, status sql.NullString
	var endedAt sql.NullTime
	var durationMs sql.NullInt64
	var inputTokens, outputTokens sql.NullInt64
	var costUSD sql.NullFloat64

	err := row.Scan(&r.ID, &r.Mode, &graphName, &goal, &model, &status,
		&r.StartedAt, &endedAt, &durationMs,
		&inputTokens, &outputTokens, &costUSD,
	)
	if err != nil {
		return nil, fmt.Errorf("scan run: %w", err)
	}

	r.GraphName = graphName.String
	r.Goal = goal.String
	r.Model = model.String
	r.Status = status.String
	if endedAt.Valid {
		r.EndedAt = &endedAt.Time
	}
	r.DurationMs = durationMs.Int64
	r.TotalInputTokens = int(inputTokens.Int64)
	r.TotalOutputTokens = int(outputTokens.Int64)
	r.TotalCostUSD = costUSD.Float64

	return &r, nil
}

func scanRunRow(rows *sql.Rows) (*RunRecord, error) {
	return scanRun(rows)
}

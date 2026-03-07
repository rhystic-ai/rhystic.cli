package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rhystic/attractor/pkg/events"
)

func tempDB(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpenCreatesMissingDirectory(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b", "c", "test.db")
	s, err := Open(nested)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	s.Close()

	if _, err := os.Stat(nested); err != nil {
		t.Fatalf("db file should exist: %v", err)
	}
}

func TestCreateAndGetRun(t *testing.T) {
	s := tempDB(t)

	run := Run{
		ID:        "run_123",
		Mode:      "pipeline",
		GraphName: "test-graph",
		Goal:      "test goal",
		Model:     "minimax/minimax-m2.5",
		StartedAt: time.Now().Truncate(time.Second),
	}

	if err := s.CreateRun(run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	got, err := s.GetRun("run_123")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}

	if got.ID != run.ID {
		t.Errorf("ID: got %q, want %q", got.ID, run.ID)
	}
	if got.Mode != "pipeline" {
		t.Errorf("Mode: got %q, want %q", got.Mode, "pipeline")
	}
	if got.GraphName != "test-graph" {
		t.Errorf("GraphName: got %q, want %q", got.GraphName, "test-graph")
	}
	if got.Goal != "test goal" {
		t.Errorf("Goal: got %q, want %q", got.Goal, "test goal")
	}
	if got.Status != "" {
		t.Errorf("Status: got %q, want empty", got.Status)
	}
}

func TestUpdateRun(t *testing.T) {
	s := tempDB(t)

	s.CreateRun(Run{
		ID:        "run_456",
		Mode:      "agent",
		StartedAt: time.Now(),
	})

	now := time.Now().Truncate(time.Second)
	err := s.UpdateRun("run_456", RunUpdate{
		Status:            "success",
		EndedAt:           now,
		DurationMs:        5000,
		TotalInputTokens:  1000,
		TotalOutputTokens: 500,
		TotalCostUSD:      0.0042,
	})
	if err != nil {
		t.Fatalf("update run: %v", err)
	}

	got, _ := s.GetRun("run_456")
	if got.Status != "success" {
		t.Errorf("Status: got %q, want %q", got.Status, "success")
	}
	if got.DurationMs != 5000 {
		t.Errorf("DurationMs: got %d, want 5000", got.DurationMs)
	}
	if got.TotalInputTokens != 1000 {
		t.Errorf("TotalInputTokens: got %d, want 1000", got.TotalInputTokens)
	}
}

func TestListRuns(t *testing.T) {
	s := tempDB(t)

	for i := 0; i < 5; i++ {
		s.CreateRun(Run{
			ID:        "run_" + time.Now().Format("150405.000000000"),
			Mode:      "pipeline",
			StartedAt: time.Now(),
		})
		time.Sleep(time.Millisecond) // ensure ordering
	}

	runs, err := s.ListRuns(3)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 3 {
		t.Errorf("got %d runs, want 3", len(runs))
	}
}

func TestInsertAndGetEvents(t *testing.T) {
	s := tempDB(t)

	s.CreateRun(Run{ID: "run_evt", Mode: "pipeline", StartedAt: time.Now()})

	evts := []events.Event{
		{
			Type:      events.EventNodeStart,
			RunID:     "run_evt",
			NodeID:    "node_1",
			Timestamp: time.Now(),
			Data:      events.EventData{NodeLabel: "Plan", NodeType: "codergen"},
		},
		{
			Type:      events.EventLLMEnd,
			RunID:     "run_evt",
			NodeID:    "node_1",
			Timestamp: time.Now(),
			Data:      events.EventData{Response: "done", InputTokens: 100, OutputTokens: 50, Model: "test-model"},
		},
		{
			Type:      events.EventNodeEnd,
			RunID:     "run_evt",
			NodeID:    "node_1",
			Timestamp: time.Now(),
			Data:      events.EventData{Status: "success", Notes: "ok"},
		},
		{
			Type:      events.EventToolStart,
			RunID:     "run_evt",
			NodeID:    "node_1",
			Timestamp: time.Now(),
			Data:      events.EventData{ToolName: "shell", ToolArgs: "ls -la"},
		},
		{
			Type:      events.EventLog,
			RunID:     "run_evt",
			Timestamp: time.Now(),
			Data:      events.EventData{Level: "warn", Message: "something fishy"},
		},
	}

	for _, evt := range evts {
		if err := s.InsertEvent(evt); err != nil {
			t.Fatalf("insert event %s: %v", evt.Type, err)
		}
	}

	// Get all events
	all, err := s.GetEvents("run_evt", EventFilter{})
	if err != nil {
		t.Fatalf("get events: %v", err)
	}
	if len(all) != 5 {
		t.Errorf("got %d events, want 5", len(all))
	}

	// Filter by type
	nodeEvents, err := s.GetEvents("run_evt", EventFilter{EventType: "node_start"})
	if err != nil {
		t.Fatalf("get node events: %v", err)
	}
	if len(nodeEvents) != 1 {
		t.Errorf("got %d node_start events, want 1", len(nodeEvents))
	}
	if nodeEvents[0].Data.NodeLabel != "Plan" {
		t.Errorf("NodeLabel: got %q, want %q", nodeEvents[0].Data.NodeLabel, "Plan")
	}

	// Filter by node
	node1Events, err := s.GetEvents("run_evt", EventFilter{NodeID: "node_1"})
	if err != nil {
		t.Fatalf("get node_1 events: %v", err)
	}
	if len(node1Events) != 4 {
		t.Errorf("got %d node_1 events, want 4", len(node1Events))
	}

	// Filter by level (log events)
	warnEvents, err := s.GetEvents("run_evt", EventFilter{Level: "warn"})
	if err != nil {
		t.Fatalf("get warn events: %v", err)
	}
	if len(warnEvents) != 1 {
		t.Errorf("got %d warn events, want 1", len(warnEvents))
	}
}

func TestInsertAndGetConversation(t *testing.T) {
	s := tempDB(t)
	s.CreateRun(Run{ID: "run_conv", Mode: "pipeline", StartedAt: time.Now()})

	turns := []ConversationTurn{
		{RunID: "run_conv", NodeID: "node_1", TurnIndex: 0, Role: "user", Content: "hello", Timestamp: time.Now()},
		{RunID: "run_conv", NodeID: "node_1", TurnIndex: 1, Role: "assistant", Content: "hi there", ToolCalls: `[{"id":"tc_1","name":"shell"}]`, Timestamp: time.Now()},
		{RunID: "run_conv", NodeID: "node_1", TurnIndex: 2, Role: "tool", ToolResults: `[{"tool_call_id":"tc_1","content":"ok"}]`, Timestamp: time.Now()},
	}

	for _, t2 := range turns {
		if err := s.InsertConversationTurn(t2); err != nil {
			t.Fatalf("insert conversation turn: %v", err)
		}
	}

	got, err := s.GetConversation("run_conv", "node_1")
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d turns, want 3", len(got))
	}
	if got[0].Role != "user" || got[0].Content != "hello" {
		t.Errorf("turn 0: got role=%q content=%q", got[0].Role, got[0].Content)
	}
	if got[1].ToolCalls == "" {
		t.Error("turn 1: expected tool_calls to be non-empty")
	}
}

func TestInsertAndGetArtifacts(t *testing.T) {
	s := tempDB(t)
	s.CreateRun(Run{ID: "run_art", Mode: "pipeline", StartedAt: time.Now()})

	if err := s.InsertArtifact("run_art", "node_1", "prompt", "Write a function"); err != nil {
		t.Fatalf("insert prompt: %v", err)
	}
	if err := s.InsertArtifact("run_art", "node_1", "response", "func foo() {}"); err != nil {
		t.Fatalf("insert response: %v", err)
	}

	arts, err := s.GetArtifacts("run_art", "node_1")
	if err != nil {
		t.Fatalf("get artifacts: %v", err)
	}
	if len(arts) != 2 {
		t.Fatalf("got %d artifacts, want 2", len(arts))
	}
	if arts[0].Kind != "prompt" {
		t.Errorf("kind: got %q, want %q", arts[0].Kind, "prompt")
	}
	if arts[1].Content != "func foo() {}" {
		t.Errorf("content: got %q, want %q", arts[1].Content, "func foo() {}")
	}
}

func TestInsertAndGetTokenUsage(t *testing.T) {
	s := tempDB(t)
	s.CreateRun(Run{ID: "run_tok", Mode: "agent", StartedAt: time.Now()})

	records := []TokenUsageRecord{
		{RunID: "run_tok", NodeID: "n1", Model: "m1", InputTokens: 100, OutputTokens: 50, CostUSD: 0.001, Timestamp: time.Now()},
		{RunID: "run_tok", NodeID: "n1", Model: "m1", InputTokens: 200, OutputTokens: 80, CostUSD: 0.002, Timestamp: time.Now()},
	}

	for _, r := range records {
		if err := s.InsertTokenUsage(r); err != nil {
			t.Fatalf("insert token usage: %v", err)
		}
	}

	got, err := s.GetTokenUsage("run_tok")
	if err != nil {
		t.Fatalf("get token usage: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d records, want 2", len(got))
	}
	if got[0].InputTokens != 100 {
		t.Errorf("InputTokens: got %d, want 100", got[0].InputTokens)
	}
}

func TestInsertAndGetContextSnapshot(t *testing.T) {
	s := tempDB(t)
	s.CreateRun(Run{ID: "run_snap", Mode: "pipeline", StartedAt: time.Now()})

	snapshot := map[string]any{"goal": "test", "outcome": "success", "count": float64(42)}
	nodes := []string{"start", "plan", "implement"}

	if err := s.InsertContextSnapshot("run_snap", "implement", snapshot, nodes); err != nil {
		t.Fatalf("insert context snapshot: %v", err)
	}

	got, err := s.GetContextSnapshot("run_snap", "implement")
	if err != nil {
		t.Fatalf("get context snapshot: %v", err)
	}

	if got.Snapshot["goal"] != "test" {
		t.Errorf("snapshot goal: got %v, want %q", got.Snapshot["goal"], "test")
	}
	if len(got.CompletedNodes) != 3 {
		t.Errorf("completed nodes: got %d, want 3", len(got.CompletedNodes))
	}
	if got.CompletedNodes[2] != "implement" {
		t.Errorf("last node: got %q, want %q", got.CompletedNodes[2], "implement")
	}
}

func TestGetRunSummary(t *testing.T) {
	s := tempDB(t)
	s.CreateRun(Run{ID: "run_sum", Mode: "pipeline", GraphName: "test", StartedAt: time.Now()})

	// Insert some events
	now := time.Now()
	s.InsertEvent(events.Event{Type: events.EventNodeStart, RunID: "run_sum", NodeID: "n1", Timestamp: now, Data: events.EventData{NodeLabel: "A"}})
	s.InsertEvent(events.Event{Type: events.EventNodeStart, RunID: "run_sum", NodeID: "n2", Timestamp: now, Data: events.EventData{NodeLabel: "B"}})
	s.InsertEvent(events.Event{Type: events.EventToolStart, RunID: "run_sum", NodeID: "n1", Timestamp: now, Data: events.EventData{ToolName: "shell"}})
	s.InsertEvent(events.Event{Type: events.EventToolEnd, RunID: "run_sum", NodeID: "n1", Timestamp: now, Data: events.EventData{ToolName: "shell", IsError: true}})

	s.InsertTokenUsage(TokenUsageRecord{RunID: "run_sum", NodeID: "n1", Model: "m1", InputTokens: 500, OutputTokens: 200, CostUSD: 0.01, Timestamp: now})
	s.InsertTokenUsage(TokenUsageRecord{RunID: "run_sum", NodeID: "n2", Model: "m1", InputTokens: 300, OutputTokens: 100, CostUSD: 0.005, Timestamp: now})

	summary, err := s.GetRunSummary("run_sum")
	if err != nil {
		t.Fatalf("get run summary: %v", err)
	}

	if summary.Run.GraphName != "test" {
		t.Errorf("GraphName: got %q, want %q", summary.Run.GraphName, "test")
	}
	if summary.EventCount != 4 {
		t.Errorf("EventCount: got %d, want 4", summary.EventCount)
	}
	if summary.NodeCount != 2 {
		t.Errorf("NodeCount: got %d, want 2", summary.NodeCount)
	}
	if summary.ToolCallCount != 1 {
		t.Errorf("ToolCallCount: got %d, want 1", summary.ToolCallCount)
	}
	if summary.ErrorCount != 1 {
		t.Errorf("ErrorCount: got %d, want 1", summary.ErrorCount)
	}
	if summary.TotalInputTokens != 800 {
		t.Errorf("TotalInputTokens: got %d, want 800", summary.TotalInputTokens)
	}
}

func TestListRunNodes(t *testing.T) {
	s := tempDB(t)
	s.CreateRun(Run{ID: "run_nodes", Mode: "pipeline", StartedAt: time.Now()})

	now := time.Now()
	s.InsertEvent(events.Event{Type: events.EventNodeStart, RunID: "run_nodes", NodeID: "start", Timestamp: now})
	s.InsertEvent(events.Event{Type: events.EventNodeStart, RunID: "run_nodes", NodeID: "plan", Timestamp: now.Add(time.Second)})
	s.InsertEvent(events.Event{Type: events.EventNodeStart, RunID: "run_nodes", NodeID: "implement", Timestamp: now.Add(2 * time.Second)})

	nodes, err := s.ListRunNodes("run_nodes")
	if err != nil {
		t.Fatalf("list run nodes: %v", err)
	}
	if len(nodes) != 3 {
		t.Fatalf("got %d nodes, want 3", len(nodes))
	}
	if nodes[0] != "start" || nodes[1] != "plan" || nodes[2] != "implement" {
		t.Errorf("nodes: got %v, want [start plan implement]", nodes)
	}
}

func TestPersistEvents(t *testing.T) {
	s := tempDB(t)
	s.CreateRun(Run{ID: "run_persist", Mode: "agent", StartedAt: time.Now()})

	ch := make(chan events.Event, 10)
	ch <- events.Event{
		Type: events.EventNodeStart, RunID: "run_persist", NodeID: "n1",
		Timestamp: time.Now(), Data: events.EventData{NodeLabel: "test"},
	}
	ch <- events.Event{
		Type: events.EventLLMEnd, RunID: "run_persist", NodeID: "n1",
		Timestamp: time.Now(), Data: events.EventData{InputTokens: 500, OutputTokens: 200},
	}
	close(ch)

	s.PersistEvents(ch, "test-model")

	evts, _ := s.GetEvents("run_persist", EventFilter{})
	if len(evts) != 2 {
		t.Errorf("got %d events, want 2", len(evts))
	}

	usage, _ := s.GetTokenUsage("run_persist")
	if len(usage) != 1 {
		t.Fatalf("got %d usage records, want 1", len(usage))
	}
	if usage[0].InputTokens != 500 {
		t.Errorf("InputTokens: got %d, want 500", usage[0].InputTokens)
	}
	if usage[0].Model != "test-model" {
		t.Errorf("Model: got %q, want %q", usage[0].Model, "test-model")
	}
}

func TestEventPagination(t *testing.T) {
	s := tempDB(t)
	s.CreateRun(Run{ID: "run_page", Mode: "pipeline", StartedAt: time.Now()})

	for i := 0; i < 20; i++ {
		s.InsertEvent(events.Event{
			Type: events.EventLog, RunID: "run_page",
			Timestamp: time.Now(),
			Data:      events.EventData{Level: "info", Message: "msg"},
		})
	}

	page1, _ := s.GetEvents("run_page", EventFilter{Limit: 5})
	if len(page1) != 5 {
		t.Errorf("page1: got %d, want 5", len(page1))
	}

	page2, _ := s.GetEvents("run_page", EventFilter{Limit: 5, Offset: 5})
	if len(page2) != 5 {
		t.Errorf("page2: got %d, want 5", len(page2))
	}

	if page1[0].ID == page2[0].ID {
		t.Error("page1 and page2 should have different events")
	}
}

package engine

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/rhystic/attractor/pkg/dot"
	"github.com/rhystic/attractor/pkg/events"
	"github.com/rhystic/attractor/pkg/store"
)

// TestIntegrationPipelineStore runs a simulated pipeline end-to-end and
// verifies that the store contains the expected run, events, and context
// snapshots produced during execution.
func TestIntegrationPipelineStore(t *testing.T) {
	const dotSrc = `digraph Integration {
		graph [goal="integration test"];
		start [shape=Mdiamond];
		plan  [label="Plan the work"];
		impl  [label="Implement"];
		exit  [shape=Msquare];
		start -> plan -> impl -> exit;
	}`

	graph, err := dot.Parse(dotSrc)
	if err != nil {
		t.Fatalf("parse DOT: %v", err)
	}

	// Open in-memory store (temp file so WAL works)
	dbPath := filepath.Join(t.TempDir(), "integration.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	// Build engine (nil LLM client = simulation mode)
	cfg := DefaultConfig()
	cfg.LogsRoot = t.TempDir()

	eng := New(graph, nil, cfg)
	defer eng.Close()
	eng.Store = db

	// Subscribe a persistence goroutine (mirrors what main.go does)
	persistCh := eng.Subscribe()
	go db.PersistEvents(persistCh, eng.Model())

	// Run the pipeline
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	outcome, err := eng.Run(ctx)
	if err != nil {
		t.Fatalf("engine run: %v", err)
	}
	if outcome.Status != "success" {
		t.Fatalf("expected success, got %s: %s", outcome.Status, outcome.FailureReason)
	}

	// Close the emitter so the PersistEvents goroutine drains and exits
	eng.Close()
	// Small sleep to let goroutine finish draining
	time.Sleep(100 * time.Millisecond)

	runID := eng.RunID()

	// ---------------------------------------------------------------
	// Verify: run record
	// ---------------------------------------------------------------
	t.Run("RunRecord", func(t *testing.T) {
		run, err := db.GetRun(runID)
		if err != nil {
			t.Fatalf("get run: %v", err)
		}
		if run.Mode != "pipeline" {
			t.Errorf("Mode: got %q, want %q", run.Mode, "pipeline")
		}
		if run.GraphName != "Integration" {
			t.Errorf("GraphName: got %q, want %q", run.GraphName, "Integration")
		}
		if run.Goal != "integration test" {
			t.Errorf("Goal: got %q, want %q", run.Goal, "integration test")
		}
		if run.Status != "success" {
			t.Errorf("Status: got %q, want %q", run.Status, "success")
		}
		if run.EndedAt == nil {
			t.Error("EndedAt should be set")
		}
		if run.DurationMs < 0 {
			t.Errorf("DurationMs: got %d, want >= 0", run.DurationMs)
		}
	})

	// ---------------------------------------------------------------
	// Verify: events were persisted
	// ---------------------------------------------------------------
	t.Run("Events", func(t *testing.T) {
		allEvents, err := db.GetEvents(runID, store.EventFilter{})
		if err != nil {
			t.Fatalf("get events: %v", err)
		}
		if len(allEvents) == 0 {
			t.Fatal("expected events, got none")
		}

		// Count expected event types
		typeCounts := make(map[events.EventType]int)
		for _, e := range allEvents {
			typeCounts[e.EventType]++
		}

		// We expect at least: pipeline_start, pipeline_end,
		// node_start (for start, plan, impl), node_end (for start, plan, impl),
		// edge_selected (start->plan, plan->impl, impl->exit),
		// llm_start (for plan and impl — codergen nodes in sim mode)
		assertMinCount := func(et events.EventType, min int) {
			t.Helper()
			if typeCounts[et] < min {
				t.Errorf("%s: got %d, want >= %d", et, typeCounts[et], min)
			}
		}

		assertMinCount(events.EventPipelineStart, 1)
		assertMinCount(events.EventPipelineEnd, 1)
		assertMinCount(events.EventNodeStart, 3) // start, plan, impl
		assertMinCount(events.EventNodeEnd, 3)
		assertMinCount(events.EventEdgeSelected, 3) // start->plan, plan->impl, impl->exit
		assertMinCount(events.EventLLMStart, 2)     // plan, impl (codergen sim mode emits LLMStart)
	})

	// ---------------------------------------------------------------
	// Verify: events filtered by node
	// ---------------------------------------------------------------
	t.Run("EventsByNode", func(t *testing.T) {
		planEvents, err := db.GetEvents(runID, store.EventFilter{NodeID: "plan"})
		if err != nil {
			t.Fatalf("get plan events: %v", err)
		}
		if len(planEvents) == 0 {
			t.Error("expected events for node 'plan', got none")
		}

		// All returned events should have node_id = "plan"
		for _, e := range planEvents {
			if e.NodeID != "plan" {
				t.Errorf("event %d has NodeID %q, want %q", e.ID, e.NodeID, "plan")
			}
		}
	})

	// ---------------------------------------------------------------
	// Verify: context snapshots
	// ---------------------------------------------------------------
	t.Run("ContextSnapshots", func(t *testing.T) {
		// The engine saves a snapshot after each non-terminal node: start, plan, impl
		for _, nodeID := range []string{"start", "plan", "impl"} {
			snap, err := db.GetContextSnapshot(runID, nodeID)
			if err != nil {
				t.Errorf("get context snapshot for %q: %v", nodeID, err)
				continue
			}
			if snap.RunID != runID {
				t.Errorf("snapshot RunID: got %q, want %q", snap.RunID, runID)
			}
			if snap.NodeID != nodeID {
				t.Errorf("snapshot NodeID: got %q, want %q", snap.NodeID, nodeID)
			}
			if len(snap.CompletedNodes) == 0 {
				t.Errorf("snapshot for %q: no completed nodes", nodeID)
			}
		}

		// The "impl" snapshot should list all three nodes as completed
		implSnap, err := db.GetContextSnapshot(runID, "impl")
		if err != nil {
			t.Fatalf("get impl snapshot: %v", err)
		}
		if len(implSnap.CompletedNodes) != 3 {
			t.Errorf("impl completed nodes: got %d, want 3", len(implSnap.CompletedNodes))
		}

		// Context should contain goal from graph mirroring
		if goal, ok := implSnap.Snapshot["goal"]; !ok {
			t.Error("impl snapshot missing 'goal' key")
		} else if goal != "integration test" {
			t.Errorf("impl snapshot goal: got %v, want %q", goal, "integration test")
		}
	})

	// ---------------------------------------------------------------
	// Verify: run summary aggregation
	// ---------------------------------------------------------------
	t.Run("RunSummary", func(t *testing.T) {
		summary, err := db.GetRunSummary(runID)
		if err != nil {
			t.Fatalf("get run summary: %v", err)
		}
		if summary.Run.ID != runID {
			t.Errorf("summary Run.ID: got %q, want %q", summary.Run.ID, runID)
		}
		if summary.EventCount == 0 {
			t.Error("summary EventCount should be > 0")
		}
		if summary.NodeCount < 3 {
			t.Errorf("summary NodeCount: got %d, want >= 3", summary.NodeCount)
		}
	})

	// ---------------------------------------------------------------
	// Verify: node execution order
	// ---------------------------------------------------------------
	t.Run("ListRunNodes", func(t *testing.T) {
		nodes, err := db.ListRunNodes(runID)
		if err != nil {
			t.Fatalf("list run nodes: %v", err)
		}
		if len(nodes) < 3 {
			t.Fatalf("got %d nodes, want >= 3", len(nodes))
		}
		// First node should be "start"
		if nodes[0] != "start" {
			t.Errorf("first node: got %q, want %q", nodes[0], "start")
		}
	})

	// ---------------------------------------------------------------
	// Verify: ListRuns returns our run
	// ---------------------------------------------------------------
	t.Run("ListRuns", func(t *testing.T) {
		runs, err := db.ListRuns(10)
		if err != nil {
			t.Fatalf("list runs: %v", err)
		}
		if len(runs) != 1 {
			t.Fatalf("got %d runs, want 1", len(runs))
		}
		if runs[0].ID != runID {
			t.Errorf("run ID: got %q, want %q", runs[0].ID, runID)
		}
	})
}

// TestIntegrationPipelineBranchingStore verifies event persistence for a
// branching pipeline with a decision node, ensuring edge_selected events
// record the routing decision.
func TestIntegrationPipelineBranchingStore(t *testing.T) {
	const dotSrc = `digraph Branching {
		graph [goal="branch test"];
		start [shape=Mdiamond];
		gate  [shape=diamond, label="Decision"];
		pathA [label="Path A"];
		exit  [shape=Msquare];
		start -> gate;
		gate -> pathA [label="go", weight="10"];
		gate -> exit  [label="skip", weight="1"];
		pathA -> exit;
	}`

	graph, err := dot.Parse(dotSrc)
	if err != nil {
		t.Fatalf("parse DOT: %v", err)
	}

	dbPath := filepath.Join(t.TempDir(), "branch.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	cfg := DefaultConfig()
	cfg.LogsRoot = t.TempDir()

	eng := New(graph, nil, cfg)
	defer eng.Close()
	eng.Store = db

	persistCh := eng.Subscribe()
	go db.PersistEvents(persistCh, eng.Model())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	outcome, err := eng.Run(ctx)
	if err != nil {
		t.Fatalf("engine run: %v", err)
	}
	if outcome.Status != "success" {
		t.Fatalf("expected success, got %s", outcome.Status)
	}

	eng.Close()
	time.Sleep(100 * time.Millisecond)

	runID := eng.RunID()

	// Verify edge_selected events include the branching decision
	t.Run("EdgeSelectedEvents", func(t *testing.T) {
		edgeEvents, err := db.GetEvents(runID, store.EventFilter{
			EventType: string(events.EventEdgeSelected),
		})
		if err != nil {
			t.Fatalf("get edge events: %v", err)
		}
		if len(edgeEvents) < 2 {
			t.Fatalf("got %d edge_selected events, want >= 2", len(edgeEvents))
		}

		// The gate should route to pathA (higher weight)
		var gateEdge *store.StoredEvent
		for i := range edgeEvents {
			if edgeEvents[i].Data.FromNode == "gate" {
				gateEdge = &edgeEvents[i]
				break
			}
		}
		if gateEdge == nil {
			t.Fatal("no edge_selected event from gate node")
		}
		if gateEdge.Data.ToNode != "pathA" {
			t.Errorf("gate routed to %q, want %q", gateEdge.Data.ToNode, "pathA")
		}
	})

	// Verify all executed nodes appear
	t.Run("ExecutedNodes", func(t *testing.T) {
		nodes, err := db.ListRunNodes(runID)
		if err != nil {
			t.Fatalf("list run nodes: %v", err)
		}
		expected := map[string]bool{"start": true, "gate": true, "pathA": true}
		for _, n := range nodes {
			delete(expected, n)
		}
		for missing := range expected {
			t.Errorf("missing executed node: %q", missing)
		}
	})
}

// TestIntegrationPipelineNoStore ensures the engine runs correctly when
// Store is nil (backward compatibility).
func TestIntegrationPipelineNoStore(t *testing.T) {
	const dotSrc = `digraph NoStore {
		start [shape=Mdiamond];
		task  [label="Task"];
		exit  [shape=Msquare];
		start -> task -> exit;
	}`

	graph, err := dot.Parse(dotSrc)
	if err != nil {
		t.Fatalf("parse DOT: %v", err)
	}

	cfg := DefaultConfig()
	cfg.LogsRoot = t.TempDir()

	eng := New(graph, nil, cfg)
	defer eng.Close()

	// Store is nil by default — this should not panic or error
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	outcome, err := eng.Run(ctx)
	if err != nil {
		t.Fatalf("engine run: %v", err)
	}
	if outcome.Status != "success" {
		t.Fatalf("expected success, got %s: %s", outcome.Status, outcome.FailureReason)
	}
}

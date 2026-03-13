package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pcontext "github.com/rhystic/attractor/pkg/context"
	"github.com/rhystic/attractor/pkg/dot"
	"github.com/rhystic/attractor/pkg/events"
)

// TestIntegrationRolePipeline runs a pipeline with mixed roles in simulation
// mode and verifies each node references its role correctly.
func TestIntegrationRolePipeline(t *testing.T) {
	const dotSrc = `digraph RolePipeline {
		graph [goal="Implement a reviewed feature"];
		start [shape=Mdiamond];

		research [
			label="Research Codebase",
			role="researcher",
			prompt="Explore the project structure and document key findings."
		];

		plan [
			label="Create Work Plan",
			role="project-manager",
			prompt="Break down the goal into tasks based on research findings."
		];

		implement [
			label="Implement Feature",
			role="developer",
			prompt="Implement the feature according to the plan."
		];

		test [
			label="Run Tests",
			role="quality",
			prompt="Write and run tests for the implemented feature."
		];

		review [
			label="Code Review",
			role="reviewer",
			prompt="Review the code changes for correctness and style."
		];

		exit [shape=Msquare];

		start -> research -> plan -> implement -> test -> review -> exit;
	}`

	graph, err := dot.Parse(dotSrc)
	if err != nil {
		t.Fatalf("parse DOT: %v", err)
	}

	cfg := DefaultConfig()
	cfg.LogsRoot = t.TempDir()

	eng := New(graph, nil, cfg)
	defer eng.Close()

	// Collect events
	eventCh := eng.Subscribe()
	var collected []events.Event
	done := make(chan struct{})
	go func() {
		for event := range eventCh {
			collected = append(collected, event)
		}
		close(done)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	outcome, err := eng.Run(ctx)
	if err != nil {
		t.Fatalf("engine run: %v", err)
	}
	if outcome.Status != pcontext.StatusSuccess {
		t.Fatalf("expected success, got %s: %s", outcome.Status, outcome.FailureReason)
	}

	eng.Close()
	<-done

	// The engine creates a subdirectory under LogsRoot named by run ID.
	runLogsRoot := filepath.Join(cfg.LogsRoot, eng.RunID())

	// Verify all 5 role nodes executed
	t.Run("AllNodesExecuted", func(t *testing.T) {
		expectedNodes := []string{"research", "plan", "implement", "test", "review"}
		for _, nodeID := range expectedNodes {
			found := false
			for _, e := range collected {
				if e.Type == events.EventNodeEnd && e.NodeID == nodeID {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("node %q was not executed", nodeID)
			}
		}
	})

	// Verify simulated responses include role names
	t.Run("RoleInSimulatedResponse", func(t *testing.T) {
		roleNodes := map[string]string{
			"research":  "researcher",
			"plan":      "project-manager",
			"implement": "developer",
			"test":      "quality",
			"review":    "reviewer",
		}
		for nodeID, roleName := range roleNodes {
			responsePath := filepath.Join(runLogsRoot, nodeID, "response.md")
			content, err := os.ReadFile(responsePath)
			if err != nil {
				t.Errorf("read response for %q: %v", nodeID, err)
				continue
			}
			if !strings.Contains(string(content), "(role: "+roleName+")") {
				t.Errorf("response for %q does not contain role %q: %s", nodeID, roleName, string(content))
			}
		}
	})

	// Verify prompts were written with {file:} expansion intact (no errors)
	t.Run("PromptsWritten", func(t *testing.T) {
		for _, nodeID := range []string{"research", "plan", "implement", "test", "review"} {
			promptPath := filepath.Join(runLogsRoot, nodeID, "prompt.md")
			content, err := os.ReadFile(promptPath)
			if err != nil {
				t.Errorf("read prompt for %q: %v", nodeID, err)
				continue
			}
			if len(content) == 0 {
				t.Errorf("empty prompt for %q", nodeID)
			}
		}
	})

	// Verify execution order via node_start events
	t.Run("ExecutionOrder", func(t *testing.T) {
		var order []string
		for _, e := range collected {
			if e.Type == events.EventNodeStart && e.NodeID != "start" {
				order = append(order, e.NodeID)
			}
		}
		expected := []string{"research", "plan", "implement", "test", "review"}
		if len(order) < len(expected) {
			t.Fatalf("got %d node starts, want %d: %v", len(order), len(expected), order)
		}
		for i, exp := range expected {
			if i >= len(order) {
				break
			}
			if order[i] != exp {
				t.Errorf("order[%d] = %q, want %q", i, order[i], exp)
			}
		}
	})
}

// TestIntegrationRoleNodeAttributes verifies that Role() returns
// the correct value from parsed DOT attributes.
func TestIntegrationRoleNodeAttributes(t *testing.T) {
	const dotSrc = `digraph Roles {
		start [shape=Mdiamond];
		a [role="developer"];
		b [role="reviewer"];
		c [label="No Role"];
		exit [shape=Msquare];
		start -> a -> b -> c -> exit;
	}`

	graph, err := dot.Parse(dotSrc)
	if err != nil {
		t.Fatalf("parse DOT: %v", err)
	}

	tests := []struct {
		nodeID   string
		expected string
	}{
		{"a", "developer"},
		{"b", "reviewer"},
		{"c", ""},
		{"start", ""},
	}

	for _, tt := range tests {
		t.Run(tt.nodeID, func(t *testing.T) {
			node, ok := graph.Nodes[tt.nodeID]
			if !ok {
				t.Fatalf("node %q not found", tt.nodeID)
			}
			if got := node.Role(); got != tt.expected {
				t.Errorf("Role() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// TestIntegrationRolePipelineWithDevops tests the devops role in a pipeline.
func TestIntegrationRolePipelineWithDevops(t *testing.T) {
	const dotSrc = `digraph Deploy {
		graph [goal="Deploy the application"];
		start [shape=Mdiamond];
		build [label="Build", role="developer", prompt="Build the application."];
		deploy [label="Deploy", role="devops", prompt="Deploy to production."];
		exit [shape=Msquare];
		start -> build -> deploy -> exit;
	}`

	graph, err := dot.Parse(dotSrc)
	if err != nil {
		t.Fatalf("parse DOT: %v", err)
	}

	cfg := DefaultConfig()
	cfg.LogsRoot = t.TempDir()

	eng := New(graph, nil, cfg)
	defer eng.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	outcome, err := eng.Run(ctx)
	if err != nil {
		t.Fatalf("engine run: %v", err)
	}
	if outcome.Status != pcontext.StatusSuccess {
		t.Fatalf("expected success, got %s: %s", outcome.Status, outcome.FailureReason)
	}

	// The engine creates a subdirectory under LogsRoot named by run ID.
	runLogsRoot := filepath.Join(cfg.LogsRoot, eng.RunID())

	// Verify deploy response includes devops role
	content, err := os.ReadFile(filepath.Join(runLogsRoot, "deploy", "response.md"))
	if err != nil {
		t.Fatalf("read deploy response: %v", err)
	}
	if !strings.Contains(string(content), "(role: devops)") {
		t.Errorf("deploy response missing devops role: %s", string(content))
	}
}

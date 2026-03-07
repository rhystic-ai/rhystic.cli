package engine

import (
	"context"
	"testing"

	pcontext "github.com/rhystic/attractor/pkg/context"
	"github.com/rhystic/attractor/pkg/dot"
)

func TestEngineSimplePipeline(t *testing.T) {
	input := `digraph Simple {
		start [shape=Mdiamond]
		exit [shape=Msquare]
		task [label="Simple Task", auto_status="true"]
		start -> task -> exit
	}`

	graph, err := dot.Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	cfg := DefaultConfig()
	cfg.LogsRoot = t.TempDir()

	eng := New(graph, nil, cfg) // nil client = simulation mode
	defer eng.Close()

	outcome, err := eng.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if outcome.Status != pcontext.StatusSuccess {
		t.Errorf("Expected success, got %s: %s", outcome.Status, outcome.FailureReason)
	}
}

func TestEngineBranchingPipeline(t *testing.T) {
	input := `digraph Branch {
		start [shape=Mdiamond]
		exit [shape=Msquare]
		task [label="Task"]
		gate [shape=diamond]
		
		start -> task -> gate
		gate -> exit [label="done", weight="10"]
		gate -> task [label="retry"]
	}`

	graph, err := dot.Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	cfg := DefaultConfig()
	cfg.LogsRoot = t.TempDir()

	eng := New(graph, nil, cfg)
	defer eng.Close()

	outcome, err := eng.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Should take highest weight edge (done) and exit
	if outcome.Status != pcontext.StatusSuccess {
		t.Errorf("Expected success, got %s", outcome.Status)
	}
}

func TestEngineConditionEvaluation(t *testing.T) {
	tests := []struct {
		name      string
		condition string
		outcome   pcontext.Outcome
		expected  bool
	}{
		{
			name:      "outcome equals success",
			condition: "outcome=success",
			outcome:   pcontext.NewSuccessOutcome(""),
			expected:  true,
		},
		{
			name:      "outcome equals fail",
			condition: "outcome=fail",
			outcome:   pcontext.NewFailOutcome(""),
			expected:  true,
		},
		{
			name:      "outcome not equals",
			condition: "outcome!=success",
			outcome:   pcontext.NewFailOutcome(""),
			expected:  true,
		},
		{
			name:      "outcome not equals false",
			condition: "outcome!=success",
			outcome:   pcontext.NewSuccessOutcome(""),
			expected:  false,
		},
	}

	graph := &dot.Graph{
		Nodes: make(map[string]*dot.Node),
	}

	cfg := DefaultConfig()
	cfg.LogsRoot = t.TempDir()

	eng := New(graph, nil, cfg)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := eng.evaluateCondition(tt.condition, tt.outcome)
			if result != tt.expected {
				t.Errorf("evaluateCondition(%q) = %v, want %v", tt.condition, result, tt.expected)
			}
		})
	}
}

func TestEngineEdgeSelection(t *testing.T) {
	input := `digraph Test {
		start [shape=Mdiamond]
		exit [shape=Msquare]
		A [label="A"]
		B [label="B"]
		C [label="C"]
		
		start -> A
		A -> B [label="low", weight="1"]
		A -> C [label="high", weight="10"]
		B -> exit
		C -> exit
	}`

	graph, err := dot.Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	cfg := DefaultConfig()
	cfg.LogsRoot = t.TempDir()

	eng := New(graph, nil, cfg)

	nodeA := graph.Nodes["A"]
	outcome := pcontext.NewSuccessOutcome("")

	edge := eng.selectEdge(nodeA, outcome)
	if edge == nil {
		t.Fatal("Expected edge to be selected")
	}

	// Should select C (higher weight)
	if edge.To != "C" {
		t.Errorf("Expected edge to C (weight 10), got %s", edge.To)
	}
}

func TestEngineEdgeSelectionWithCondition(t *testing.T) {
	input := `digraph Test {
		start [shape=Mdiamond]
		exit [shape=Msquare]
		A [label="A"]
		B [label="B"]
		C [label="C"]
		
		start -> A
		A -> B [label="on success", condition="outcome=success"]
		A -> C [label="on fail", condition="outcome=fail"]
		B -> exit
		C -> exit
	}`

	graph, err := dot.Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	cfg := DefaultConfig()
	cfg.LogsRoot = t.TempDir()

	eng := New(graph, nil, cfg)

	nodeA := graph.Nodes["A"]

	// Success should go to B
	successOutcome := pcontext.NewSuccessOutcome("")
	edge := eng.selectEdge(nodeA, successOutcome)
	if edge.To != "B" {
		t.Errorf("Expected edge to B on success, got %s", edge.To)
	}

	// Fail should go to C
	failOutcome := pcontext.NewFailOutcome("")
	edge = eng.selectEdge(nodeA, failOutcome)
	if edge.To != "C" {
		t.Errorf("Expected edge to C on fail, got %s", edge.To)
	}
}

func TestEngineEdgeSelectionWithPreferredLabel(t *testing.T) {
	input := `digraph Test {
		start [shape=Mdiamond]
		exit [shape=Msquare]
		A [label="A"]
		B [label="B"]
		C [label="C"]
		
		start -> A
		A -> B [label="[Y] Yes"]
		A -> C [label="[N] No"]
		B -> exit
		C -> exit
	}`

	graph, err := dot.Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	cfg := DefaultConfig()
	cfg.LogsRoot = t.TempDir()

	eng := New(graph, nil, cfg)

	nodeA := graph.Nodes["A"]

	// Preferred label "yes" should match "[Y] Yes"
	outcome := pcontext.NewSuccessOutcome("").WithPreferredLabel("yes")
	edge := eng.selectEdge(nodeA, outcome)
	if edge.To != "B" {
		t.Errorf("Expected edge to B with preferred 'yes', got %s", edge.To)
	}

	// Preferred label "no" should match "[N] No"
	outcome = pcontext.NewSuccessOutcome("").WithPreferredLabel("no")
	edge = eng.selectEdge(nodeA, outcome)
	if edge.To != "C" {
		t.Errorf("Expected edge to C with preferred 'no', got %s", edge.To)
	}
}

func TestEngineEdgeSelectionWithSuggestedNexts(t *testing.T) {
	input := `digraph Test {
		start [shape=Mdiamond]
		exit [shape=Msquare]
		A [label="A"]
		B [label="B"]
		C [label="C"]
		
		start -> A
		A -> B
		A -> C
		B -> exit
		C -> exit
	}`

	graph, err := dot.Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	cfg := DefaultConfig()
	cfg.LogsRoot = t.TempDir()

	eng := New(graph, nil, cfg)

	nodeA := graph.Nodes["A"]

	// Suggested next "C" should route to C
	outcome := pcontext.NewSuccessOutcome("").WithSuggestedNexts("C")
	edge := eng.selectEdge(nodeA, outcome)
	if edge.To != "C" {
		t.Errorf("Expected edge to C with suggested next, got %s", edge.To)
	}
}

func TestEngineBestByWeightThenLexical(t *testing.T) {
	input := `digraph Test {
		start [shape=Mdiamond]
		exit [shape=Msquare]
		A [label="A"]
		B [label="B"]
		C [label="C"]
		D [label="D"]
		
		start -> A
		A -> B [weight="5"]
		A -> C [weight="5"]
		A -> D [weight="3"]
		B -> exit
		C -> exit
		D -> exit
	}`

	graph, err := dot.Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	cfg := DefaultConfig()
	cfg.LogsRoot = t.TempDir()

	eng := New(graph, nil, cfg)

	nodeA := graph.Nodes["A"]
	outcome := pcontext.NewSuccessOutcome("")

	edge := eng.selectEdge(nodeA, outcome)

	// B and C have same weight (5), B comes first lexically
	if edge.To != "B" {
		t.Errorf("Expected edge to B (same weight, lexical first), got %s", edge.To)
	}
}

func TestEngineContextMirroring(t *testing.T) {
	input := `digraph Test {
		graph [goal="Test Goal", custom="value"]
		start [shape=Mdiamond]
		exit [shape=Msquare]
		start -> exit
	}`

	graph, err := dot.Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	cfg := DefaultConfig()
	cfg.LogsRoot = t.TempDir()

	eng := New(graph, nil, cfg)

	eng.mirrorGraphAttributes()

	if eng.Context.GetString("goal") != "Test Goal" {
		t.Error("Expected goal to be mirrored")
	}

	if eng.Context.GetString("graph.custom") != "value" {
		t.Error("Expected custom attribute to be mirrored")
	}
}

func TestEngineGoalGates(t *testing.T) {
	input := `digraph Test {
		start [shape=Mdiamond]
		exit [shape=Msquare]
		critical [label="Critical", goal_gate="true"]
		start -> critical -> exit
	}`

	graph, err := dot.Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	cfg := DefaultConfig()
	cfg.LogsRoot = t.TempDir()

	eng := New(graph, nil, cfg)

	// Simulate completed critical node with failure
	eng.completedNodes = []string{"critical"}
	eng.nodeOutcomes = map[string]pcontext.Outcome{
		"critical": pcontext.NewFailOutcome("Failed"),
	}

	ok, failed := eng.checkGoalGates()
	if ok {
		t.Error("Expected goal gate check to fail")
	}
	if failed == nil || failed.ID != "critical" {
		t.Error("Expected failed gate to be 'critical'")
	}

	// Success case
	eng.nodeOutcomes["critical"] = pcontext.NewSuccessOutcome("")
	ok, failed = eng.checkGoalGates()
	if !ok {
		t.Error("Expected goal gate check to pass")
	}
	if failed != nil {
		t.Error("Expected no failed gate")
	}
}

func TestEngineRetryTarget(t *testing.T) {
	input := `digraph Test {
		graph [retry_target="plan", fallback_retry_target="start"]
		start [shape=Mdiamond]
		exit [shape=Msquare]
		plan [label="Plan"]
		critical [label="Critical", goal_gate="true", retry_target="plan"]
		start -> plan -> critical -> exit
	}`

	graph, err := dot.Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	cfg := DefaultConfig()
	cfg.LogsRoot = t.TempDir()

	eng := New(graph, nil, cfg)

	// Node-level retry target
	criticalNode := graph.Nodes["critical"]
	target := eng.getRetryTarget(criticalNode)
	if target != "plan" {
		t.Errorf("Expected retry target 'plan', got '%s'", target)
	}

	// Graph-level fallback
	planNode := graph.Nodes["plan"]
	target = eng.getRetryTarget(planNode)
	if target != "plan" {
		t.Errorf("Expected graph retry target 'plan', got '%s'", target)
	}
}

func TestEngineBackoff(t *testing.T) {
	cfg := DefaultConfig()
	eng := &Engine{Config: cfg}

	delays := []struct {
		attempt  int
		expected int64 // approximate milliseconds
	}{
		{1, 200},
		{2, 400},
		{3, 800},
		{4, 1600},
		{5, 3200},
	}

	for _, d := range delays {
		delay := eng.calculateBackoff(d.attempt)
		delayMs := delay.Milliseconds()

		// Allow some variance
		if delayMs < d.expected/2 || delayMs > d.expected*2 {
			t.Errorf("calculateBackoff(%d) = %dms, expected ~%dms", d.attempt, delayMs, d.expected)
		}
	}
}

func TestNormalizeLabel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello", "hello"},
		{"[Y] Yes, continue", "yes, continue"},
		{"Y) Yes", "yes"},
		{"Y - Yes", "yes"},
		{"  Spaces  ", "spaces"},
		{"[A] Approve changes", "approve changes"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeLabel(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeLabel(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.LogsRoot == "" {
		t.Error("Expected default logs root")
	}
	if cfg.MaxRetries <= 0 {
		t.Error("Expected positive max retries")
	}
	if !cfg.CheckpointAfter {
		t.Error("Expected checkpoint after to be true")
	}
}

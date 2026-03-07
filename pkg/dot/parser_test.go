package dot

import (
	"testing"
	"time"
)

func TestParseSimpleGraph(t *testing.T) {
	input := `digraph Simple {
		start [shape=Mdiamond, label="Start"]
		exit  [shape=Msquare, label="Exit"]
		start -> exit
	}`

	graph, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if graph.Name != "Simple" {
		t.Errorf("Expected graph name 'Simple', got '%s'", graph.Name)
	}

	if len(graph.Nodes) != 2 {
		t.Errorf("Expected 2 nodes, got %d", len(graph.Nodes))
	}

	if len(graph.Edges) != 1 {
		t.Errorf("Expected 1 edge, got %d", len(graph.Edges))
	}

	// Check start node
	start := graph.Nodes["start"]
	if start == nil {
		t.Fatal("Start node not found")
	}
	if start.Shape() != "Mdiamond" {
		t.Errorf("Expected start shape 'Mdiamond', got '%s'", start.Shape())
	}
	if start.Label() != "Start" {
		t.Errorf("Expected start label 'Start', got '%s'", start.Label())
	}

	// Check exit node
	exit := graph.Nodes["exit"]
	if exit == nil {
		t.Fatal("Exit node not found")
	}
	if exit.Shape() != "Msquare" {
		t.Errorf("Expected exit shape 'Msquare', got '%s'", exit.Shape())
	}
}

func TestParseGraphAttributes(t *testing.T) {
	input := `digraph Test {
		graph [goal="Build a feature", default_max_retry="10"]
		start [shape=Mdiamond]
		exit [shape=Msquare]
		start -> exit
	}`

	graph, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if graph.Goal() != "Build a feature" {
		t.Errorf("Expected goal 'Build a feature', got '%s'", graph.Goal())
	}

	if graph.DefaultMaxRetry() != 10 {
		t.Errorf("Expected default_max_retry 10, got %d", graph.DefaultMaxRetry())
	}
}

func TestParseNodeAttributes(t *testing.T) {
	input := `digraph Test {
		start [shape=Mdiamond]
		exit [shape=Msquare]
		
		task [
			label="My Task",
			prompt="Do something important",
			max_retries="3",
			goal_gate="true",
			timeout="900s",
			class="code,important"
		]
		
		start -> task -> exit
	}`

	graph, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	task := graph.Nodes["task"]
	if task == nil {
		t.Fatal("Task node not found")
	}

	if task.Label() != "My Task" {
		t.Errorf("Expected label 'My Task', got '%s'", task.Label())
	}

	if task.Prompt() != "Do something important" {
		t.Errorf("Expected prompt 'Do something important', got '%s'", task.Prompt())
	}

	if task.MaxRetries() != 3 {
		t.Errorf("Expected max_retries 3, got %d", task.MaxRetries())
	}

	if !task.GoalGate() {
		t.Error("Expected goal_gate to be true")
	}

	if task.Timeout() != 900*time.Second {
		t.Errorf("Expected timeout 900s, got %v", task.Timeout())
	}

	classes := task.Class()
	if len(classes) != 2 || classes[0] != "code" || classes[1] != "important" {
		t.Errorf("Expected classes [code, important], got %v", classes)
	}
}

func TestParseEdgeAttributes(t *testing.T) {
	input := `digraph Test {
		start [shape=Mdiamond]
		exit [shape=Msquare]
		
		A -> B [label="success", condition="outcome=success", weight="10"]
		B -> exit [label="done"]
		start -> A
	}`

	graph, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Find A -> B edge
	var abEdge *Edge
	for _, e := range graph.Edges {
		if e.From == "A" && e.To == "B" {
			abEdge = e
			break
		}
	}

	if abEdge == nil {
		t.Fatal("A -> B edge not found")
	}

	if abEdge.Label() != "success" {
		t.Errorf("Expected label 'success', got '%s'", abEdge.Label())
	}

	if abEdge.Condition() != "outcome=success" {
		t.Errorf("Expected condition 'outcome=success', got '%s'", abEdge.Condition())
	}

	if abEdge.Weight() != 10 {
		t.Errorf("Expected weight 10, got %d", abEdge.Weight())
	}
}

func TestParseChainedEdges(t *testing.T) {
	input := `digraph Test {
		start [shape=Mdiamond]
		exit [shape=Msquare]
		A -> B -> C -> exit [label="chain"]
		start -> A
	}`

	graph, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Should have 4 edges: start->A, A->B, B->C, C->exit
	if len(graph.Edges) != 4 {
		t.Errorf("Expected 4 edges, got %d", len(graph.Edges))
	}

	// Check that chain edges have the label
	chainCount := 0
	for _, e := range graph.Edges {
		if e.Label() == "chain" {
			chainCount++
		}
	}
	if chainCount != 3 {
		t.Errorf("Expected 3 chain-labeled edges, got %d", chainCount)
	}
}

func TestParseSubgraph(t *testing.T) {
	input := `digraph Test {
		start [shape=Mdiamond]
		exit [shape=Msquare]
		
		subgraph cluster_loop {
			graph [label="Loop A"]
			node [thread_id="loop-a", timeout="900s"]
			
			Plan [label="Plan next step"]
			Implement [label="Implement", timeout="1800s"]
			
			Plan -> Implement
		}
		
		start -> Plan
		Implement -> exit
	}`

	graph, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(graph.Subgraphs) != 1 {
		t.Errorf("Expected 1 subgraph, got %d", len(graph.Subgraphs))
	}

	sg := graph.Subgraphs[0]
	if sg.Name != "cluster_loop" {
		t.Errorf("Expected subgraph name 'cluster_loop', got '%s'", sg.Name)
	}

	// Check that nodes inherited defaults
	plan := graph.Nodes["Plan"]
	if plan == nil {
		t.Fatal("Plan node not found")
	}
	if plan.ThreadID() != "loop-a" {
		t.Errorf("Expected Plan thread_id 'loop-a', got '%s'", plan.ThreadID())
	}
	if plan.Timeout() != 900*time.Second {
		t.Errorf("Expected Plan timeout 900s, got %v", plan.Timeout())
	}

	// Check override
	impl := graph.Nodes["Implement"]
	if impl == nil {
		t.Fatal("Implement node not found")
	}
	if impl.Timeout() != 1800*time.Second {
		t.Errorf("Expected Implement timeout 1800s, got %v", impl.Timeout())
	}
}

func TestParseComments(t *testing.T) {
	input := `// This is a line comment
digraph Test {
	/* This is a block comment */
	start [shape=Mdiamond] // inline comment
	exit [shape=Msquare]
	start -> exit
}`

	graph, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(graph.Nodes) != 2 {
		t.Errorf("Expected 2 nodes, got %d", len(graph.Nodes))
	}
}

func TestParseBranchingWorkflow(t *testing.T) {
	input := `digraph Branch {
		graph [goal="Implement and validate a feature"]
		
		start [shape=Mdiamond]
		exit [shape=Msquare]
		
		plan [label="Plan"]
		implement [label="Implement"]
		validate [label="Validate"]
		gate [shape=diamond, label="Tests passing?"]
		
		start -> plan -> implement -> validate -> gate
		gate -> exit [label="Yes", condition="outcome=success"]
		gate -> implement [label="No", condition="outcome!=success"]
	}`

	graph, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Check gate node
	gate := graph.Nodes["gate"]
	if gate == nil {
		t.Fatal("Gate node not found")
	}
	if gate.Shape() != "diamond" {
		t.Errorf("Expected gate shape 'diamond', got '%s'", gate.Shape())
	}

	// Check conditional edges
	gateEdges := graph.OutgoingEdges("gate")
	if len(gateEdges) != 2 {
		t.Errorf("Expected 2 gate edges, got %d", len(gateEdges))
	}
}

func TestFindStartAndExitNodes(t *testing.T) {
	input := `digraph Test {
		entry [shape=Mdiamond, label="Entry"]
		finish [shape=Msquare, label="Finish"]
		entry -> finish
	}`

	graph, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	start := graph.FindStartNode()
	if start == nil {
		t.Fatal("Start node not found")
	}
	if start.ID != "entry" {
		t.Errorf("Expected start node 'entry', got '%s'", start.ID)
	}

	exit := graph.FindExitNode()
	if exit == nil {
		t.Fatal("Exit node not found")
	}
	if exit.ID != "finish" {
		t.Errorf("Expected exit node 'finish', got '%s'", exit.ID)
	}
}

func TestOutgoingEdges(t *testing.T) {
	input := `digraph Test {
		start [shape=Mdiamond]
		exit [shape=Msquare]
		A [label="A"]
		B [label="B"]
		C [label="C"]
		
		A -> B [label="to B"]
		A -> C [label="to C"]
		B -> exit
		C -> exit
		start -> A
	}`

	graph, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	edges := graph.OutgoingEdges("A")
	if len(edges) != 2 {
		t.Errorf("Expected 2 outgoing edges from A, got %d", len(edges))
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"900s", 900 * time.Second},
		{"15m", 15 * time.Minute},
		{"2h", 2 * time.Hour},
		{"250ms", 250 * time.Millisecond},
		{"1d", 24 * time.Hour},
		{"\"30s\"", 30 * time.Second},
		{"invalid", 0},
		{"", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ParseDuration(tt.input)
			if result != tt.expected {
				t.Errorf("ParseDuration(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDeriveClass(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Loop A", "loop-a"},
		{"Code Review", "code-review"},
		{"Test!", "test"},
		{"Hello World 123", "hello-world-123"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := DeriveClass(tt.input)
			if result != tt.expected {
				t.Errorf("DeriveClass(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseTopLevelAttributes(t *testing.T) {
	input := `digraph Test {
		rankdir = LR
		goal = "Build something"
		start [shape=Mdiamond]
		exit [shape=Msquare]
		start -> exit
	}`

	graph, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if graph.Attributes["rankdir"] != "LR" {
		t.Errorf("Expected rankdir 'LR', got '%s'", graph.Attributes["rankdir"])
	}

	if graph.Goal() != "Build something" {
		t.Errorf("Expected goal 'Build something', got '%s'", graph.Goal())
	}
}

func TestParseStringEscapes(t *testing.T) {
	input := `digraph Test {
		start [shape=Mdiamond]
		exit [shape=Msquare]
		task [label="Line1\nLine2", prompt="Say \"Hello\""]
		start -> task -> exit
	}`

	graph, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	task := graph.Nodes["task"]
	if task.Label() != "Line1\nLine2" {
		t.Errorf("Expected label with newline, got %q", task.Label())
	}

	if task.Prompt() != `Say "Hello"` {
		t.Errorf("Expected prompt with quotes, got %q", task.Prompt())
	}
}

func TestParseError(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"missing digraph", "graph Test {}"},
		{"unterminated string", `digraph Test { a [label="unclosed] }`},
		{"missing brace", `digraph Test { a`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.input)
			if err == nil {
				t.Error("Expected parse error, got nil")
			}
		})
	}
}

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantCmd string
		wantErr bool
	}{
		{
			name:    "no args",
			args:    []string{},
			wantCmd: "help",
		},
		{
			name:    "help flag as command",
			args:    []string{"-h"},
			wantCmd: "-h", // -h alone is treated as a command
		},
		{
			name:    "help command",
			args:    []string{"help"},
			wantCmd: "help",
		},
		{
			name:    "version flag as command",
			args:    []string{"-v"},
			wantCmd: "-v", // -v alone is treated as a command
		},
		{
			name:    "version command",
			args:    []string{"version"},
			wantCmd: "version",
		},
		{
			name:    "run command",
			args:    []string{"run", "-f", "test.dot"},
			wantCmd: "run",
		},
		{
			name:    "run with positional",
			args:    []string{"run", "test.dot"},
			wantCmd: "run",
		},
		{
			name:    "agent command",
			args:    []string{"agent", "Fix the bug"},
			wantCmd: "agent",
		},
		{
			name:    "validate command",
			args:    []string{"validate", "test.dot"},
			wantCmd: "validate",
		},
		{
			name:    "unknown flag",
			args:    []string{"run", "--unknown"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, _, err := parseArgs(tt.args)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if cmd != tt.wantCmd {
				t.Errorf("Expected cmd '%s', got '%s'", tt.wantCmd, cmd)
			}
		})
	}
}

func TestParseArgsOptions(t *testing.T) {
	args := []string{
		"run",
		"-f", "pipeline.dot",
		"-m", "anthropic/claude-opus-4",
		"-l", "/tmp/logs",
		"-t", "1h",
		"--max-retries", "10",
		"--verbose",
		"--no-color",
		"-i",
	}

	cmd, opts, err := parseArgs(args)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if cmd != "run" {
		t.Errorf("Expected cmd 'run'")
	}

	if opts.dotFile != "pipeline.dot" {
		t.Errorf("Expected dotFile 'pipeline.dot', got '%s'", opts.dotFile)
	}

	if opts.model != "anthropic/claude-opus-4" {
		t.Errorf("Expected model 'anthropic/claude-opus-4', got '%s'", opts.model)
	}

	if opts.logsDir != "/tmp/logs" {
		t.Errorf("Expected logsDir '/tmp/logs', got '%s'", opts.logsDir)
	}

	if opts.timeout.Hours() != 1 {
		t.Errorf("Expected timeout 1h, got %v", opts.timeout)
	}

	if opts.maxRetries != 10 {
		t.Errorf("Expected maxRetries 10, got %d", opts.maxRetries)
	}

	if !opts.verbose {
		t.Error("Expected verbose to be true")
	}

	if !opts.noColor {
		t.Error("Expected noColor to be true")
	}

	if !opts.interactive {
		t.Error("Expected interactive to be true")
	}
}

func TestHelpOutput(t *testing.T) {
	var stdout bytes.Buffer

	err := run([]string{"help"}, nil, &stdout, nil)
	if err != nil {
		t.Fatalf("Help failed: %v", err)
	}

	output := stdout.String()

	expectedStrings := []string{
		"Attractor",
		"Usage:",
		"Commands:",
		"run",
		"agent",
		"validate",
		"Options:",
		"OPENROUTER_API_KEY",
		"Examples:",
	}

	for _, s := range expectedStrings {
		if !strings.Contains(output, s) {
			t.Errorf("Help output missing '%s'", s)
		}
	}
}

func TestVersionOutput(t *testing.T) {
	var stdout bytes.Buffer

	err := run([]string{"version"}, nil, &stdout, nil)
	if err != nil {
		t.Fatalf("Version failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "attractor") {
		t.Error("Version output missing 'attractor'")
	}
	if !strings.Contains(output, version) {
		t.Errorf("Version output missing version '%s'", version)
	}
}

func TestValidatePipeline(t *testing.T) {
	tmpDir := t.TempDir()

	// Valid pipeline
	validDot := `digraph Test {
		start [shape=Mdiamond]
		exit [shape=Msquare]
		task [label="Task"]
		start -> task -> exit
	}`

	validPath := filepath.Join(tmpDir, "valid.dot")
	if err := os.WriteFile(validPath, []byte(validDot), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err := run([]string{"validate", validPath}, nil, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Validate valid pipeline failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Validation passed") {
		t.Error("Expected 'Validation passed' in output")
	}

	// Invalid pipeline (missing start)
	invalidDot := `digraph Test {
		exit [shape=Msquare]
		task [label="Task"]
		task -> exit
	}`

	invalidPath := filepath.Join(tmpDir, "invalid.dot")
	if err := os.WriteFile(invalidPath, []byte(invalidDot), 0644); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()
	err = run([]string{"validate", invalidPath}, nil, &stdout, &stderr)
	if err == nil {
		t.Error("Expected validation to fail for invalid pipeline")
	}
}

func TestValidatePipelineDetails(t *testing.T) {
	tmpDir := t.TempDir()

	dotContent := `digraph MyPipeline {
		graph [goal="Build something"]
		start [shape=Mdiamond]
		exit [shape=Msquare]
		task1 [label="Task 1"]
		task2 [label="Task 2"]
		unreachable [label="Unreachable"]
		
		start -> task1 -> task2 -> exit
	}`

	dotPath := filepath.Join(tmpDir, "pipeline.dot")
	if err := os.WriteFile(dotPath, []byte(dotContent), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	err := run([]string{"validate", dotPath}, nil, &stdout, nil)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	output := stdout.String()

	// Check graph info is displayed
	if !strings.Contains(output, "MyPipeline") {
		t.Error("Expected graph name in output")
	}

	// Check warning about unreachable node
	if !strings.Contains(output, "unreachable") {
		t.Error("Expected warning about unreachable node")
	}

	// Check goal is displayed
	if !strings.Contains(output, "Build something") {
		t.Error("Expected goal in output")
	}
}

func TestRunPipelineNoFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run([]string{"run"}, nil, &stdout, &stderr)
	if err == nil {
		t.Error("Expected error when no DOT file specified")
	}
}

func TestRunPipelineFileNotFound(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run([]string{"run", "-f", "/nonexistent/path.dot"}, nil, &stdout, &stderr)
	if err == nil {
		t.Error("Expected error when DOT file not found")
	}
}

func TestAgentNoPrompt(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run([]string{"agent"}, strings.NewReader(""), &stdout, &stderr)
	if err == nil {
		t.Error("Expected error when no prompt provided")
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		input    string
		max      int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello..."},
		{"", 5, ""},
		{"ab", 5, "ab"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := truncateString(tt.input, tt.max)
			if result != tt.expected {
				t.Errorf("truncateString(%q, %d) = %q, want %q", tt.input, tt.max, result, tt.expected)
			}
		})
	}
}

func TestMarkReachable(t *testing.T) {
	dotContent := `digraph Test {
		start [shape=Mdiamond]
		exit [shape=Msquare]
		A [label="A"]
		B [label="B"]
		unreachable [label="Unreachable"]
		
		start -> A -> B -> exit
	}`

	graph, err := parseTestDot(dotContent)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	reachable := make(map[string]bool)
	testMarkReachable(graph, "start", reachable)

	if !reachable["start"] {
		t.Error("start should be reachable")
	}
	if !reachable["A"] {
		t.Error("A should be reachable")
	}
	if !reachable["B"] {
		t.Error("B should be reachable")
	}
	if !reachable["exit"] {
		t.Error("exit should be reachable")
	}
	if reachable["unreachable"] {
		t.Error("unreachable should not be reachable")
	}
}

func parseTestDot(content string) (*testGraph, error) {
	// Simple wrapper for testing
	nodes := make(map[string]bool)
	edges := make(map[string][]string)

	// Extract nodes and edges from content (simplified)
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "->") {
			// Handle chained edges: A -> B -> C
			parts := strings.Split(line, "->")
			for i := 0; i < len(parts)-1; i++ {
				from := strings.TrimSpace(strings.Split(parts[i], "[")[0])
				to := strings.TrimSpace(strings.Split(parts[i+1], "[")[0])
				nodes[from] = true
				nodes[to] = true
				edges[from] = append(edges[from], to)
			}
		} else if strings.Contains(line, "[") {
			name := strings.TrimSpace(strings.Split(line, "[")[0])
			if name != "" && name != "graph" && name != "digraph" {
				nodes[name] = true
			}
		}
	}

	return &testGraph{nodes: nodes, edges: edges}, nil
}

type testGraph struct {
	nodes map[string]bool
	edges map[string][]string
}

// Implement a simplified OutgoingEdges for testing
func (g *testGraph) OutgoingEdges(nodeID string) []string {
	return g.edges[nodeID]
}

// Test version of testMarkReachable that works with our simplified graph
func testMarkReachable(graph *testGraph, nodeID string, reachable map[string]bool) {
	if reachable[nodeID] {
		return
	}
	reachable[nodeID] = true
	for _, to := range graph.OutgoingEdges(nodeID) {
		testMarkReachable(graph, to, reachable)
	}
}

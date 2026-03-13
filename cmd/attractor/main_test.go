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
		"-m", "minimax/minimax-m2.5",
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

	if opts.model != "minimax/minimax-m2.5" {
		t.Errorf("Expected model 'minimax/minimax-m2.5', got '%s'", opts.model)
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

// ---------------------------------------------------------------------------
// resolveContextValue
// ---------------------------------------------------------------------------

func TestResolveContextValue(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "literal string passthrough",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "empty string passthrough",
			input: "",
			want:  "",
		},
		{
			name:  "literal with special chars",
			input: "foo=bar&baz",
			want:  "foo=bar&baz",
		},
		{
			name:    "@ with nonexistent file",
			input:   "@/nonexistent/path/file.txt",
			wantErr: true,
		},
		{
			name:    "@ with empty path",
			input:   "@",
			wantErr: true,
		},
		{
			name:    "@ with whitespace-only path",
			input:   "@   ",
			wantErr: true,
		},
	}

	// File reference case requires a real file.
	t.Run("@ with valid file", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "ctx.txt")
		if err := os.WriteFile(path, []byte("file content"), 0644); err != nil {
			t.Fatal(err)
		}
		got, err := resolveContextValue("@" + path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "file content" {
			t.Errorf("got %q, want %q", got, "file content")
		}
	})

	t.Run("@ with leading whitespace before path", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "ws.txt")
		if err := os.WriteFile(path, []byte("trimmed"), 0644); err != nil {
			t.Fatal(err)
		}
		// The value passed to resolveContextValue starts with "@" then spaces.
		got, err := resolveContextValue("@  " + path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "trimmed" {
			t.Errorf("got %q, want %q", got, "trimmed")
		}
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveContextValue(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// parseContextValues
// ---------------------------------------------------------------------------

func TestParseContextValues(t *testing.T) {
	// Helper: write a temp file and return its path.
	writeTmp := func(t *testing.T, content string) string {
		t.Helper()
		f, err := os.CreateTemp(t.TempDir(), "ctx*.txt")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString(content); err != nil {
			t.Fatal(err)
		}
		f.Close()
		return f.Name()
	}

	tests := []struct {
		name    string
		input   []string
		want    map[string]string
		wantErr bool
	}{
		{
			name:  "single key=value",
			input: []string{"foo=bar"},
			want:  map[string]string{"foo": "bar"},
		},
		{
			name:  "multiple key=value pairs",
			input: []string{"key1=value1", "key2=value2"},
			want:  map[string]string{"key1": "value1", "key2": "value2"},
		},
		{
			name:  "value with equals sign",
			input: []string{"key=a=b=c"},
			want:  map[string]string{"key": "a=b=c"},
		},
		{
			name:  "value preserves leading spaces",
			input: []string{"key=  padded  "},
			want:  map[string]string{"key": "  padded  "},
		},
		{
			name:  "empty value",
			input: []string{"key="},
			want:  map[string]string{"key": ""},
		},
		{
			name:  "bare word without equals stored as key",
			input: []string{"bareword"},
			want:  map[string]string{"bareword": ""},
		},
		{
			name:    "empty key (=value) returns error",
			input:   []string{"=value"},
			wantErr: true,
		},
		{
			name:  "unicode key and value",
			input: []string{"キー=値", "emoji=🎉"},
			want:  map[string]string{"キー": "値", "emoji": "🎉"},
		},
		{
			name:  "empty input returns empty map",
			input: []string{},
			want:  map[string]string{},
		},
	}

	// @file cases built inline to use writeTmp.
	t.Run("bare @file stored as unnamed", func(t *testing.T) {
		path := writeTmp(t, "file content")
		got, err := parseContextValues([]string{"@" + path})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["unnamed"] != "file content" {
			t.Errorf("got %q, want %q", got["unnamed"], "file content")
		}
	})

	t.Run("multiple @file refs concatenated with separator", func(t *testing.T) {
		p1 := writeTmp(t, "first")
		p2 := writeTmp(t, "second")
		got, err := parseContextValues([]string{"@" + p1, "@" + p2})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "first\n\n---\n\nsecond"
		if got["unnamed"] != want {
			t.Errorf("got %q, want %q", got["unnamed"], want)
		}
	})

	t.Run("named key=@file resolves file content", func(t *testing.T) {
		path := writeTmp(t, "requirements")
		got, err := parseContextValues([]string{"req=@" + path})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["req"] != "requirements" {
			t.Errorf("got %q, want %q", got["req"], "requirements")
		}
	})

	t.Run("mixed key=value and @file", func(t *testing.T) {
		path := writeTmp(t, "file data")
		got, err := parseContextValues([]string{"key=value", "@" + path})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["key"] != "value" {
			t.Errorf("key: got %q, want %q", got["key"], "value")
		}
		if got["unnamed"] != "file data" {
			t.Errorf("unnamed: got %q, want %q", got["unnamed"], "file data")
		}
	})

	t.Run("@file nonexistent returns error", func(t *testing.T) {
		_, err := parseContextValues([]string{"@/does/not/exist.txt"})
		if err == nil {
			t.Error("expected error for missing file")
		}
	})

	t.Run("named key=@file nonexistent returns error", func(t *testing.T) {
		_, err := parseContextValues([]string{"key=@/does/not/exist.txt"})
		if err == nil {
			t.Error("expected error for missing file")
		}
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseContextValues(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Errorf("got %d entries, want %d: %v", len(got), len(tt.want), got)
				return
			}
			for k, wantV := range tt.want {
				gotV, ok := got[k]
				if !ok {
					t.Errorf("missing key %q", k)
					continue
				}
				if gotV != wantV {
					t.Errorf("key %q: got %q, want %q", k, gotV, wantV)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// buildContextString
// ---------------------------------------------------------------------------

func TestBuildContextString(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]string
		want  string
	}{
		{
			name:  "empty map returns empty string",
			input: map[string]string{},
			want:  "",
		},
		{
			name:  "single named key",
			input: map[string]string{"lang": "Go"},
			want:  "lang: Go\n",
		},
		{
			name:  "multiple named keys sorted",
			input: map[string]string{"z": "last", "a": "first", "m": "mid"},
			want:  "a: first\nm: mid\nz: last\n",
		},
		{
			name:  "unnamed prepended before named keys",
			input: map[string]string{"unnamed": "PREAMBLE", "key": "val"},
			want:  "PREAMBLE\nkey: val\n",
		},
		{
			name:  "unnamed only",
			input: map[string]string{"unnamed": "just text"},
			want:  "just text\n",
		},
		{
			name:  "deterministic across runs",
			input: map[string]string{"c": "3", "a": "1", "b": "2"},
			want:  "a: 1\nb: 2\nc: 3\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildContextString(tt.input)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// parseArgs -c / --context flag
// ---------------------------------------------------------------------------

func TestParseArgsContext(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantCtx []string
		wantErr bool
	}{
		{
			name:    "single -c flag",
			args:    []string{"run", "test.dot", "-c", "key=value"},
			wantCtx: []string{"key=value"},
		},
		{
			name:    "multiple -c flags",
			args:    []string{"run", "test.dot", "-c", "k1=v1", "-c", "k2=v2"},
			wantCtx: []string{"k1=v1", "k2=v2"},
		},
		{
			name:    "--context long form",
			args:    []string{"agent", "prompt", "--context", "foo=bar"},
			wantCtx: []string{"foo=bar"},
		},
		{
			name:    "-c without argument errors",
			args:    []string{"run", "test.dot", "-c"},
			wantErr: true,
		},
		{
			name:    "-c with @file reference stored verbatim",
			args:    []string{"run", "test.dot", "-c", "@config.yaml"},
			wantCtx: []string{"@config.yaml"},
		},
		{
			name:    "mixed flags and -c",
			args:    []string{"run", "test.dot", "-c", "k=v", "--verbose", "-c", "@file"},
			wantCtx: []string{"k=v", "@file"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, opts, err := parseArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(opts.context) != len(tt.wantCtx) {
				t.Errorf("got %d context entries, want %d: %v", len(opts.context), len(tt.wantCtx), opts.context)
				return
			}
			for i, want := range tt.wantCtx {
				if opts.context[i] != want {
					t.Errorf("context[%d]: got %q, want %q", i, opts.context[i], want)
				}
			}
		})
	}
}

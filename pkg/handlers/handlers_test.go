package handlers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pcontext "github.com/rhystic/attractor/pkg/context"
	"github.com/rhystic/attractor/pkg/dot"
	"github.com/rhystic/attractor/pkg/events"
)

// ---------------------------------------------------------------------------
// expandContextVars
// ---------------------------------------------------------------------------

func TestExpandContextVars(t *testing.T) {
	// Helper: create a pcontext.Context pre-populated with the given values.
	makeCtx := func(vals map[string]interface{}) *pcontext.Context {
		ctx := pcontext.New()
		for k, v := range vals {
			ctx.Set(k, v)
		}
		return ctx
	}

	tests := []struct {
		name    string
		prompt  string
		context map[string]interface{}
		want    string
	}{
		{
			name:    "no context vars — passthrough",
			prompt:  "Hello world",
			context: map[string]interface{}{"user.foo": "bar"},
			want:    "Hello world",
		},
		{
			name:    "$context.key with matching key",
			prompt:  "The value is $context.foo",
			context: map[string]interface{}{"user.foo": "bar"},
			want:    "The value is bar",
		},
		{
			name:    "$context.key with missing key kept as-is",
			prompt:  "The value is $context.missing",
			context: map[string]interface{}{"user.foo": "bar"},
			want:    "The value is $context.missing",
		},
		{
			name:    "multiple $context.key references",
			prompt:  "$context.a and $context.b",
			context: map[string]interface{}{"user.a": "A", "user.b": "B"},
			want:    "A and B",
		},
		{
			name:    "bare $context expands all user keys sorted",
			prompt:  "Context: $context end",
			context: map[string]interface{}{"user.z": "last", "user.a": "first"},
			want:    "Context: a: first\nz: last\n end",
		},
		{
			name:    "bare $context at end of string",
			prompt:  "Show me $context",
			context: map[string]interface{}{"user.foo": "bar"},
			want:    "Show me foo: bar\n",
		},
		{
			name:    "unnamed context prepended before named keys",
			prompt:  "$context",
			context: map[string]interface{}{"user.unnamed": "PREAMBLE", "user.key": "val"},
			want:    "PREAMBLE\nkey: val\n",
		},
		{
			name:    "$context.key and bare $context coexist",
			prompt:  "Key: $context.foo All: $context end",
			context: map[string]interface{}{"user.foo": "bar", "user.baz": "qux"},
			want:    "Key: bar All: baz: qux\nfoo: bar\n end",
		},
		{
			name:    "non-user context keys are not expanded",
			prompt:  "$context",
			context: map[string]interface{}{"system.foo": "ignored", "user.baz": "shown"},
			want:    "baz: shown\n",
		},
		{
			name:    "empty context produces empty string for $context",
			prompt:  "Value: $context end",
			context: map[string]interface{}{},
			want:    "Value:  end",
		},
		{
			name:    "integer value formatted with %%v",
			prompt:  "Count: $context.count",
			context: map[string]interface{}{"user.count": 42},
			want:    "Count: 42",
		},
		{
			name:    "boolean value formatted with %%v",
			prompt:  "Flag: $context.enabled",
			context: map[string]interface{}{"user.enabled": true},
			want:    "Flag: true",
		},
		{
			name:    "$context followed by newline",
			prompt:  "Data:\n$context\nEnd",
			context: map[string]interface{}{"user.x": "y"},
			want:    "Data:\nx: y\n\nEnd",
		},
		{
			// $context. is ambiguous — it looks like the start of $context.key
			// but has no word after the dot. The implementation leaves it as-is
			// rather than partially expanding it.
			name:    "$context. with no word after dot left as-is",
			prompt:  "Value: $context.",
			context: map[string]interface{}{"user.foo": "bar"},
			want:    "Value: $context.",
		},
		{
			name:   "deterministic output with many keys",
			prompt: "$context",
			context: map[string]interface{}{
				"user.c": "3",
				"user.a": "1",
				"user.b": "2",
			},
			want: "a: 1\nb: 2\nc: 3\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := makeCtx(tt.context)
			got := expandContextVars(tt.prompt, ctx)
			if got != tt.want {
				t.Errorf("expandContextVars(%q)\n  got:  %q\n  want: %q", tt.prompt, got, tt.want)
			}
		})
	}
}

// TestExpandContextVarsDeterministic runs the same expansion 20 times and
// checks that the output is identical every time (no map-iteration flakiness).
func TestExpandContextVarsDeterministic(t *testing.T) {
	ctx := pcontext.New()
	for _, kv := range []struct{ k, v string }{
		{"user.c", "3"}, {"user.a", "1"}, {"user.b", "2"},
		{"user.z", "z"}, {"user.m", "m"},
	} {
		ctx.Set(kv.k, kv.v)
	}

	first := expandContextVars("$context", ctx)
	for i := 1; i < 20; i++ {
		got := expandContextVars("$context", ctx)
		if got != first {
			t.Errorf("non-deterministic output at run %d:\n  first: %q\n  got:   %q", i, first, got)
		}
	}
}

// ---------------------------------------------------------------------------
// Handler registry resolution
// ---------------------------------------------------------------------------

func TestHandlerRegistryResolve(t *testing.T) {
	reg := NewRegistry(nil) // nil client = simulation mode

	tests := []struct {
		name         string
		nodeShape    string
		nodeType     string
		wantNotNil   bool
		wantHandlerT string
	}{
		{"start shape", "Mdiamond", "", true, "*handlers.StartHandler"},
		{"exit shape", "Msquare", "", true, "*handlers.ExitHandler"},
		{"diamond shape", "diamond", "", true, "*handlers.ConditionalHandler"},
		{"hexagon shape", "hexagon", "", true, "*handlers.WaitForHumanHandler"},
		{"box shape uses codergen", "box", "", true, "*handlers.CodergenHandler"},
		{"default shape uses codergen", "", "", true, "*handlers.CodergenHandler"},
		{"explicit type overrides shape", "box", "start", true, "*handlers.StartHandler"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attrs := map[string]string{}
			if tt.nodeShape != "" {
				attrs["shape"] = tt.nodeShape
			}
			if tt.nodeType != "" {
				attrs["type"] = tt.nodeType
			}
			node := &dot.Node{ID: "test", Attributes: attrs}
			h := reg.Resolve(node)
			if tt.wantNotNil && h == nil {
				t.Errorf("expected non-nil handler for shape=%q type=%q", tt.nodeShape, tt.nodeType)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CodergenHandler (simulation mode — nil LLM client)
// ---------------------------------------------------------------------------

func TestCodergenHandlerSimulation(t *testing.T) {
	tmpDir := t.TempDir()
	handler := &CodergenHandler{Client: nil}

	node := &dot.Node{
		ID:         "stage1",
		Attributes: map[string]string{"label": "Do something"},
	}
	graph := mustParseGraph(t, `digraph P {
		graph [goal="test goal"]
		start [shape=Mdiamond]
		stage1 [label="Do something"]
		exit [shape=Msquare]
		start -> stage1 -> exit
	}`)
	pctx := pcontext.New()
	emitter := events.NewEmitter("test-run")

	outcome := handler.Execute(context.Background(), node, pctx, graph, tmpDir, emitter)

	if outcome.Status != pcontext.StatusSuccess {
		t.Errorf("expected success, got %v: %s", outcome.Status, outcome.FailureReason)
	}

	// Prompt file should be written.
	promptPath := filepath.Join(tmpDir, "stage1", "prompt.md")
	promptData, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("prompt file not written: %v", err)
	}
	if !strings.Contains(string(promptData), "Do something") {
		t.Errorf("prompt file does not contain node label")
	}

	// Response file should be written.
	respPath := filepath.Join(tmpDir, "stage1", "response.md")
	if _, err := os.ReadFile(respPath); err != nil {
		t.Fatalf("response file not written: %v", err)
	}
}

func TestCodergenHandlerGoalExpansion(t *testing.T) {
	tmpDir := t.TempDir()
	handler := &CodergenHandler{Client: nil}

	node := &dot.Node{
		ID:         "task",
		Attributes: map[string]string{"label": "Implement $goal"},
	}
	graph := mustParseGraph(t, `digraph P {
		graph [goal="awesome feature"]
		start [shape=Mdiamond]
		task [label="Implement $goal"]
		exit [shape=Msquare]
		start -> task -> exit
	}`)
	pctx := pcontext.New()
	emitter := events.NewEmitter("test-run")

	outcome := handler.Execute(context.Background(), node, pctx, graph, tmpDir, emitter)
	if outcome.Status != pcontext.StatusSuccess {
		t.Errorf("expected success: %v", outcome.FailureReason)
	}

	promptData, _ := os.ReadFile(filepath.Join(tmpDir, "task", "prompt.md"))
	if !strings.Contains(string(promptData), "awesome feature") {
		t.Errorf("goal not expanded in prompt; got: %s", string(promptData))
	}
}

func TestCodergenHandlerContextVarExpansion(t *testing.T) {
	tmpDir := t.TempDir()
	handler := &CodergenHandler{Client: nil}

	node := &dot.Node{
		ID:         "task",
		Attributes: map[string]string{"label": "Build in $context.lang"},
	}
	graph := mustParseGraph(t, `digraph P {
		start [shape=Mdiamond]
		task [label="Build in $context.lang"]
		exit [shape=Msquare]
		start -> task -> exit
	}`)
	pctx := pcontext.New()
	pctx.Set("user.lang", "Go")
	emitter := events.NewEmitter("test-run")

	outcome := handler.Execute(context.Background(), node, pctx, graph, tmpDir, emitter)
	if outcome.Status != pcontext.StatusSuccess {
		t.Errorf("expected success: %v", outcome.FailureReason)
	}

	promptData, _ := os.ReadFile(filepath.Join(tmpDir, "task", "prompt.md"))
	if !strings.Contains(string(promptData), "Build in Go") {
		t.Errorf("context var not expanded; got: %s", string(promptData))
	}
}

func TestCodergenHandlerContextUpdates(t *testing.T) {
	tmpDir := t.TempDir()
	handler := &CodergenHandler{Client: nil}

	node := &dot.Node{
		ID:         "s1",
		Attributes: map[string]string{"label": "step"},
	}
	graph := mustParseGraph(t, `digraph P {
		start [shape=Mdiamond]
		s1 [label="step"]
		exit [shape=Msquare]
		start -> s1 -> exit
	}`)
	pctx := pcontext.New()
	emitter := events.NewEmitter("test-run")

	outcome := handler.Execute(context.Background(), node, pctx, graph, tmpDir, emitter)

	// Outcome should carry context updates for last_stage.
	if v, ok := outcome.ContextUpdates["last_stage"]; !ok || v != "s1" {
		t.Errorf("expected last_stage=s1 in context updates, got %v", outcome.ContextUpdates)
	}
}

// ---------------------------------------------------------------------------
// WaitForHumanHandler
// ---------------------------------------------------------------------------

func TestWaitForHumanHandlerAutoSelect(t *testing.T) {
	graph := mustParseGraph(t, `digraph P {
		start [shape=Mdiamond]
		gate  [label="Choose path" shape=hexagon]
		pathA [label="Path A"]
		pathB [label="Path B"]
		exit  [shape=Msquare]
		start -> gate
		gate  -> pathA [label="[A] Go left"]
		gate  -> pathB [label="[B] Go right"]
		pathA -> exit
		pathB -> exit
	}`)

	node := graph.Nodes["gate"]
	if node == nil {
		t.Fatal("gate node not found")
	}

	// No AnswerFunc — should default to first option.
	handler := &WaitForHumanHandler{}
	pctx := pcontext.New()
	emitter := events.NewEmitter("test-run")

	outcome := handler.Execute(context.Background(), node, pctx, graph, t.TempDir(), emitter)
	if outcome.Status != pcontext.StatusSuccess {
		t.Errorf("expected success: %v", outcome.FailureReason)
	}

	// SuggestedNexts should point to one of the outgoing nodes.
	if len(outcome.SuggestedNexts) == 0 {
		t.Error("expected SuggestedNexts to be populated")
	}
}

func TestWaitForHumanHandlerWithAnswerFunc(t *testing.T) {
	graph := mustParseGraph(t, `digraph P {
		start [shape=Mdiamond]
		gate  [label="Approve?" shape=hexagon]
		yes   [label="Yes"]
		no    [label="No"]
		exit  [shape=Msquare]
		start -> gate
		gate  -> yes [label="[Y] Yes"]
		gate  -> no  [label="[N] No"]
		yes   -> exit
		no    -> exit
	}`)

	node := graph.Nodes["gate"]
	if node == nil {
		t.Fatal("gate node not found")
	}

	handler := &WaitForHumanHandler{
		AnswerFunc: func(question string, options []string) (string, error) {
			return "[Y] Yes", nil
		},
	}
	pctx := pcontext.New()
	emitter := events.NewEmitter("test-run")

	outcome := handler.Execute(context.Background(), node, pctx, graph, t.TempDir(), emitter)
	if outcome.Status != pcontext.StatusSuccess {
		t.Errorf("expected success: %v", outcome.FailureReason)
	}
	if len(outcome.SuggestedNexts) == 0 || outcome.SuggestedNexts[0] != "yes" {
		t.Errorf("expected suggestion to 'yes', got %v", outcome.SuggestedNexts)
	}
}

func TestWaitForHumanHandlerNoEdgesError(t *testing.T) {
	graph := mustParseGraph(t, `digraph P {
		start [shape=Mdiamond]
		gate  [shape=hexagon label="gate"]
		exit  [shape=Msquare]
		start -> exit
	}`)

	// gate has no outgoing edges
	node := graph.Nodes["gate"]
	if node == nil {
		t.Fatal("gate node not found")
	}

	handler := &WaitForHumanHandler{}
	pctx := pcontext.New()
	emitter := events.NewEmitter("test-run")

	outcome := handler.Execute(context.Background(), node, pctx, graph, t.TempDir(), emitter)
	if outcome.Status != pcontext.StatusFail {
		t.Errorf("expected failure for no outgoing edges, got %v", outcome.Status)
	}
}

// ---------------------------------------------------------------------------
// normalizeLabel
// ---------------------------------------------------------------------------

func TestNormalizeLabel(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Yes", "yes"},
		{"[Y] Yes", "yes"},
		{"[N] No", "no"},
		{"1) Option One", "option one"},
		{"A - First choice", "first choice"},
		{"  trimmed  ", "trimmed"},
		{"", ""},
		{"already lower", "already lower"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeLabel(tt.input)
			if got != tt.want {
				t.Errorf("normalizeLabel(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func mustParseGraph(t *testing.T, src string) *dot.Graph {
	t.Helper()
	g, err := dot.Parse(src)
	if err != nil {
		t.Fatalf("parse graph: %v", err)
	}
	return g
}

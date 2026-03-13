package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rhystic/attractor/pkg/events"
	"github.com/rhystic/attractor/pkg/llm"
	"github.com/rhystic/attractor/pkg/tools"
)

// ---------------------------------------------------------------------------
// LLM mock helpers
// ---------------------------------------------------------------------------

// mockLLMServer returns an httptest.Server that serves a queue of JSON
// response bodies in order. Each call pops the first response.
// If the queue is exhausted it returns a 500 error.
func mockLLMServer(t *testing.T, responses []string) (*httptest.Server, *llm.Client) {
	t.Helper()
	var idx int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := int(atomic.AddInt32(&idx, 1)) - 1
		if i >= len(responses) {
			http.Error(w, "no more responses", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, responses[i])
	}))
	t.Cleanup(srv.Close)
	client := llm.NewClient("test-key", llm.WithBaseURL(srv.URL))
	return srv, client
}

// textResponse builds a minimal OpenRouter-compatible JSON response with
// a simple text message and no tool calls.
func textResponse(text string) string {
	return fmt.Sprintf(`{
		"id": "resp-1",
		"model": "test-model",
		"choices": [{
			"index": 0,
			"message": {"role": "assistant", "content": %s},
			"finish_reason": "stop"
		}],
		"usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
	}`, jsonStr(text))
}

// toolCallResponse builds a response that requests a single tool call.
func toolCallResponse(toolName, argsJSON string) string {
	return fmt.Sprintf(`{
		"id": "resp-2",
		"model": "test-model",
		"choices": [{
			"index": 0,
			"message": {
				"role": "assistant",
				"content": null,
				"tool_calls": [{
					"id": "call-1",
					"type": "function",
					"function": {"name": %s, "arguments": %s}
				}]
			},
			"finish_reason": "tool_calls"
		}],
		"usage": {"prompt_tokens": 20, "completion_tokens": 8, "total_tokens": 28}
	}`, jsonStr(toolName), jsonStr(argsJSON))
}

func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// ---------------------------------------------------------------------------
// Submit — basic flow
// ---------------------------------------------------------------------------

func TestSubmitSimpleResponse(t *testing.T) {
	_, client := mockLLMServer(t, []string{
		textResponse("Task complete!"),
	})

	cfg := DefaultConfig()
	cfg.Model = "test-model"
	session := NewSession(client, cfg)
	defer session.Events.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := session.Submit(ctx, "Do the thing"); err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	resp := session.LastResponse()
	if resp != "Task complete!" {
		t.Errorf("LastResponse() = %q, want %q", resp, "Task complete!")
	}
}

func TestSubmitRecordsHistory(t *testing.T) {
	_, client := mockLLMServer(t, []string{
		textResponse("Done."),
	})

	cfg := DefaultConfig()
	cfg.Model = "test-model"
	session := NewSession(client, cfg)
	defer session.Events.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := session.Submit(ctx, "Hello"); err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	// Expect: user turn + assistant turn.
	if len(session.History) != 2 {
		t.Fatalf("expected 2 history turns, got %d", len(session.History))
	}
	if session.History[0].Role != llm.RoleUser {
		t.Errorf("turn 0: got role %q, want %q", session.History[0].Role, llm.RoleUser)
	}
	if session.History[1].Role != llm.RoleAssistant {
		t.Errorf("turn 1: got role %q, want %q", session.History[1].Role, llm.RoleAssistant)
	}
	if session.History[0].Content != "Hello" {
		t.Errorf("user content = %q, want %q", session.History[0].Content, "Hello")
	}
}

func TestSubmitAccumulatesUsage(t *testing.T) {
	_, client := mockLLMServer(t, []string{
		textResponse("Done."),
	})

	cfg := DefaultConfig()
	cfg.Model = "test-model"
	session := NewSession(client, cfg)
	defer session.Events.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := session.Submit(ctx, "go"); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	usage := session.TotalUsage()
	if usage.InputTokens == 0 && usage.OutputTokens == 0 {
		t.Error("expected non-zero usage after Submit")
	}
}

// ---------------------------------------------------------------------------
// Submit — tool calls
// ---------------------------------------------------------------------------

func TestSubmitWithToolCall(t *testing.T) {
	// Session will:
	//  1. LLM requests "echo" tool
	//  2. Tool returns "hello from echo"
	//  3. LLM responds with final text
	_, client := mockLLMServer(t, []string{
		toolCallResponse("echo", `{"input":"test"}`),
		textResponse("Echo result received."),
	})

	cfg := DefaultConfig()
	cfg.Model = "test-model"

	// Register a simple echo tool.
	reg := tools.NewRegistry()
	reg.Register(echoTool{})
	session := NewSession(client, cfg, WithToolRegistry(reg))
	defer session.Events.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := session.Submit(ctx, "Echo something"); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// History: user, assistant (tool call), tool result, assistant (final)
	if len(session.History) < 4 {
		t.Fatalf("expected ≥4 history turns, got %d", len(session.History))
	}

	// Find tool result turn.
	var toolTurn *Turn
	for i := range session.History {
		if session.History[i].Role == llm.RoleTool {
			toolTurn = &session.History[i]
			break
		}
	}
	if toolTurn == nil {
		t.Fatal("no tool result turn in history")
	}
	if len(toolTurn.Results) == 0 {
		t.Fatal("tool result turn has no results")
	}
	if toolTurn.Results[0].IsError {
		t.Errorf("tool result is error: %s", toolTurn.Results[0].Content)
	}
}

func TestSubmitUnknownToolRecordsError(t *testing.T) {
	_, client := mockLLMServer(t, []string{
		toolCallResponse("nonexistent_tool", `{}`),
		textResponse("Handled the error."),
	})

	cfg := DefaultConfig()
	cfg.Model = "test-model"
	session := NewSession(client, cfg)
	defer session.Events.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := session.Submit(ctx, "Use a missing tool"); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Find tool result turn — it should be an error.
	for _, turn := range session.History {
		if turn.Role == llm.RoleTool {
			for _, r := range turn.Results {
				if r.IsError && strings.Contains(r.Content, "unknown tool") {
					return // found expected error
				}
			}
		}
	}
	t.Error("expected unknown-tool error in history")
}

// ---------------------------------------------------------------------------
// Submit — context cancellation
// ---------------------------------------------------------------------------

func TestSubmitContextCancelled(t *testing.T) {
	// Server that delays long enough for the test context to expire.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second): // longer than test context timeout
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() {
		// CloseClientConnections forces active connections to close so
		// srv.Close() doesn't block waiting for the handler.
		srv.CloseClientConnections()
		srv.Close()
	})

	client := llm.NewClient("key", llm.WithBaseURL(srv.URL),
		llm.WithHTTPClient(&http.Client{Timeout: 5 * time.Second}))
	cfg := DefaultConfig()
	cfg.Model = "test-model"
	session := NewSession(client, cfg)
	defer session.Events.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	err := session.Submit(ctx, "take a long time")
	if err == nil {
		t.Error("expected error from context cancellation, got nil")
	}
}

// ---------------------------------------------------------------------------
// Submit — session state guard
// ---------------------------------------------------------------------------

func TestSubmitRejectsWhileProcessing(t *testing.T) {
	// Slow server — first response blocks.
	slow := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-slow
		fmt.Fprint(w, textResponse("done"))
	}))
	defer srv.Close()
	defer close(slow)

	client := llm.NewClient("key", llm.WithBaseURL(srv.URL))
	cfg := DefaultConfig()
	cfg.Model = "test-model"
	session := NewSession(client, cfg)
	defer session.Events.Close()

	ctx := context.Background()

	// Start first submit in background.
	done := make(chan error, 1)
	go func() { done <- session.Submit(ctx, "first") }()

	// Give it a moment to enter StateProcessing.
	time.Sleep(50 * time.Millisecond)

	// Second submit should fail immediately.
	err := session.Submit(ctx, "concurrent")
	if err == nil {
		t.Error("expected error submitting while processing")
	}
	if !strings.Contains(err.Error(), "session not ready") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Abort
// ---------------------------------------------------------------------------

func TestAbortStopsSession(t *testing.T) {
	// Infinite tool-call loop until abort.
	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		if n == 1 {
			// Trigger abort from a separate goroutine after short delay.
			fmt.Fprint(w, toolCallResponse("echo", `{"input":"x"}`))
		} else {
			fmt.Fprint(w, textResponse("stopped"))
		}
	}))
	defer srv.Close()

	client := llm.NewClient("key", llm.WithBaseURL(srv.URL))
	cfg := DefaultConfig()
	cfg.Model = "test-model"
	reg := tools.NewRegistry()
	reg.Register(echoTool{})
	session := NewSession(client, cfg, WithToolRegistry(reg))
	defer session.Events.Close()

	ctx := context.Background()

	done := make(chan error, 1)
	go func() { done <- session.Submit(ctx, "do stuff") }()

	// Let first tool call complete, then abort.
	time.Sleep(100 * time.Millisecond)
	session.Abort()

	select {
	case err := <-done:
		if err == nil {
			// Either aborted cleanly before abort was noticed, or finished normally.
			// Both are acceptable — just verify session is closed.
		}
		if session.State != StateClosed {
			t.Errorf("expected StateClosed after Abort, got %v", session.State)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Submit did not return after Abort")
	}
}

// ---------------------------------------------------------------------------
// Loop detection
// ---------------------------------------------------------------------------

func TestDetectLoopRepeatingPattern(t *testing.T) {
	session := NewSession(nil, DefaultConfig())

	// Inject 10 identical tool-call turns to trigger loop detection (window=10).
	for i := 0; i < 10; i++ {
		session.History = append(session.History, Turn{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{
				{ID: fmt.Sprintf("c%d", i), Name: "shell", Arguments: json.RawMessage(`{"command":"ls"}`)},
			},
			Timestamp: time.Now(),
		})
	}

	if !session.detectLoop() {
		t.Error("detectLoop() = false, want true for 10 identical calls")
	}
}

func TestDetectLoopNoPattern(t *testing.T) {
	session := NewSession(nil, DefaultConfig())

	// 10 different tool calls — no repeating pattern.
	for i := 0; i < 10; i++ {
		session.History = append(session.History, Turn{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{
				{ID: fmt.Sprintf("c%d", i), Name: "shell",
					Arguments: json.RawMessage(fmt.Sprintf(`{"command":"cmd%d"}`, i))},
			},
			Timestamp: time.Now(),
		})
	}

	if session.detectLoop() {
		t.Error("detectLoop() = true, want false for all-different calls")
	}
}

func TestDetectLoopInsufficientHistory(t *testing.T) {
	session := NewSession(nil, DefaultConfig())

	// Only 5 turns — below the window of 10.
	for i := 0; i < 5; i++ {
		session.History = append(session.History, Turn{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{
				{ID: fmt.Sprintf("c%d", i), Name: "shell", Arguments: json.RawMessage(`{"command":"ls"}`)},
			},
			Timestamp: time.Now(),
		})
	}

	if session.detectLoop() {
		t.Error("detectLoop() = true, want false when history < window")
	}
}

func TestDetectLoopPeriod2Pattern(t *testing.T) {
	session := NewSession(nil, DefaultConfig())

	// Alternating two calls — period-2 pattern over window of 10.
	cmds := []string{"ls", "pwd"}
	for i := 0; i < 10; i++ {
		session.History = append(session.History, Turn{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{
				{ID: fmt.Sprintf("c%d", i), Name: "shell",
					Arguments: json.RawMessage(fmt.Sprintf(`{"command":%q}`, cmds[i%2]))},
			},
			Timestamp: time.Now(),
		})
	}

	if !session.detectLoop() {
		t.Error("detectLoop() = false, want true for period-2 pattern")
	}
}

// ---------------------------------------------------------------------------
// Events emitted during Submit
// ---------------------------------------------------------------------------

func TestSubmitEmitsExpectedEvents(t *testing.T) {
	_, client := mockLLMServer(t, []string{
		textResponse("All done."),
	})

	cfg := DefaultConfig()
	cfg.Model = "test-model"
	session := NewSession(client, cfg)

	// Collect events before submitting.
	eventCh := session.Events.Subscribe()
	var received []events.EventType
	collectDone := make(chan struct{})
	go func() {
		defer close(collectDone)
		for ev := range eventCh {
			received = append(received, ev.Type)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := session.Submit(ctx, "hello"); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	session.Events.Close()
	<-collectDone

	// We expect at least an LLMStart and LLMEnd event.
	has := func(et events.EventType) bool {
		for _, e := range received {
			if e == et {
				return true
			}
		}
		return false
	}

	if !has(events.EventLLMStart) {
		t.Error("expected EventLLMStart")
	}
	if !has(events.EventLLMEnd) {
		t.Error("expected EventLLMEnd")
	}
	if !has(events.EventLog) {
		t.Error("expected EventLog")
	}
}

// ---------------------------------------------------------------------------
// LastResponse
// ---------------------------------------------------------------------------

func TestLastResponseEmptyHistory(t *testing.T) {
	session := NewSession(nil, DefaultConfig())
	if got := session.LastResponse(); got != "" {
		t.Errorf("LastResponse() = %q, want empty", got)
	}
}

func TestLastResponseSkipsToolTurns(t *testing.T) {
	session := NewSession(nil, DefaultConfig())
	session.History = []Turn{
		{Role: llm.RoleUser, Content: "hi", Timestamp: time.Now()},
		{Role: llm.RoleAssistant, Content: "first reply", Timestamp: time.Now()},
		{Role: llm.RoleTool, Results: []ToolResult{{Content: "tool out"}}, Timestamp: time.Now()},
		{Role: llm.RoleAssistant, Content: "final reply", Timestamp: time.Now()},
	}
	if got := session.LastResponse(); got != "final reply" {
		t.Errorf("LastResponse() = %q, want %q", got, "final reply")
	}
}

// ---------------------------------------------------------------------------
// DefaultConfig
// ---------------------------------------------------------------------------

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Model == "" {
		t.Error("Model should not be empty")
	}
	if !cfg.EnableLoopDetection {
		t.Error("EnableLoopDetection should be true")
	}
	if cfg.LoopDetectionWindow <= 0 {
		t.Errorf("LoopDetectionWindow = %d, want > 0", cfg.LoopDetectionWindow)
	}
	if cfg.DefaultCommandTimeoutMs <= 0 {
		t.Errorf("DefaultCommandTimeoutMs = %d, want > 0", cfg.DefaultCommandTimeoutMs)
	}
}

// ---------------------------------------------------------------------------
// Session options
// ---------------------------------------------------------------------------

func TestWithToolRegistry(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(echoTool{})

	session := NewSession(nil, DefaultConfig(), WithToolRegistry(reg))

	if _, ok := session.ToolRegistry.Get("echo"); !ok {
		t.Error("custom tool registry not applied")
	}
}

// ---------------------------------------------------------------------------
// echoTool — minimal tool for testing
// ---------------------------------------------------------------------------

type echoTool struct{}

func (echoTool) Name() string        { return "echo" }
func (echoTool) Description() string { return "Echoes input" }
func (echoTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"input":{"type":"string"}}}`)
}
func (echoTool) Execute(_ context.Context, _ tools.ExecutionEnvironment, args json.RawMessage) (string, error) {
	var p struct{ Input string }
	if err := json.Unmarshal(args, &p); err != nil {
		return "", err
	}
	return "echo: " + p.Input, nil
}

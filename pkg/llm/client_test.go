package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewTextMessage(t *testing.T) {
	msg := NewTextMessage(RoleUser, "Hello")

	if msg.Role != RoleUser {
		t.Errorf("Expected role %s, got %s", RoleUser, msg.Role)
	}

	if len(msg.Content) != 1 {
		t.Fatalf("Expected 1 content part, got %d", len(msg.Content))
	}

	if msg.Content[0].Kind != ContentText {
		t.Errorf("Expected content kind %s, got %s", ContentText, msg.Content[0].Kind)
	}

	if msg.Text() != "Hello" {
		t.Errorf("Expected text 'Hello', got '%s'", msg.Text())
	}
}

func TestMessageHelpers(t *testing.T) {
	sys := NewSystemMessage("You are helpful")
	if sys.Role != RoleSystem {
		t.Errorf("Expected system role")
	}

	user := NewUserMessage("Hi")
	if user.Role != RoleUser {
		t.Errorf("Expected user role")
	}

	assist := NewAssistantMessage("Hello!")
	if assist.Role != RoleAssistant {
		t.Errorf("Expected assistant role")
	}

	tool := NewToolResultMessage("call_123", "result", false)
	if tool.Role != RoleTool {
		t.Errorf("Expected tool role")
	}
	if tool.ToolCallID != "call_123" {
		t.Errorf("Expected tool call ID")
	}
}

func TestMessageToolCalls(t *testing.T) {
	msg := Message{
		Role: RoleAssistant,
		Content: []ContentPart{
			{Kind: ContentText, Text: "I'll help you"},
			{
				Kind: ContentToolCall,
				ToolCall: &ToolCall{
					ID:        "call_1",
					Name:      "read_file",
					Arguments: json.RawMessage(`{"path": "test.go"}`),
				},
			},
			{
				Kind: ContentToolCall,
				ToolCall: &ToolCall{
					ID:        "call_2",
					Name:      "write_file",
					Arguments: json.RawMessage(`{"path": "out.txt", "content": "data"}`),
				},
			},
		},
	}

	calls := msg.ToolCalls()
	if len(calls) != 2 {
		t.Fatalf("Expected 2 tool calls, got %d", len(calls))
	}

	if calls[0].Name != "read_file" {
		t.Errorf("Expected first tool 'read_file', got '%s'", calls[0].Name)
	}
	if calls[1].Name != "write_file" {
		t.Errorf("Expected second tool 'write_file', got '%s'", calls[1].Name)
	}
}

func TestUsageAdd(t *testing.T) {
	u1 := Usage{
		InputTokens:  100,
		OutputTokens: 50,
		TotalTokens:  150,
	}

	u2 := Usage{
		InputTokens:     200,
		OutputTokens:    100,
		TotalTokens:     300,
		ReasoningTokens: 50,
	}

	combined := u1.Add(u2)

	if combined.InputTokens != 300 {
		t.Errorf("Expected 300 input tokens, got %d", combined.InputTokens)
	}
	if combined.OutputTokens != 150 {
		t.Errorf("Expected 150 output tokens, got %d", combined.OutputTokens)
	}
	if combined.TotalTokens != 450 {
		t.Errorf("Expected 450 total tokens, got %d", combined.TotalTokens)
	}
	if combined.ReasoningTokens != 50 {
		t.Errorf("Expected 50 reasoning tokens, got %d", combined.ReasoningTokens)
	}
}

func TestClientCreation(t *testing.T) {
	client := NewClient("test-api-key")

	if client.apiKey != "test-api-key" {
		t.Errorf("Expected API key 'test-api-key'")
	}

	if client.baseURL != "https://openrouter.ai/api/v1" {
		t.Errorf("Expected default base URL")
	}
}

func TestClientWithOptions(t *testing.T) {
	customHTTP := &http.Client{}
	client := NewClient("key",
		WithBaseURL("https://custom.api.com"),
		WithHTTPClient(customHTTP),
		WithSiteInfo("https://mysite.com", "MySite"),
	)

	if client.baseURL != "https://custom.api.com" {
		t.Errorf("Expected custom base URL")
	}

	if client.httpClient != customHTTP {
		t.Error("Expected custom HTTP client")
	}

	if client.siteURL != "https://mysite.com" {
		t.Errorf("Expected custom site URL")
	}
}

func TestClientComplete(t *testing.T) {
	// Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("Expected Authorization header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type header")
		}

		// Return mock response
		resp := openRouterResponse{
			ID:    "resp-123",
			Model: "test-model",
			Choices: []struct {
				Index        int               `json:"index"`
				Message      openRouterMessage `json:"message"`
				FinishReason string            `json:"finish_reason"`
			}{
				{
					Index:        0,
					Message:      openRouterMessage{Role: "assistant", Content: "Hello!"},
					FinishReason: "stop",
				},
			},
			Usage: struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			}{
				PromptTokens:     10,
				CompletionTokens: 5,
				TotalTokens:      15,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("test-key", WithBaseURL(server.URL))

	req := Request{
		Model: "test-model",
		Messages: []Message{
			NewUserMessage("Hi"),
		},
	}

	resp, err := client.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	if resp.ID != "resp-123" {
		t.Errorf("Expected response ID 'resp-123', got '%s'", resp.ID)
	}

	if resp.Text() != "Hello!" {
		t.Errorf("Expected text 'Hello!', got '%s'", resp.Text())
	}

	if resp.FinishReason.Reason != "stop" {
		t.Errorf("Expected finish reason 'stop', got '%s'", resp.FinishReason.Reason)
	}

	if resp.Usage.InputTokens != 10 {
		t.Errorf("Expected 10 input tokens, got %d", resp.Usage.InputTokens)
	}
}

func TestClientCompleteWithTools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return response with tool call
		resp := openRouterResponse{
			ID:    "resp-456",
			Model: "test-model",
			Choices: []struct {
				Index        int               `json:"index"`
				Message      openRouterMessage `json:"message"`
				FinishReason string            `json:"finish_reason"`
			}{
				{
					Index: 0,
					Message: openRouterMessage{
						Role:    "assistant",
						Content: nil,
						ToolCalls: []openRouterToolCall{
							{
								ID:   "call_abc",
								Type: "function",
								Function: openRouterToolCallFunction{
									Name:      "read_file",
									Arguments: `{"file_path": "test.go"}`,
								},
							},
						},
					},
					FinishReason: "tool_calls",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("test-key", WithBaseURL(server.URL))

	req := Request{
		Model: "test-model",
		Messages: []Message{
			NewUserMessage("Read test.go"),
		},
		Tools: []ToolDefinition{
			{
				Name:        "read_file",
				Description: "Read a file",
				Parameters:  json.RawMessage(`{"type": "object"}`),
			},
		},
	}

	resp, err := client.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	if resp.FinishReason.Reason != "tool_calls" {
		t.Errorf("Expected finish reason 'tool_calls', got '%s'", resp.FinishReason.Reason)
	}

	calls := resp.ToolCalls()
	if len(calls) != 1 {
		t.Fatalf("Expected 1 tool call, got %d", len(calls))
	}

	if calls[0].Name != "read_file" {
		t.Errorf("Expected tool name 'read_file', got '%s'", calls[0].Name)
	}

	if calls[0].ID != "call_abc" {
		t.Errorf("Expected tool call ID 'call_abc', got '%s'", calls[0].ID)
	}
}

func TestClientError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		resp := openRouterResponse{
			Error: &struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Code    string `json:"code"`
			}{
				Message: "Invalid request",
				Type:    "invalid_request",
				Code:    "400",
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("test-key", WithBaseURL(server.URL))

	req := Request{
		Model:    "test-model",
		Messages: []Message{NewUserMessage("Hi")},
	}

	_, err := client.Complete(context.Background(), req)
	if err == nil {
		t.Error("Expected error for bad request")
	}
}

func TestNormalizeFinishReason(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"stop", "stop"},
		{"length", "length"},
		{"tool_calls", "tool_calls"},
		{"function_call", "tool_calls"},
		{"content_filter", "content_filter"},
		{"", "stop"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeFinishReason(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeFinishReason(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestToolDefinitionJSON(t *testing.T) {
	tool := ToolDefinition{
		Name:        "test_tool",
		Description: "A test tool",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"arg1": {"type": "string"}
			},
			"required": ["arg1"]
		}`),
	}

	data, err := json.Marshal(tool)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var parsed ToolDefinition
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if parsed.Name != "test_tool" {
		t.Errorf("Expected name 'test_tool', got '%s'", parsed.Name)
	}
}

func TestCollectStream(t *testing.T) {
	events := make(chan StreamEvent, 10)

	// Simulate stream events
	events <- StreamEvent{Type: StreamStart}
	events <- StreamEvent{Type: StreamTextDelta, Delta: "Hello"}
	events <- StreamEvent{Type: StreamTextDelta, Delta: " World"}
	events <- StreamEvent{
		Type:     StreamToolCall,
		ToolCall: &ToolCall{ID: "call_1", Name: "test"},
	}
	events <- StreamEvent{
		Type:         StreamFinish,
		FinishReason: &FinishReason{Reason: "stop"},
		Usage:        &Usage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
	}
	close(events)

	resp, err := CollectStream(events)
	if err != nil {
		t.Fatalf("CollectStream failed: %v", err)
	}

	if resp.Text() != "Hello World" {
		t.Errorf("Expected text 'Hello World', got '%s'", resp.Text())
	}

	calls := resp.ToolCalls()
	if len(calls) != 1 {
		t.Errorf("Expected 1 tool call, got %d", len(calls))
	}

	if resp.FinishReason.Reason != "stop" {
		t.Errorf("Expected finish reason 'stop'")
	}

	if resp.Usage.TotalTokens != 15 {
		t.Errorf("Expected 15 total tokens")
	}
}

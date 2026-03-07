// Package llm provides a unified LLM client interface for OpenRouter.
package llm

import (
	"encoding/json"
	"time"
)

// Role represents the role of a message in a conversation.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ContentKind identifies the type of content in a ContentPart.
type ContentKind string

const (
	ContentText       ContentKind = "text"
	ContentImage      ContentKind = "image"
	ContentToolCall   ContentKind = "tool_call"
	ContentToolResult ContentKind = "tool_result"
)

// ContentPart represents a single piece of content within a message.
type ContentPart struct {
	Kind       ContentKind     `json:"kind"`
	Text       string          `json:"text,omitempty"`
	Image      *ImageData      `json:"image,omitempty"`
	ToolCall   *ToolCall       `json:"tool_call,omitempty"`
	ToolResult *ToolResultData `json:"tool_result,omitempty"`
}

// ImageData contains image information for multimodal messages.
type ImageData struct {
	URL       string `json:"url,omitempty"`
	Data      []byte `json:"data,omitempty"`
	MediaType string `json:"media_type,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

// ToolCall represents a model-initiated tool invocation.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolResultData holds the result of a tool execution.
type ToolResultData struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error"`
}

// Message represents a single message in a conversation.
type Message struct {
	Role       Role          `json:"role"`
	Content    []ContentPart `json:"content"`
	Name       string        `json:"name,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}

// NewTextMessage creates a message with a single text content part.
func NewTextMessage(role Role, text string) Message {
	return Message{
		Role: role,
		Content: []ContentPart{
			{Kind: ContentText, Text: text},
		},
	}
}

// NewSystemMessage creates a system message.
func NewSystemMessage(text string) Message {
	return NewTextMessage(RoleSystem, text)
}

// NewUserMessage creates a user message.
func NewUserMessage(text string) Message {
	return NewTextMessage(RoleUser, text)
}

// NewAssistantMessage creates an assistant message.
func NewAssistantMessage(text string) Message {
	return NewTextMessage(RoleAssistant, text)
}

// NewToolResultMessage creates a tool result message.
func NewToolResultMessage(toolCallID, content string, isError bool) Message {
	return Message{
		Role:       RoleTool,
		ToolCallID: toolCallID,
		Content: []ContentPart{
			{
				Kind: ContentToolResult,
				ToolResult: &ToolResultData{
					ToolCallID: toolCallID,
					Content:    content,
					IsError:    isError,
				},
			},
		},
	}
}

// Text returns the concatenated text from all text content parts.
func (m Message) Text() string {
	var result string
	for _, part := range m.Content {
		if part.Kind == ContentText {
			result += part.Text
		}
	}
	return result
}

// ToolCalls extracts all tool calls from the message.
func (m Message) ToolCalls() []ToolCall {
	var calls []ToolCall
	for _, part := range m.Content {
		if part.Kind == ContentToolCall && part.ToolCall != nil {
			calls = append(calls, *part.ToolCall)
		}
	}
	return calls
}

// ToolDefinition describes a tool available to the model.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // JSON Schema
}

// ToolChoice controls how the model selects tools.
type ToolChoice struct {
	Type string `json:"type"` // "auto", "none", "required", or "tool"
	Name string `json:"name,omitempty"`
}

// Request represents a completion request to the LLM.
type Request struct {
	Model           string           `json:"model"`
	Messages        []Message        `json:"messages"`
	Tools           []ToolDefinition `json:"tools,omitempty"`
	ToolChoice      *ToolChoice      `json:"tool_choice,omitempty"`
	Temperature     *float64         `json:"temperature,omitempty"`
	TopP            *float64         `json:"top_p,omitempty"`
	MaxTokens       *int             `json:"max_tokens,omitempty"`
	StopSequences   []string         `json:"stop,omitempty"`
	ReasoningEffort string           `json:"reasoning_effort,omitempty"`
	Stream          bool             `json:"stream,omitempty"`
}

// FinishReason indicates why the model stopped generating.
type FinishReason struct {
	Reason string `json:"reason"` // "stop", "length", "tool_calls", "content_filter", "error"
	Raw    string `json:"raw,omitempty"`
}

// Usage contains token usage information.
type Usage struct {
	InputTokens     int `json:"input_tokens"`
	OutputTokens    int `json:"output_tokens"`
	TotalTokens     int `json:"total_tokens"`
	ReasoningTokens int `json:"reasoning_tokens,omitempty"`
	CacheReadTokens int `json:"cache_read_tokens,omitempty"`
}

// Add combines two Usage objects.
func (u Usage) Add(other Usage) Usage {
	return Usage{
		InputTokens:     u.InputTokens + other.InputTokens,
		OutputTokens:    u.OutputTokens + other.OutputTokens,
		TotalTokens:     u.TotalTokens + other.TotalTokens,
		ReasoningTokens: u.ReasoningTokens + other.ReasoningTokens,
		CacheReadTokens: u.CacheReadTokens + other.CacheReadTokens,
	}
}

// ModelPricing holds cost per million tokens in USD.
type ModelPricing struct {
	InputPerMillion  float64
	OutputPerMillion float64
}

// ModelPrices contains known model pricing (USD per million tokens).
var ModelPrices = map[string]ModelPricing{
	"minimax/minimax-m2.5": {InputPerMillion: 0.295, OutputPerMillion: 1.20},
}

// Cost calculates the cost in USD for this usage with the given model.
// Returns total, inputCost, outputCost. If model is unknown, returns zeros.
func (u Usage) Cost(model string) (total, inputCost, outputCost float64) {
	pricing, ok := ModelPrices[model]
	if !ok {
		return 0, 0, 0
	}
	inputCost = float64(u.InputTokens) / 1_000_000 * pricing.InputPerMillion
	outputCost = float64(u.OutputTokens) / 1_000_000 * pricing.OutputPerMillion
	total = inputCost + outputCost
	return
}

// HasPricing returns true if the model has known pricing.
func HasPricing(model string) bool {
	_, ok := ModelPrices[model]
	return ok
}

// Response represents a completion response from the LLM.
type Response struct {
	ID           string          `json:"id"`
	Model        string          `json:"model"`
	Message      Message         `json:"message"`
	FinishReason FinishReason    `json:"finish_reason"`
	Usage        Usage           `json:"usage"`
	Raw          json.RawMessage `json:"raw,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}

// Text returns the text content from the response message.
func (r Response) Text() string {
	return r.Message.Text()
}

// ToolCalls returns the tool calls from the response message.
func (r Response) ToolCalls() []ToolCall {
	return r.Message.ToolCalls()
}

// StreamEventType identifies the type of streaming event.
type StreamEventType string

const (
	StreamStart     StreamEventType = "stream_start"
	StreamTextDelta StreamEventType = "text_delta"
	StreamToolCall  StreamEventType = "tool_call"
	StreamFinish    StreamEventType = "finish"
	StreamError     StreamEventType = "error"
)

// StreamEvent represents a single event in a streaming response.
type StreamEvent struct {
	Type         StreamEventType `json:"type"`
	Delta        string          `json:"delta,omitempty"`
	ToolCall     *ToolCall       `json:"tool_call,omitempty"`
	FinishReason *FinishReason   `json:"finish_reason,omitempty"`
	Usage        *Usage          `json:"usage,omitempty"`
	Error        error           `json:"error,omitempty"`
	Raw          json.RawMessage `json:"raw,omitempty"`
}

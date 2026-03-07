package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Client provides a unified interface for LLM completions via OpenRouter.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	siteURL    string
	siteName   string
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithBaseURL sets a custom base URL for the OpenRouter API.
func WithBaseURL(url string) ClientOption {
	return func(c *Client) {
		c.baseURL = url
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = client
	}
}

// WithSiteInfo sets the site URL and name for OpenRouter rankings.
func WithSiteInfo(url, name string) ClientOption {
	return func(c *Client) {
		c.siteURL = url
		c.siteName = name
	}
}

// NewClient creates a new OpenRouter client.
func NewClient(apiKey string, opts ...ClientOption) *Client {
	c := &Client{
		apiKey:  apiKey,
		baseURL: "https://openrouter.ai/api/v1",
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
		siteURL:  "https://github.com/rhystic/attractor",
		siteName: "Attractor",
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// NewClientFromEnv creates a client using the OPENROUTER_API_KEY environment variable.
func NewClientFromEnv(opts ...ClientOption) (*Client, error) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENROUTER_API_KEY environment variable not set")
	}
	return NewClient(apiKey, opts...), nil
}

// openRouterRequest is the request format for OpenRouter's API.
type openRouterRequest struct {
	Model       string              `json:"model"`
	Messages    []openRouterMessage `json:"messages"`
	Tools       []openRouterTool    `json:"tools,omitempty"`
	ToolChoice  any                 `json:"tool_choice,omitempty"`
	Temperature *float64            `json:"temperature,omitempty"`
	TopP        *float64            `json:"top_p,omitempty"`
	MaxTokens   *int                `json:"max_tokens,omitempty"`
	Stop        []string            `json:"stop,omitempty"`
	Stream      bool                `json:"stream,omitempty"`
}

type openRouterMessage struct {
	Role       string               `json:"role"`
	Content    any                  `json:"content"` // string or []openRouterContentPart
	Name       string               `json:"name,omitempty"`
	ToolCallID string               `json:"tool_call_id,omitempty"`
	ToolCalls  []openRouterToolCall `json:"tool_calls,omitempty"`
}

type openRouterContentPart struct {
	Type     string              `json:"type"`
	Text     string              `json:"text,omitempty"`
	ImageURL *openRouterImageURL `json:"image_url,omitempty"`
}

type openRouterImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

type openRouterTool struct {
	Type     string                 `json:"type"`
	Function openRouterToolFunction `json:"function"`
}

type openRouterToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type openRouterToolCall struct {
	ID       string                     `json:"id"`
	Type     string                     `json:"type"`
	Function openRouterToolCallFunction `json:"function"`
}

type openRouterToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openRouterResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Created int64  `json:"created"`
	Choices []struct {
		Index        int               `json:"index"`
		Message      openRouterMessage `json:"message"`
		FinishReason string            `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

type openRouterStreamChunk struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role      string               `json:"role,omitempty"`
			Content   string               `json:"content,omitempty"`
			ToolCalls []openRouterToolCall `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason,omitempty"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
}

// convertRequest converts a unified Request to OpenRouter format.
func (c *Client) convertRequest(req Request) openRouterRequest {
	orReq := openRouterRequest{
		Model:       req.Model,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		MaxTokens:   req.MaxTokens,
		Stop:        req.StopSequences,
		Stream:      req.Stream,
	}

	// Convert messages
	for _, msg := range req.Messages {
		orMsg := c.convertMessage(msg)
		orReq.Messages = append(orReq.Messages, orMsg)
	}

	// Convert tools
	for _, tool := range req.Tools {
		orReq.Tools = append(orReq.Tools, openRouterTool{
			Type: "function",
			Function: openRouterToolFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Parameters,
			},
		})
	}

	// Convert tool choice
	if req.ToolChoice != nil {
		switch req.ToolChoice.Type {
		case "auto":
			orReq.ToolChoice = "auto"
		case "none":
			orReq.ToolChoice = "none"
		case "required":
			orReq.ToolChoice = "required"
		case "tool":
			orReq.ToolChoice = map[string]any{
				"type": "function",
				"function": map[string]string{
					"name": req.ToolChoice.Name,
				},
			}
		}
	}

	return orReq
}

func (c *Client) convertMessage(msg Message) openRouterMessage {
	orMsg := openRouterMessage{
		Role:       string(msg.Role),
		Name:       msg.Name,
		ToolCallID: msg.ToolCallID,
	}

	// Check if we have only text content
	hasOnlyText := true
	var toolCalls []openRouterToolCall
	for _, part := range msg.Content {
		if part.Kind != ContentText {
			hasOnlyText = false
		}
		if part.Kind == ContentToolCall && part.ToolCall != nil {
			toolCalls = append(toolCalls, openRouterToolCall{
				ID:   part.ToolCall.ID,
				Type: "function",
				Function: openRouterToolCallFunction{
					Name:      part.ToolCall.Name,
					Arguments: string(part.ToolCall.Arguments),
				},
			})
		}
	}

	if len(toolCalls) > 0 {
		orMsg.ToolCalls = toolCalls
	}

	if hasOnlyText {
		// Simple string content
		orMsg.Content = msg.Text()
	} else {
		// Multimodal content
		var parts []openRouterContentPart
		for _, part := range msg.Content {
			switch part.Kind {
			case ContentText:
				parts = append(parts, openRouterContentPart{
					Type: "text",
					Text: part.Text,
				})
			case ContentImage:
				if part.Image != nil {
					var url string
					if part.Image.URL != "" {
						url = part.Image.URL
					} else if len(part.Image.Data) > 0 {
						mediaType := part.Image.MediaType
						if mediaType == "" {
							mediaType = "image/png"
						}
						url = fmt.Sprintf("data:%s;base64,%s",
							mediaType, base64.StdEncoding.EncodeToString(part.Image.Data))
					}
					parts = append(parts, openRouterContentPart{
						Type: "image_url",
						ImageURL: &openRouterImageURL{
							URL:    url,
							Detail: part.Image.Detail,
						},
					})
				}
			case ContentToolResult:
				if part.ToolResult != nil {
					parts = append(parts, openRouterContentPart{
						Type: "text",
						Text: part.ToolResult.Content,
					})
				}
			}
		}
		orMsg.Content = parts
	}

	return orMsg
}

func (c *Client) convertResponse(orResp openRouterResponse, raw json.RawMessage) Response {
	resp := Response{
		ID:        orResp.ID,
		Model:     orResp.Model,
		CreatedAt: time.Unix(orResp.Created, 0),
		Raw:       raw,
		Usage: Usage{
			InputTokens:  orResp.Usage.PromptTokens,
			OutputTokens: orResp.Usage.CompletionTokens,
			TotalTokens:  orResp.Usage.TotalTokens,
		},
	}

	if len(orResp.Choices) > 0 {
		choice := orResp.Choices[0]
		resp.FinishReason = FinishReason{
			Reason: normalizeFinishReason(choice.FinishReason),
			Raw:    choice.FinishReason,
		}
		resp.Message = c.convertResponseMessage(choice.Message)
	}

	return resp
}

func (c *Client) convertResponseMessage(orMsg openRouterMessage) Message {
	msg := Message{
		Role: Role(orMsg.Role),
		Name: orMsg.Name,
	}

	// Handle content
	switch content := orMsg.Content.(type) {
	case string:
		if content != "" {
			msg.Content = append(msg.Content, ContentPart{
				Kind: ContentText,
				Text: content,
			})
		}
	case []any:
		for _, part := range content {
			if partMap, ok := part.(map[string]any); ok {
				if partMap["type"] == "text" {
					if text, ok := partMap["text"].(string); ok {
						msg.Content = append(msg.Content, ContentPart{
							Kind: ContentText,
							Text: text,
						})
					}
				}
			}
		}
	}

	// Handle tool calls
	for _, tc := range orMsg.ToolCalls {
		msg.Content = append(msg.Content, ContentPart{
			Kind: ContentToolCall,
			ToolCall: &ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: json.RawMessage(tc.Function.Arguments),
			},
		})
	}

	return msg
}

func normalizeFinishReason(reason string) string {
	switch reason {
	case "stop":
		return "stop"
	case "length":
		return "length"
	case "tool_calls", "function_call":
		return "tool_calls"
	case "content_filter":
		return "content_filter"
	default:
		if reason != "" {
			return reason
		}
		return "stop"
	}
}

// Complete sends a completion request and returns the full response.
func (c *Client) Complete(ctx context.Context, req Request) (Response, error) {
	req.Stream = false
	orReq := c.convertRequest(req)

	body, err := json.Marshal(orReq)
	if err != nil {
		return Response{}, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("HTTP-Referer", c.siteURL)
	httpReq.Header.Set("X-Title", c.siteName)

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("send request: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return Response{}, fmt.Errorf("read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		var errResp openRouterResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error != nil {
			return Response{}, fmt.Errorf("API error (%d): %s", httpResp.StatusCode, errResp.Error.Message)
		}
		return Response{}, fmt.Errorf("API error (%d): %s", httpResp.StatusCode, string(respBody))
	}

	var orResp openRouterResponse
	if err := json.Unmarshal(respBody, &orResp); err != nil {
		return Response{}, fmt.Errorf("unmarshal response: %w", err)
	}

	if orResp.Error != nil {
		return Response{}, fmt.Errorf("API error: %s", orResp.Error.Message)
	}

	return c.convertResponse(orResp, respBody), nil
}

// Stream sends a completion request and returns a channel of streaming events.
func (c *Client) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	req.Stream = true
	orReq := c.convertRequest(req)

	body, err := json.Marshal(orReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("HTTP-Referer", c.siteURL)
	httpReq.Header.Set("X-Title", c.siteName)
	httpReq.Header.Set("Accept", "text/event-stream")

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		defer httpResp.Body.Close()
		respBody, _ := io.ReadAll(httpResp.Body)
		var errResp openRouterResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error != nil {
			return nil, fmt.Errorf("API error (%d): %s", httpResp.StatusCode, errResp.Error.Message)
		}
		return nil, fmt.Errorf("API error (%d): %s", httpResp.StatusCode, string(respBody))
	}

	events := make(chan StreamEvent, 100)

	go func() {
		defer httpResp.Body.Close()
		defer close(events)

		events <- StreamEvent{Type: StreamStart}

		scanner := bufio.NewScanner(httpResp.Body)
		// Increase buffer size for long lines
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

		var toolCalls = make(map[int]*ToolCall)

		for scanner.Scan() {
			line := scanner.Text()

			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}

			var chunk openRouterStreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			if len(chunk.Choices) == 0 {
				continue
			}

			choice := chunk.Choices[0]
			delta := choice.Delta

			// Handle text delta
			if delta.Content != "" {
				events <- StreamEvent{
					Type:  StreamTextDelta,
					Delta: delta.Content,
				}
			}

			// Handle tool calls
			for _, tc := range delta.ToolCalls {
				// Find or create tool call by index
				idx := 0 // OpenRouter may not always provide index
				if existing, ok := toolCalls[idx]; ok {
					// Accumulate arguments
					existing.Arguments = json.RawMessage(
						string(existing.Arguments) + tc.Function.Arguments,
					)
				} else {
					toolCalls[idx] = &ToolCall{
						ID:        tc.ID,
						Name:      tc.Function.Name,
						Arguments: json.RawMessage(tc.Function.Arguments),
					}
				}
			}

			// Handle finish
			if choice.FinishReason != "" {
				// Emit accumulated tool calls
				for _, tc := range toolCalls {
					events <- StreamEvent{
						Type:     StreamToolCall,
						ToolCall: tc,
					}
				}

				finishReason := FinishReason{
					Reason: normalizeFinishReason(choice.FinishReason),
					Raw:    choice.FinishReason,
				}

				var usage *Usage
				if chunk.Usage != nil {
					usage = &Usage{
						InputTokens:  chunk.Usage.PromptTokens,
						OutputTokens: chunk.Usage.CompletionTokens,
						TotalTokens:  chunk.Usage.TotalTokens,
					}
				}

				events <- StreamEvent{
					Type:         StreamFinish,
					FinishReason: &finishReason,
					Usage:        usage,
				}
			}
		}

		if err := scanner.Err(); err != nil {
			events <- StreamEvent{
				Type:  StreamError,
				Error: err,
			}
		}
	}()

	return events, nil
}

// CollectStream collects all events from a stream into a single Response.
func CollectStream(events <-chan StreamEvent) (Response, error) {
	var resp Response
	var textBuilder strings.Builder
	var toolCalls []ToolCall

	for event := range events {
		switch event.Type {
		case StreamTextDelta:
			textBuilder.WriteString(event.Delta)
		case StreamToolCall:
			if event.ToolCall != nil {
				toolCalls = append(toolCalls, *event.ToolCall)
			}
		case StreamFinish:
			if event.FinishReason != nil {
				resp.FinishReason = *event.FinishReason
			}
			if event.Usage != nil {
				resp.Usage = *event.Usage
			}
		case StreamError:
			return resp, event.Error
		}
	}

	// Build the message
	text := textBuilder.String()
	if text != "" {
		resp.Message.Content = append(resp.Message.Content, ContentPart{
			Kind: ContentText,
			Text: text,
		})
	}
	for _, tc := range toolCalls {
		tc := tc // capture
		resp.Message.Content = append(resp.Message.Content, ContentPart{
			Kind:     ContentToolCall,
			ToolCall: &tc,
		})
	}
	resp.Message.Role = RoleAssistant

	return resp, nil
}

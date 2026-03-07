// Package agent provides the coding agent loop implementation.
package agent

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/rhystic/attractor/pkg/events"
	"github.com/rhystic/attractor/pkg/llm"
	"github.com/rhystic/attractor/pkg/tools"
)

// SessionState represents the current state of an agent session.
type SessionState string

const (
	StateIdle          SessionState = "idle"
	StateProcessing    SessionState = "processing"
	StateAwaitingInput SessionState = "awaiting_input"
	StateClosed        SessionState = "closed"
)

// Config holds session configuration.
type Config struct {
	MaxTurns                int            // 0 = unlimited
	MaxToolRoundsPerInput   int            // 0 = unlimited
	DefaultCommandTimeoutMs int            // Default: 10000 (10s)
	MaxCommandTimeoutMs     int            // Default: 600000 (10min)
	ReasoningEffort         string         // "low", "medium", "high"
	ToolOutputLimits        map[string]int // Per-tool char limits
	EnableLoopDetection     bool           // Default: true
	LoopDetectionWindow     int            // Default: 10
	Model                   string         // Model identifier
	SystemPrompt            string         // Custom system prompt
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		MaxTurns:                0,
		MaxToolRoundsPerInput:   0,
		DefaultCommandTimeoutMs: 10000,
		MaxCommandTimeoutMs:     600000,
		ReasoningEffort:         "high",
		EnableLoopDetection:     true,
		LoopDetectionWindow:     10,
		Model:                   "minimax/minimax-m2.5",
		ToolOutputLimits: map[string]int{
			"read_file": 50000,
			"shell":     100000,
			"grep":      50000,
			"glob":      50000,
		},
	}
}

// Turn represents a single entry in the conversation history.
type Turn struct {
	Role      llm.Role       `json:"role"`
	Content   string         `json:"content,omitempty"`
	ToolCalls []llm.ToolCall `json:"tool_calls,omitempty"`
	Results   []ToolResult   `json:"results,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

// ToolResult holds a tool execution result.
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error"`
}

// Session represents an agent session.
type Session struct {
	ID           string
	Config       Config
	State        SessionState
	History      []Turn
	Client       *llm.Client
	ToolRegistry *tools.Registry
	ExecEnv      tools.ExecutionEnvironment
	Events       *events.Emitter

	mu            sync.Mutex
	steeringQueue []string
	followupQueue []string
	abortCh       chan struct{}
	totalTokens   llm.Usage
}

// NewSession creates a new agent session.
func NewSession(client *llm.Client, cfg Config) *Session {
	if cfg.Model == "" {
		cfg.Model = DefaultConfig().Model
	}

	return &Session{
		ID:           generateID(),
		Config:       cfg,
		State:        StateIdle,
		Client:       client,
		ToolRegistry: tools.CreateDefaultRegistry(),
		ExecEnv:      tools.NewLocalExecutionEnvironment(""),
		Events:       events.NewEmitter(generateID()),
		abortCh:      make(chan struct{}),
	}
}

// Submit processes user input through the agent loop.
func (s *Session) Submit(ctx context.Context, input string) error {
	s.mu.Lock()
	if s.State != StateIdle && s.State != StateAwaitingInput {
		s.mu.Unlock()
		return fmt.Errorf("session not ready: %s", s.State)
	}
	s.State = StateProcessing
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		if s.State == StateProcessing {
			s.State = StateIdle
		}
		s.mu.Unlock()
	}()

	// Add user turn
	s.History = append(s.History, Turn{
		Role:      llm.RoleUser,
		Content:   input,
		Timestamp: time.Now(),
	})

	s.Events.Emit(events.Event{
		Type: events.EventLog,
		Data: events.EventData{
			Level:   "info",
			Message: fmt.Sprintf("Processing input: %s", truncate(input, 100)),
		},
	})

	// Drain any pending steering messages
	s.drainSteering()

	roundCount := 0

	for {
		// Check limits
		if s.Config.MaxToolRoundsPerInput > 0 && roundCount >= s.Config.MaxToolRoundsPerInput {
			s.Events.EmitLog("warn", fmt.Sprintf("Tool round limit reached (%d)", roundCount))
			break
		}

		if s.Config.MaxTurns > 0 && len(s.History) >= s.Config.MaxTurns {
			s.Events.EmitLog("warn", fmt.Sprintf("Turn limit reached (%d)", len(s.History)))
			break
		}

		// Check abort
		select {
		case <-s.abortCh:
			return fmt.Errorf("session aborted")
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Build and send request
		resp, err := s.callLLM(ctx)
		if err != nil {
			s.Events.EmitLLMError("", err)
			s.Events.EmitError("", err)
			return err
		}

		// Record assistant turn
		assistantTurn := Turn{
			Role:      llm.RoleAssistant,
			Content:   resp.Text(),
			ToolCalls: resp.ToolCalls(),
			Timestamp: time.Now(),
		}
		s.History = append(s.History, assistantTurn)

		s.Events.Emit(events.Event{
			Type: events.EventLLMEnd,
			Data: events.EventData{
				Response:     truncate(resp.Text(), 500),
				InputTokens:  resp.Usage.InputTokens,
				OutputTokens: resp.Usage.OutputTokens,
			},
		})

		// If no tool calls, natural completion
		toolCalls := resp.ToolCalls()
		if len(toolCalls) == 0 {
			break
		}

		// Execute tool calls
		roundCount++
		results := s.executeToolCalls(ctx, toolCalls)

		// Record tool results turn
		s.History = append(s.History, Turn{
			Role:      llm.RoleTool,
			Results:   results,
			Timestamp: time.Now(),
		})

		// Drain steering messages
		s.drainSteering()

		// Loop detection
		if s.Config.EnableLoopDetection {
			if s.detectLoop() {
				warning := fmt.Sprintf("Loop detected: the last %d tool calls follow a repeating pattern. Try a different approach.",
					s.Config.LoopDetectionWindow)
				s.History = append(s.History, Turn{
					Role:      llm.RoleUser,
					Content:   warning,
					Timestamp: time.Now(),
				})
				s.Events.EmitLoopDetected("", warning)
				s.Events.EmitLog("warn", warning)
			}
		}
	}

	// Process follow-ups
	s.mu.Lock()
	if len(s.followupQueue) > 0 {
		nextInput := s.followupQueue[0]
		s.followupQueue = s.followupQueue[1:]
		s.mu.Unlock()
		return s.Submit(ctx, nextInput)
	}
	s.mu.Unlock()

	return nil
}

// Steer adds a steering message to be injected between tool rounds.
func (s *Session) Steer(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.steeringQueue = append(s.steeringQueue, message)
}

// FollowUp queues a message to process after current input completes.
func (s *Session) FollowUp(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.followupQueue = append(s.followupQueue, message)
}

// Abort signals the session to stop.
func (s *Session) Abort() {
	close(s.abortCh)
	s.mu.Lock()
	s.State = StateClosed
	s.mu.Unlock()
}

// LastResponse returns the last assistant response text.
func (s *Session) LastResponse() string {
	for i := len(s.History) - 1; i >= 0; i-- {
		if s.History[i].Role == llm.RoleAssistant && s.History[i].Content != "" {
			return s.History[i].Content
		}
	}
	return ""
}

// TotalUsage returns accumulated token usage.
func (s *Session) TotalUsage() llm.Usage {
	return s.totalTokens
}

// Model returns the model being used by this session.
func (s *Session) Model() string {
	return s.Config.Model
}

func (s *Session) callLLM(ctx context.Context) (llm.Response, error) {
	// Build messages
	messages := []llm.Message{
		llm.NewSystemMessage(s.buildSystemPrompt()),
	}

	for _, turn := range s.History {
		switch turn.Role {
		case llm.RoleUser:
			messages = append(messages, llm.NewUserMessage(turn.Content))
		case llm.RoleAssistant:
			msg := llm.Message{Role: llm.RoleAssistant}
			if turn.Content != "" {
				msg.Content = append(msg.Content, llm.ContentPart{
					Kind: llm.ContentText,
					Text: turn.Content,
				})
			}
			for _, tc := range turn.ToolCalls {
				msg.Content = append(msg.Content, llm.ContentPart{
					Kind: llm.ContentToolCall,
					ToolCall: &llm.ToolCall{
						ID:        tc.ID,
						Name:      tc.Name,
						Arguments: tc.Arguments,
					},
				})
			}
			messages = append(messages, msg)
		case llm.RoleTool:
			for _, result := range turn.Results {
				messages = append(messages, llm.NewToolResultMessage(
					result.ToolCallID,
					result.Content,
					result.IsError,
				))
			}
		}
	}

	// Build tool definitions
	var toolDefs []llm.ToolDefinition
	for _, tool := range s.ToolRegistry.All() {
		toolDefs = append(toolDefs, llm.ToolDefinition{
			Name:        tool.Name(),
			Description: tool.Description(),
			Parameters:  tool.Parameters(),
		})
	}

	req := llm.Request{
		Model:           s.Config.Model,
		Messages:        messages,
		Tools:           toolDefs,
		ReasoningEffort: s.Config.ReasoningEffort,
	}

	s.Events.EmitLLMStart("", s.Config.Model, truncate(messages[len(messages)-1].Text(), 200))

	resp, err := s.Client.Complete(ctx, req)
	if err != nil {
		return llm.Response{}, err
	}

	s.totalTokens = s.totalTokens.Add(resp.Usage)
	return resp, nil
}

func (s *Session) executeToolCalls(ctx context.Context, toolCalls []llm.ToolCall) []ToolResult {
	var results []ToolResult

	for _, tc := range toolCalls {
		s.Events.EmitToolStart("", tc.Name, string(tc.Arguments))

		tool, ok := s.ToolRegistry.Get(tc.Name)
		if !ok {
			toolErr := fmt.Errorf("unknown tool: %s", tc.Name)
			result := ToolResult{
				ToolCallID: tc.ID,
				Content:    toolErr.Error(),
				IsError:    true,
			}
			results = append(results, result)
			s.Events.EmitToolError("", tc.Name, toolErr)
			s.Events.EmitToolEnd("", tc.Name, result.Content, true)
			continue
		}

		output, err := tool.Execute(ctx, s.ExecEnv, tc.Arguments)
		if err != nil {
			result := ToolResult{
				ToolCallID: tc.ID,
				Content:    fmt.Sprintf("Tool error: %s", err),
				IsError:    true,
			}
			results = append(results, result)
			s.Events.EmitToolError("", tc.Name, err)
			s.Events.EmitToolEnd("", tc.Name, result.Content, true)
			continue
		}

		// Truncate output
		truncated := s.truncateToolOutput(tc.Name, output)

		result := ToolResult{
			ToolCallID: tc.ID,
			Content:    truncated,
			IsError:    false,
		}
		results = append(results, result)
		s.Events.EmitToolOutput("", tc.Name, output)
		s.Events.EmitToolEnd("", tc.Name, output, false)
	}

	return results
}

func (s *Session) truncateToolOutput(toolName, output string) string {
	limit := 100000 // Default
	if l, ok := s.Config.ToolOutputLimits[toolName]; ok {
		limit = l
	}

	if len(output) <= limit {
		return output
	}

	// Head/tail truncation
	half := limit / 2
	removed := len(output) - limit

	return output[:half] +
		fmt.Sprintf("\n\n[WARNING: Output truncated. %d characters removed from middle.]\n\n", removed) +
		output[len(output)-half:]
}

func (s *Session) drainSteering() {
	s.mu.Lock()
	queue := s.steeringQueue
	s.steeringQueue = nil
	s.mu.Unlock()

	for _, msg := range queue {
		s.History = append(s.History, Turn{
			Role:      llm.RoleUser,
			Content:   "[STEERING] " + msg,
			Timestamp: time.Now(),
		})
		s.Events.EmitLog("info", fmt.Sprintf("Steering injected: %s", truncate(msg, 100)))
	}
}

func (s *Session) detectLoop() bool {
	window := s.Config.LoopDetectionWindow
	if window <= 0 {
		window = 10
	}

	// Collect recent tool call signatures
	var signatures []string
	count := 0
	for i := len(s.History) - 1; i >= 0 && count < window; i-- {
		if len(s.History[i].ToolCalls) > 0 {
			for _, tc := range s.History[i].ToolCalls {
				sig := tc.Name + ":" + string(tc.Arguments)
				signatures = append([]string{sig}, signatures...)
				count++
				if count >= window {
					break
				}
			}
		}
	}

	if len(signatures) < window {
		return false
	}

	// Check for repeating patterns of length 1, 2, or 3
	for patternLen := 1; patternLen <= 3; patternLen++ {
		if window%patternLen != 0 {
			continue
		}
		pattern := signatures[:patternLen]
		allMatch := true
		for i := patternLen; i < window; i += patternLen {
			for j := 0; j < patternLen; j++ {
				if signatures[i+j] != pattern[j] {
					allMatch = false
					break
				}
			}
			if !allMatch {
				break
			}
		}
		if allMatch {
			return true
		}
	}

	return false
}

func (s *Session) buildSystemPrompt() string {
	if s.Config.SystemPrompt != "" {
		return s.Config.SystemPrompt
	}

	return fmt.Sprintf(`You are an expert coding agent. You help users with software engineering tasks by reading files, editing code, running commands, and iterating until tasks are complete.

## Environment
- Platform: %s
- Working Directory: %s
- Current Time: %s

## Guidelines
1. Use tools to accomplish tasks. Don't just describe what to do - actually do it.
2. Read files before editing to understand the current state.
3. After making changes, verify they work by running appropriate tests or commands.
4. If something fails, analyze the error and try a different approach.
5. Be thorough but efficient. Don't read more than necessary.
6. When editing files, provide enough context in old_string to make the match unique.
7. For shell commands, prefer simple commands over complex pipelines when possible.

## Important
- Always verify your changes work before considering a task complete.
- If you're unsure about something, gather more information first.
- Don't make assumptions about file contents without reading them.`,
		runtime.GOOS,
		s.ExecEnv.WorkingDirectory(),
		time.Now().Format(time.RFC3339),
	)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// Generate runs the agent on a single prompt and returns the response.
func Generate(ctx context.Context, client *llm.Client, prompt string, cfg Config) (string, error) {
	session := NewSession(client, cfg)
	if err := session.Submit(ctx, prompt); err != nil {
		return "", err
	}
	return session.LastResponse(), nil
}

// StreamGenerate runs the agent and streams events.
func StreamGenerate(ctx context.Context, client *llm.Client, prompt string, cfg Config) (<-chan events.Event, error) {
	session := NewSession(client, cfg)
	eventCh := session.Events.Subscribe()

	go func() {
		defer session.Events.Close()
		if err := session.Submit(ctx, prompt); err != nil {
			session.Events.EmitError("", err)
		}
	}()

	return eventCh, nil
}

// RunTask runs a task description through the agent.
func RunTask(ctx context.Context, client *llm.Client, task string) (string, error) {
	cfg := DefaultConfig()
	return Generate(ctx, client, task, cfg)
}

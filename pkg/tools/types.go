// Package tools provides the tool system for the coding agent.
package tools

import (
	"context"
	"encoding/json"
)

// Tool represents a tool available to the LLM.
type Tool interface {
	// Name returns the tool's unique identifier.
	Name() string

	// Description returns a description for the LLM.
	Description() string

	// Parameters returns the JSON Schema for the tool's parameters.
	Parameters() json.RawMessage

	// Execute runs the tool with the given arguments.
	Execute(ctx context.Context, env ExecutionEnvironment, args json.RawMessage) (string, error)
}

// ExecutionEnvironment abstracts where tools run (local, docker, etc.).
type ExecutionEnvironment interface {
	// ReadFile reads a file's contents.
	ReadFile(ctx context.Context, path string, offset, limit int) (string, error)

	// WriteFile writes content to a file.
	WriteFile(ctx context.Context, path string, content string) error

	// FileExists checks if a file exists.
	FileExists(ctx context.Context, path string) (bool, error)

	// ListDirectory lists directory contents.
	ListDirectory(ctx context.Context, path string, depth int) ([]DirEntry, error)

	// ExecCommand executes a shell command.
	ExecCommand(ctx context.Context, command string, timeoutMs int, workingDir string, env map[string]string) (*ExecResult, error)

	// Grep searches file contents.
	Grep(ctx context.Context, pattern, path string, opts GrepOptions) (string, error)

	// Glob finds files matching a pattern.
	Glob(ctx context.Context, pattern, path string) ([]string, error)

	// WorkingDirectory returns the current working directory.
	WorkingDirectory() string

	// Platform returns the platform identifier.
	Platform() string
}

// DirEntry represents a directory entry.
type DirEntry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size,omitempty"`
}

// ExecResult holds the result of command execution.
type ExecResult struct {
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ExitCode   int    `json:"exit_code"`
	TimedOut   bool   `json:"timed_out"`
	DurationMs int    `json:"duration_ms"`
}

// GrepOptions configures grep behavior.
type GrepOptions struct {
	GlobFilter      string `json:"glob_filter,omitempty"`
	CaseInsensitive bool   `json:"case_insensitive,omitempty"`
	MaxResults      int    `json:"max_results,omitempty"`
}

// Registry manages available tools.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry creates a new tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry.
func (r *Registry) Register(tool Tool) {
	r.tools[tool.Name()] = tool
}

// Get retrieves a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	tool, ok := r.tools[name]
	return tool, ok
}

// Names returns all registered tool names.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// All returns all registered tools.
func (r *Registry) All() []Tool {
	tools := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}
	return tools
}

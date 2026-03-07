# AGENTS.md - Development Guidelines for Attractor

## Project Overview

**Attractor** is a Go CLI application that executes AI-driven DOT graph pipelines and agentic workflows.

- **Language:** Go 1.25.0
- **Module:** `github.com/rhystic/attractor`
- **Architecture:** Event-driven pipeline engine with LLM integration via OpenRouter
- **Main packages:** agent, context, dot, engine, events, handlers, llm, tools

## Build & Test Commands

### Build
```bash
go build ./cmd/attractor      # Build the CLI binary
go build ./...                 # Build all packages
```

### Run
```bash
./attractor run <pipeline.dot>   # Execute a DOT pipeline
./attractor agent "prompt"       # Run coding agent with prompt
./attractor validate <file.dot>  # Validate DOT syntax
```

### Test
```bash
go test ./...                                      # Run all tests
go test -v ./...                                   # Verbose mode
go test -run TestEngineSimplePipeline ./pkg/engine/...  # Single test
go test -count=1 ./...                             # Disable test caching
go test -race ./...                                # Detect race conditions
go test -cover ./...                               # Show coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out                   # View coverage report
```

### Lint & Format
```bash
go fmt ./...                   # Format all Go code
go vet ./...                   # Run static analysis
gofmt -s -w .                  # Simplify and write
```

## Code Style Guidelines

### Imports
- **Order:** Standard library → blank line → external packages
- **Organization:** Group related imports together
- **Aliases for clarity:** Use aliases for package name conflicts (e.g., `pcontext "github.com/rhystic/attractor/pkg/context"`)

```go
import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/rhystic/attractor/pkg/agent"
	pcontext "github.com/rhystic/attractor/pkg/context"
	"github.com/rhystic/attractor/pkg/engine"
)
```

### Naming Conventions
- **Packages:** lowercase, short, singular (e.g., `engine`, `llm`, `dot`, not `engines`)
- **Variables:** camelCase for local vars, PascalCase for exported
- **Constants:** ALL_CAPS for package-level constants
- **Receivers:** Single letter or short abbreviation (e.g., `func (e *Engine)`)
- **Interfaces:** End with `-er` suffix (e.g., `Reader`, `Writer`)

### Types & Structs
- **Exported types:** Use descriptive names with clear responsibility
- **Private fields:** Use lowercase; provide getters/setters if needed
- **Functional options pattern:** Prefer for complex initialization

```go
type ClientOption func(*Client)

func WithBaseURL(url string) ClientOption {
	return func(c *Client) { c.baseURL = url }
}

client := NewClient(apiKey, WithBaseURL("https://api.example.com"))
```

### Error Handling
- **Wrap errors:** Always use `fmt.Errorf("context: %w", err)` to preserve error chain
- **Return early:** Check errors immediately, don't defer error handling
- **Error messages:** Start lowercase, describe what failed and why
- **No silent failures:** Never ignore errors unless explicitly documented

```go
content, err := os.ReadFile(filename)
if err != nil {
	return fmt.Errorf("read file: %w", err)
}
```

### Testing
- **Table-driven tests:** Use for multiple cases
- **Use `t.Run()`:** Organize subtests for clarity
- **Helper functions:** Extract common setup into helper functions
- **Naming:** `Test<Function><Scenario>` pattern
- **Assertions:** Use explicit comparisons, fail with descriptive messages

```go
func TestEngineEdgeSelection(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"case 1", "input1", "expected1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := doThing(tt.input)
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}
```

### Comments & Documentation
- **Package docs:** Every package has a comment explaining its purpose
- **Exported items:** Document all exported functions, types, constants
- **Godoc format:** `// FunctionName does X and returns Y.`
- **Inline comments:** Explain *why*, not *what* (code shows what)

```go
// Package engine orchestrates DOT graph execution with LLM integration.
package engine

// Run executes the pipeline graph with the given context.
func (e *Engine) Run(ctx context.Context) (Outcome, error) {
	// ...
}
```

### Code Organization
- **One responsibility per file:** Group related functions
- **Interfaces first:** Define contracts before implementations
- **Constants/types together:** Group at file top after imports
- **Tests:** Keep `*_test.go` files alongside implementation

### Formatting & Style
- **Line length:** Keep functions concise, prefer extraction over nesting
- **Blank lines:** Separate logical sections (imports, const, type, func)
- **Idiomatic Go:** Follow Go conventions, use standard library patterns
- **No magic numbers:** Use named constants for configuration values

## Project-Specific Conventions

### DOT Graph Processing
- Nodes use special shapes: `Mdiamond` (start), `Msquare` (exit), `diamond` (decision)
- Edge routing: Prefer higher `weight` values; tiebreak lexically by target node ID
- Edge labels: Use for routing logic (`[Y] Yes`, `[N] No`) and conditions

### Event System
- Subscribe to engine events for non-blocking updates
- Event types: `EventNodeStart`, `EventNodeEnd`, `EventLLMStart`, `EventToolStart`, etc.
- Handlers should not block the event loop

### Error Types
- Use `pcontext.Outcome` for node results with status and failure reasons
- Status values: `StatusSuccess`, `StatusPartialSuccess`, `StatusFail`
- Return wrapped errors with context for debugging

### Configuration
- Use functional options for component initialization
- Defaults should be sensible and documented
- Make config immutable after construction

## Git Conventions

- **Commit messages:** Conventional commits (`feat:`, `fix:`, `refactor:`, `test:`, `docs:`)
- **Scope optional:** Include package/module name (e.g., `feat(engine):`)
- **Description:** Lowercase, imperative mood, no period, max 50 chars
- **Example:** `fix(llm): handle streaming timeout errors`

## Running Locally

```bash
# Set API key
export OPENROUTER_API_KEY="your-key-here"

# Build
go build -o attractor ./cmd/attractor

# Test with example
./attractor validate examples/simple.dot
./attractor run examples/simple.dot

# Run all tests
go test ./...
```

## Key Files & Entry Points

- `cmd/attractor/main.go` - CLI entry point, argument parsing, command dispatch
- `pkg/engine/engine.go` - Core pipeline execution engine
- `pkg/agent/agent.go` - Agentic workflows and sessions
- `pkg/llm/client.go` - OpenRouter API integration
- `pkg/dot/parser.go` - DOT graph parser
- `pkg/events/events.go` - Event system
- `pkg/context/context.go` - Pipeline context and state management

## Performance Notes

- **Concurrency:** Engine uses goroutines for event handling; safe for concurrent node execution
- **Retries:** Exponential backoff with jitter; configurable max retries per node
- **Timeouts:** Pipeline-level timeout with signal handling for graceful shutdown
- **Resource cleanup:** Always defer engine and session cleanup


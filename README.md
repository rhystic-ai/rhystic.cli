# Attractor

A Go CLI application that executes AI-driven DOT graph pipelines with LLM integration via OpenRouter.

## Quick Start

```bash
# Set API key
export OPENROUTER_API_KEY="your-api-key"

# Build
go build -o attractor ./cmd/attractor

# Run a pipeline
./attractor run pipeline.dot

# Run the coding agent
./attractor agent "Fix the bug in main.go"

# Validate a pipeline
./attractor validate pipeline.dot
```

## Features

- **DOT-based Workflows:** Define AI pipelines as DOT graphs with decision nodes, retries, and goal gates
- **LLM Integration:** Execute nodes via OpenRouter with configurable models and streaming
- **Event System:** Non-blocking event subscription for real-time pipeline monitoring
- **Tool Execution:** Built-in tools for shell commands, file operations, and LLM calls
- **Human Gates:** Interactive nodes requiring human approval or input
- **Retry Logic:** Exponential backoff with configurable retry targets and max attempts

## Architecture

- **Engine:** Core pipeline execution with graph traversal and node orchestration
- **Agent:** Agentic workflows supporting multi-turn interactions and tool use
- **DOT Parser:** Full DOT syntax support for pipeline definitions
- **LLM Client:** OpenRouter integration with streaming and tool calling

## Development

See [AGENTS.md](./AGENTS.md) for comprehensive development guidelines, build commands, and code style.

```bash
go test ./...                           # Run tests
go test -run TestName ./pkg/package/... # Run single test
go fmt ./...                            # Format code
go vet ./...                            # Lint
```

## Environment

- **Go 1.25.0**
- **OPENROUTER_API_KEY** - Required for LLM calls

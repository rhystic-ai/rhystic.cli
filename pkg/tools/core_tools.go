package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ReadFileTool reads files from the filesystem.
type ReadFileTool struct{}

func (t *ReadFileTool) Name() string { return "read_file" }

func (t *ReadFileTool) Description() string {
	return "Read a file from the filesystem. Returns line-numbered content."
}

func (t *ReadFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"file_path": {
				"type": "string",
				"description": "Absolute or relative path to the file"
			},
			"offset": {
				"type": "integer",
				"description": "1-based line number to start reading from"
			},
			"limit": {
				"type": "integer",
				"description": "Maximum number of lines to read (default: 2000)"
			}
		},
		"required": ["file_path"]
	}`)
}

func (t *ReadFileTool) Execute(ctx context.Context, env ExecutionEnvironment, args json.RawMessage) (string, error) {
	var params struct {
		FilePath string `json:"file_path"`
		Offset   int    `json:"offset"`
		Limit    int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 2000
	}

	content, err := env.ReadFile(ctx, params.FilePath, params.Offset, limit)
	if err != nil {
		return "", err
	}

	return content, nil
}

// WriteFileTool writes files to the filesystem.
type WriteFileTool struct{}

func (t *WriteFileTool) Name() string { return "write_file" }

func (t *WriteFileTool) Description() string {
	return "Write content to a file. Creates the file and parent directories if needed."
}

func (t *WriteFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"file_path": {
				"type": "string",
				"description": "Absolute or relative path to the file"
			},
			"content": {
				"type": "string",
				"description": "The full file content"
			}
		},
		"required": ["file_path", "content"]
	}`)
}

func (t *WriteFileTool) Execute(ctx context.Context, env ExecutionEnvironment, args json.RawMessage) (string, error) {
	var params struct {
		FilePath string `json:"file_path"`
		Content  string `json:"content"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}

	if err := env.WriteFile(ctx, params.FilePath, params.Content); err != nil {
		return "", err
	}

	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(params.Content), params.FilePath), nil
}

// EditFileTool edits files using search-and-replace.
type EditFileTool struct{}

func (t *EditFileTool) Name() string { return "edit_file" }

func (t *EditFileTool) Description() string {
	return "Replace an exact string occurrence in a file. The old_string must be unique unless replace_all is true."
}

func (t *EditFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"file_path": {
				"type": "string",
				"description": "Path to the file to edit"
			},
			"old_string": {
				"type": "string",
				"description": "Exact text to find"
			},
			"new_string": {
				"type": "string",
				"description": "Replacement text"
			},
			"replace_all": {
				"type": "boolean",
				"description": "Replace all occurrences (default: false)"
			}
		},
		"required": ["file_path", "old_string", "new_string"]
	}`)
}

func (t *EditFileTool) Execute(ctx context.Context, env ExecutionEnvironment, args json.RawMessage) (string, error) {
	var params struct {
		FilePath   string `json:"file_path"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}

	// Read the file (without line numbers)
	content, err := env.ReadFile(ctx, params.FilePath, 0, 0)
	if err != nil {
		return "", err
	}

	// Strip line numbers from the content
	lines := strings.Split(content, "\n")
	var rawLines []string
	for _, line := range lines {
		// Remove "N: " prefix
		if idx := strings.Index(line, ": "); idx > 0 {
			line = line[idx+2:]
		}
		rawLines = append(rawLines, line)
	}
	rawContent := strings.Join(rawLines, "\n")

	// Count occurrences
	count := strings.Count(rawContent, params.OldString)
	if count == 0 {
		return "", fmt.Errorf("old_string not found in file")
	}

	if count > 1 && !params.ReplaceAll {
		return "", fmt.Errorf("found %d occurrences of old_string; provide more context to make it unique or set replace_all=true", count)
	}

	// Perform replacement
	var newContent string
	if params.ReplaceAll {
		newContent = strings.ReplaceAll(rawContent, params.OldString, params.NewString)
	} else {
		newContent = strings.Replace(rawContent, params.OldString, params.NewString, 1)
	}

	// Write the file
	if err := env.WriteFile(ctx, params.FilePath, newContent); err != nil {
		return "", err
	}

	return fmt.Sprintf("Successfully replaced %d occurrence(s)", count), nil
}

// ShellTool executes shell commands.
type ShellTool struct{}

func (t *ShellTool) Name() string { return "shell" }

func (t *ShellTool) Description() string {
	return "Execute a shell command. Returns stdout, stderr, and exit code."
}

func (t *ShellTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {
				"type": "string",
				"description": "The command to run"
			},
			"timeout_ms": {
				"type": "integer",
				"description": "Timeout in milliseconds (default: 10000)"
			},
			"working_dir": {
				"type": "string",
				"description": "Working directory for the command"
			},
			"description": {
				"type": "string",
				"description": "Human-readable description of what this command does"
			}
		},
		"required": ["command"]
	}`)
}

func (t *ShellTool) Execute(ctx context.Context, env ExecutionEnvironment, args json.RawMessage) (string, error) {
	var params struct {
		Command     string `json:"command"`
		TimeoutMs   int    `json:"timeout_ms"`
		WorkingDir  string `json:"working_dir"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}

	result, err := env.ExecCommand(ctx, params.Command, params.TimeoutMs, params.WorkingDir, nil)
	if err != nil {
		return "", err
	}

	var output strings.Builder
	if result.Stdout != "" {
		output.WriteString(result.Stdout)
	}
	if result.Stderr != "" {
		if output.Len() > 0 {
			output.WriteString("\n")
		}
		output.WriteString("[STDERR]\n")
		output.WriteString(result.Stderr)
	}

	if result.TimedOut {
		output.WriteString(fmt.Sprintf("\n[TIMEOUT after %dms]", result.DurationMs))
	} else if result.ExitCode != 0 {
		output.WriteString(fmt.Sprintf("\n[EXIT CODE: %d]", result.ExitCode))
	}

	return output.String(), nil
}

// GrepTool searches file contents.
type GrepTool struct{}

func (t *GrepTool) Name() string { return "grep" }

func (t *GrepTool) Description() string {
	return "Search file contents using regex patterns."
}

func (t *GrepTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {
				"type": "string",
				"description": "Regex pattern to search for"
			},
			"path": {
				"type": "string",
				"description": "Directory or file to search (default: working directory)"
			},
			"glob_filter": {
				"type": "string",
				"description": "File pattern filter (e.g., \"*.py\")"
			},
			"case_insensitive": {
				"type": "boolean",
				"description": "Case-insensitive search (default: false)"
			},
			"max_results": {
				"type": "integer",
				"description": "Maximum number of results (default: 100)"
			}
		},
		"required": ["pattern"]
	}`)
}

func (t *GrepTool) Execute(ctx context.Context, env ExecutionEnvironment, args json.RawMessage) (string, error) {
	var params struct {
		Pattern         string `json:"pattern"`
		Path            string `json:"path"`
		GlobFilter      string `json:"glob_filter"`
		CaseInsensitive bool   `json:"case_insensitive"`
		MaxResults      int    `json:"max_results"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}

	result, err := env.Grep(ctx, params.Pattern, params.Path, GrepOptions{
		GlobFilter:      params.GlobFilter,
		CaseInsensitive: params.CaseInsensitive,
		MaxResults:      params.MaxResults,
	})
	if err != nil {
		return "", err
	}

	if result == "" {
		return "No matches found", nil
	}

	return result, nil
}

// GlobTool finds files by pattern.
type GlobTool struct{}

func (t *GlobTool) Name() string { return "glob" }

func (t *GlobTool) Description() string {
	return "Find files matching a glob pattern."
}

func (t *GlobTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {
				"type": "string",
				"description": "Glob pattern (e.g., \"**/*.ts\")"
			},
			"path": {
				"type": "string",
				"description": "Base directory (default: working directory)"
			}
		},
		"required": ["pattern"]
	}`)
}

func (t *GlobTool) Execute(ctx context.Context, env ExecutionEnvironment, args json.RawMessage) (string, error) {
	var params struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}

	matches, err := env.Glob(ctx, params.Pattern, params.Path)
	if err != nil {
		return "", err
	}

	if len(matches) == 0 {
		return "No matches found", nil
	}

	return strings.Join(matches, "\n"), nil
}

// ListDirTool lists directory contents.
type ListDirTool struct{}

func (t *ListDirTool) Name() string { return "list_dir" }

func (t *ListDirTool) Description() string {
	return "List directory contents."
}

func (t *ListDirTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Directory path"
			},
			"depth": {
				"type": "integer",
				"description": "Maximum depth to traverse (default: 1)"
			}
		},
		"required": ["path"]
	}`)
}

func (t *ListDirTool) Execute(ctx context.Context, env ExecutionEnvironment, args json.RawMessage) (string, error) {
	var params struct {
		Path  string `json:"path"`
		Depth int    `json:"depth"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}

	if params.Path == "" {
		params.Path = env.WorkingDirectory()
	}

	entries, err := env.ListDirectory(ctx, params.Path, params.Depth)
	if err != nil {
		return "", err
	}

	var result strings.Builder
	for _, entry := range entries {
		if entry.IsDir {
			result.WriteString(entry.Name + "/\n")
		} else {
			result.WriteString(fmt.Sprintf("%s (%d bytes)\n", entry.Name, entry.Size))
		}
	}

	return result.String(), nil
}

// CreateDefaultRegistry creates a registry with all default tools.
func CreateDefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(&ReadFileTool{})
	r.Register(&WriteFileTool{})
	r.Register(&EditFileTool{})
	r.Register(&ShellTool{})
	r.Register(&GrepTool{})
	r.Register(&GlobTool{})
	r.Register(&ListDirTool{})
	return r
}

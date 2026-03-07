package tools

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"
)

// LocalExecutionEnvironment runs tools on the local machine.
type LocalExecutionEnvironment struct {
	workingDir string
	envFilter  EnvFilter
}

// EnvFilter controls which environment variables are passed to commands.
type EnvFilter struct {
	// InheritAll passes all environment variables (except excluded patterns).
	InheritAll bool
	// ExcludePatterns are glob patterns for variables to exclude.
	ExcludePatterns []string
	// IncludePatterns are glob patterns for variables to always include.
	IncludePatterns []string
}

// DefaultEnvFilter returns a sensible default filter.
func DefaultEnvFilter() EnvFilter {
	return EnvFilter{
		InheritAll: true,
		ExcludePatterns: []string{
			"*_API_KEY",
			"*_SECRET",
			"*_TOKEN",
			"*_PASSWORD",
			"*_CREDENTIAL",
			"AWS_*",
			"OPENROUTER_*",
			"OPENAI_*",
			"ANTHROPIC_*",
		},
		IncludePatterns: []string{
			"PATH",
			"HOME",
			"USER",
			"SHELL",
			"LANG",
			"TERM",
			"TMPDIR",
			"GOPATH",
			"GOROOT",
			"CARGO_HOME",
			"RUSTUP_HOME",
			"NVM_DIR",
			"NODE_PATH",
			"PYTHONPATH",
		},
	}
}

// NewLocalExecutionEnvironment creates a new local execution environment.
func NewLocalExecutionEnvironment(workingDir string) *LocalExecutionEnvironment {
	if workingDir == "" {
		workingDir, _ = os.Getwd()
	}
	return &LocalExecutionEnvironment{
		workingDir: workingDir,
		envFilter:  DefaultEnvFilter(),
	}
}

// WorkingDirectory returns the working directory.
func (e *LocalExecutionEnvironment) WorkingDirectory() string {
	return e.workingDir
}

// Platform returns the platform identifier.
func (e *LocalExecutionEnvironment) Platform() string {
	return runtime.GOOS
}

// ReadFile reads a file's contents with line numbers.
func (e *LocalExecutionEnvironment) ReadFile(ctx context.Context, path string, offset, limit int) (string, error) {
	path = e.resolvePath(path)

	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	lines := strings.Split(string(content), "\n")

	// Apply offset (1-indexed)
	if offset > 0 {
		offset-- // Convert to 0-indexed
		if offset >= len(lines) {
			return "", nil
		}
		lines = lines[offset:]
	}

	// Apply limit
	if limit > 0 && limit < len(lines) {
		lines = lines[:limit]
	}

	// Add line numbers
	startLine := 1
	if offset > 0 {
		startLine = offset + 1
	}

	var result strings.Builder
	for i, line := range lines {
		lineNum := startLine + i
		// Truncate very long lines
		if len(line) > 2000 {
			line = line[:2000] + "... [truncated]"
		}
		fmt.Fprintf(&result, "%d: %s\n", lineNum, line)
	}

	return result.String(), nil
}

// WriteFile writes content to a file.
func (e *LocalExecutionEnvironment) WriteFile(ctx context.Context, path string, content string) error {
	path = e.resolvePath(path)

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

// FileExists checks if a file exists.
func (e *LocalExecutionEnvironment) FileExists(ctx context.Context, path string) (bool, error) {
	path = e.resolvePath(path)
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ListDirectory lists directory contents.
func (e *LocalExecutionEnvironment) ListDirectory(ctx context.Context, path string, depth int) ([]DirEntry, error) {
	path = e.resolvePath(path)

	if depth <= 0 {
		depth = 1
	}

	var entries []DirEntry

	err := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		rel, _ := filepath.Rel(path, p)
		if rel == "." {
			return nil
		}

		// Check depth
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) > depth {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		info, _ := d.Info()
		var size int64
		if info != nil && !d.IsDir() {
			size = info.Size()
		}

		entries = append(entries, DirEntry{
			Name:  rel,
			IsDir: d.IsDir(),
			Size:  size,
		})

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walk directory: %w", err)
	}

	return entries, nil
}

// ExecCommand executes a shell command.
func (e *LocalExecutionEnvironment) ExecCommand(ctx context.Context, command string, timeoutMs int, workingDir string, env map[string]string) (*ExecResult, error) {
	if timeoutMs <= 0 {
		timeoutMs = 10000 // 10 second default
	}

	// Create timeout context
	timeout := time.Duration(timeoutMs) * time.Millisecond
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Determine shell
	shell := "/bin/bash"
	shellArg := "-c"
	if runtime.GOOS == "windows" {
		shell = "cmd.exe"
		shellArg = "/c"
	}

	cmd := exec.CommandContext(ctx, shell, shellArg, command)

	// Set working directory
	if workingDir != "" {
		cmd.Dir = e.resolvePath(workingDir)
	} else {
		cmd.Dir = e.workingDir
	}

	// Set up process group for clean killing
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Build environment
	cmd.Env = e.buildEnv(env)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	result := &ExecResult{
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		DurationMs: int(duration.Milliseconds()),
	}

	if ctx.Err() == context.DeadlineExceeded {
		result.TimedOut = true
		result.ExitCode = -1
		// Try to kill the process group
		if cmd.Process != nil {
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return result, nil
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("exec command: %w", err)
		}
	}

	return result, nil
}

// Grep searches file contents.
func (e *LocalExecutionEnvironment) Grep(ctx context.Context, pattern, path string, opts GrepOptions) (string, error) {
	if path == "" {
		path = e.workingDir
	} else {
		path = e.resolvePath(path)
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("invalid regex: %w", err)
	}
	if opts.CaseInsensitive {
		re, err = regexp.Compile("(?i)" + pattern)
		if err != nil {
			return "", fmt.Errorf("invalid regex: %w", err)
		}
	}

	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = 100
	}

	var results []string
	count := 0

	err = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			// Skip hidden and common non-code directories
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "__pycache__" {
				return fs.SkipDir
			}
			return nil
		}

		// Check glob filter
		if opts.GlobFilter != "" {
			matched, _ := filepath.Match(opts.GlobFilter, d.Name())
			if !matched {
				return nil
			}
		}

		// Read and search file
		content, err := os.ReadFile(p)
		if err != nil {
			return nil
		}

		lines := strings.Split(string(content), "\n")
		rel, _ := filepath.Rel(e.workingDir, p)

		for lineNum, line := range lines {
			if re.MatchString(line) {
				results = append(results, fmt.Sprintf("%s:%d: %s", rel, lineNum+1, line))
				count++
				if count >= maxResults {
					return fs.SkipAll
				}
			}
		}

		return nil
	})

	if err != nil && err != fs.SkipAll {
		return "", fmt.Errorf("grep: %w", err)
	}

	return strings.Join(results, "\n"), nil
}

// Glob finds files matching a pattern.
func (e *LocalExecutionEnvironment) Glob(ctx context.Context, pattern, path string) ([]string, error) {
	if path == "" {
		path = e.workingDir
	} else {
		path = e.resolvePath(path)
	}

	// Handle ** patterns
	if strings.Contains(pattern, "**") {
		return e.globRecursive(path, pattern)
	}

	// Simple glob
	matches, err := filepath.Glob(filepath.Join(path, pattern))
	if err != nil {
		return nil, fmt.Errorf("glob: %w", err)
	}

	// Convert to relative paths and sort by mtime
	type fileInfo struct {
		path  string
		mtime time.Time
	}
	var files []fileInfo
	for _, match := range matches {
		rel, _ := filepath.Rel(e.workingDir, match)
		info, err := os.Stat(match)
		if err != nil {
			continue
		}
		files = append(files, fileInfo{rel, info.ModTime()})
	}

	// Sort by mtime descending
	sort.Slice(files, func(i, j int) bool {
		return files[i].mtime.After(files[j].mtime)
	})

	result := make([]string, len(files))
	for i, f := range files {
		result[i] = f.path
	}

	return result, nil
}

func (e *LocalExecutionEnvironment) globRecursive(basePath, pattern string) ([]string, error) {
	// Split pattern into parts
	parts := strings.Split(pattern, "**")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid glob pattern: multiple ** not supported")
	}

	prefix := strings.TrimSuffix(parts[0], "/")
	suffix := strings.TrimPrefix(parts[1], "/")

	type fileInfo struct {
		path  string
		mtime time.Time
	}
	var files []fileInfo

	startPath := basePath
	if prefix != "" {
		startPath = filepath.Join(basePath, prefix)
	}

	err := filepath.WalkDir(startPath, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		// Skip hidden directories
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") {
			return fs.SkipDir
		}

		if d.IsDir() {
			return nil
		}

		// Check suffix pattern
		if suffix != "" {
			matched, _ := filepath.Match(suffix, d.Name())
			if !matched {
				// Also check full relative path
				rel, _ := filepath.Rel(startPath, p)
				matched, _ = filepath.Match(suffix, rel)
				if !matched {
					return nil
				}
			}
		}

		rel, _ := filepath.Rel(e.workingDir, p)
		info, _ := d.Info()
		mtime := time.Time{}
		if info != nil {
			mtime = info.ModTime()
		}
		files = append(files, fileInfo{rel, mtime})
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("glob recursive: %w", err)
	}

	// Sort by mtime descending
	sort.Slice(files, func(i, j int) bool {
		return files[i].mtime.After(files[j].mtime)
	})

	result := make([]string, len(files))
	for i, f := range files {
		result[i] = f.path
	}

	return result, nil
}

func (e *LocalExecutionEnvironment) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	if strings.HasPrefix(path, "~") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[1:])
	}
	return filepath.Join(e.workingDir, path)
}

func (e *LocalExecutionEnvironment) buildEnv(extra map[string]string) []string {
	result := make(map[string]string)

	// Start with filtered system environment
	for _, kv := range os.Environ() {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := parts[0]

		// Check if excluded
		excluded := false
		for _, pattern := range e.envFilter.ExcludePatterns {
			if matchGlob(pattern, key) {
				excluded = true
				break
			}
		}

		// Check if explicitly included
		for _, pattern := range e.envFilter.IncludePatterns {
			if matchGlob(pattern, key) {
				excluded = false
				break
			}
		}

		if !excluded {
			result[key] = parts[1]
		}
	}

	// Add extra environment
	for k, v := range extra {
		result[k] = v
	}

	// Convert to slice
	env := make([]string, 0, len(result))
	for k, v := range result {
		env = append(env, k+"="+v)
	}

	return env
}

func matchGlob(pattern, s string) bool {
	// Simple glob matching with * wildcard
	pattern = strings.ToUpper(pattern)
	s = strings.ToUpper(s)

	if pattern == "*" {
		return true
	}

	if strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") {
		return strings.Contains(s, pattern[1:len(pattern)-1])
	}

	if strings.HasPrefix(pattern, "*") {
		return strings.HasSuffix(s, pattern[1:])
	}

	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(s, pattern[:len(pattern)-1])
	}

	return pattern == s
}

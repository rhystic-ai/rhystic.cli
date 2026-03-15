// Package tools provides the lsp_analyze tool for pipeline agents.
package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// LSPClient manages a subprocess connection to attractor-lsp.
type LSPClient struct {
	binPath string
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	mu      sync.Mutex
	started bool
}

// NewLSPClient creates a new LSP client.
func NewLSPClient() *LSPClient {
	// Find attractor-lsp binary
	binPath := "./attractor-lsp"
	if _, err := os.Stat(binPath); err != nil {
		// Try to find in PATH
		binPath = "attractor-lsp"
	}

	return &LSPClient{
		binPath: binPath,
	}
}

// EnsureStarted ensures the LSP subprocess is running.
func (c *LSPClient) EnsureStarted() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started && c.cmd != nil && c.cmd.Process != nil {
		// Check if still running
		if err := c.cmd.Process.Signal(os.Signal(nil)); err == nil {
			return nil // Already running
		}
	}

	// Start new process
	cmd := exec.Command(c.binPath, "--no-agent") // No agent needed for diagnostics
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start lsp: %w", err)
	}

	c.cmd = cmd
	c.stdin = stdin
	c.stdout = stdout
	c.started = true

	// Send initialize
	if err := c.sendInitialize(); err != nil {
		c.Stop()
		return fmt.Errorf("initialize lsp: %w", err)
	}

	return nil
}

// Stop stops the LSP subprocess.
func (c *LSPClient) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cmd != nil && c.cmd.Process != nil {
		c.stdin.Close()
		c.cmd.Process.Kill()
		c.cmd.Wait()
	}
	c.started = false
}

// sendInitialize sends the initialize request.
func (c *LSPClient) sendInitialize() error {
	initReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"processId":    os.Getpid(),
			"rootPath":     ".",
			"capabilities": map[string]any{},
		},
	}

	if err := c.sendMessage(initReq); err != nil {
		return err
	}

	// Wait for initialize response
	if _, err := c.readMessage(); err != nil {
		return err
	}

	// Send initialized notification
	initNotif := map[string]any{
		"jsonrpc": "2.0",
		"method":  "initialized",
		"params":  map[string]any{},
	}

	return c.sendMessage(initNotif)
}

// Analyze runs LSP analysis on a file asynchronously.
func (c *LSPClient) Analyze(ctx context.Context, filePath string, content string) <-chan []Diagnostic {
	resultCh := make(chan []Diagnostic, 1)

	go func() {
		defer close(resultCh)

		// Ensure LSP is running
		if err := c.EnsureStarted(); err != nil {
			resultCh <- nil
			return
		}

		// Detect language
		languageID := detectLanguage(filePath)

		// Send didOpen
		if err := c.sendDidOpen(filePath, languageID, content); err != nil {
			resultCh <- nil
			return
		}

		// Set timeout
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		// Wait for diagnostics
		diags := c.waitForDiagnostics(ctx, filePath)
		resultCh <- diags
	}()

	return resultCh
}

// sendDidOpen sends textDocument/didOpen notification.
func (c *LSPClient) sendDidOpen(uri, languageID, content string) error {
	notif := map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/didOpen",
		"params": map[string]any{
			"textDocument": map[string]any{
				"uri":        uri,
				"languageId": languageID,
				"version":    1,
				"text":       content,
			},
		},
	}

	return c.sendMessage(notif)
}

// sendMessage sends a JSON-RPC message.
func (c *LSPClient) sendMessage(msg map[string]any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, err := c.stdin.Write([]byte(header)); err != nil {
		return err
	}
	if _, err := c.stdin.Write(data); err != nil {
		return err
	}

	return nil
}

// readMessage reads a JSON-RPC message.
func (c *LSPClient) readMessage() (map[string]any, error) {
	c.mu.Lock()
	reader := c.stdout
	c.mu.Unlock()

	// Read header
	scanner := bufio.NewScanner(reader)
	contentLength := 0

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			break // End of headers
		}
		if strings.HasPrefix(line, "Content-Length: ") {
			fmt.Sscanf(line, "Content-Length: %d", &contentLength)
		}
	}

	if contentLength == 0 {
		return nil, fmt.Errorf("no content length")
	}

	// Read body
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(reader, body); err != nil {
		return nil, err
	}

	var msg map[string]any
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, err
	}

	return msg, nil
}

// waitForDiagnostics waits for textDocument/publishDiagnostics notification.
func (c *LSPClient) waitForDiagnostics(ctx context.Context, uri string) []Diagnostic {
	var diags []Diagnostic

	for {
		select {
		case <-ctx.Done():
			return diags // Return what we have on timeout
		default:
		}

		msg, err := c.readMessage()
		if err != nil {
			return diags
		}

		// Check if it's a diagnostics notification
		if method, ok := msg["method"].(string); ok && method == "textDocument/publishDiagnostics" {
			if params, ok := msg["params"].(map[string]any); ok {
				if msgURI, ok := params["uri"].(string); ok && msgURI == uri {
					// Parse diagnostics
					if items, ok := params["diagnostics"].([]any); ok {
						for _, item := range items {
							if diag, ok := item.(map[string]any); ok {
								d := Diagnostic{
									Message: diag["message"].(string),
								}
								if severity, ok := diag["severity"].(float64); ok {
									d.Severity = int(severity)
								}
								if range_, ok := diag["range"].(map[string]any); ok {
									if start, ok := range_["start"].(map[string]any); ok {
										if line, ok := start["line"].(float64); ok {
											d.Line = int(line)
										}
									}
								}
								diags = append(diags, d)
							}
						}
					}
					return diags
				}
			}
		}
	}
}

// Diagnostic represents an LSP diagnostic.
type Diagnostic struct {
	Line     int
	Column   int
	Message  string
	Severity int // 1=Error, 2=Warning, 3=Info, 4=Hint
}

// detectLanguage detects language from file extension.
func detectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "javascript"
	default:
		return ""
	}
}

// Global LSP client instance (shared across tool calls)
var globalLSPClient *LSPClient
var lspOnce sync.Once

// GetLSPClient returns the global LSP client instance.
func GetLSPClient() *LSPClient {
	lspOnce.Do(func() {
		globalLSPClient = NewLSPClient()
	})
	return globalLSPClient
}

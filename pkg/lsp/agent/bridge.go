// Package agent provides LLM-powered analysis for the LSP server.
package agent

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rhystic/attractor/pkg/agent"
	"github.com/rhystic/attractor/pkg/llm"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// Bridge connects the LSP server to the agent system.
type Bridge struct {
	client  *llm.Client
	session *agent.Session
	enabled bool
}

// NewBridge creates a new agent bridge.
func NewBridge(enabled bool) *Bridge {
	if !enabled {
		return &Bridge{enabled: false}
	}

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return &Bridge{enabled: false}
	}

	client := llm.NewClient(apiKey)
	cfg := agent.DefaultConfig()
	cfg.MaxTurns = 1              // Single-turn for LSP
	cfg.MaxToolRoundsPerInput = 0 // No tool calls for LSP queries

	session := agent.NewSession(client, cfg)

	return &Bridge{
		client:  client,
		session: session,
		enabled: true,
	}
}

// IsEnabled returns whether the agent is available.
func (b *Bridge) IsEnabled() bool {
	return b.enabled
}

// ExplainSymbol provides an LLM-powered explanation of a symbol.
func (b *Bridge) ExplainSymbol(ctx context.Context, name, code, language string) (string, error) {
	if !b.enabled {
		return "", fmt.Errorf("agent not enabled")
	}

	prompt := fmt.Sprintf(`Explain what the symbol "%s" does in this %s code:

%s

Provide a brief, clear explanation in 1-2 sentences.`, name, language, truncateCode(code, 50))

	err := b.session.Submit(ctx, prompt)
	if err != nil {
		return "", err
	}

	return b.session.LastResponse(), nil
}

// AnalyzeCode provides LLM-powered code analysis.
func (b *Bridge) AnalyzeCode(ctx context.Context, code, language string) []protocol.Diagnostic {
	if !b.enabled {
		return nil
	}

	// Run analysis in background with timeout
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	prompt := fmt.Sprintf(`Analyze this %s code for potential issues, bugs, or improvements:

%s

List any issues found. If no issues, say "No issues found".`, language, truncateCode(code, 100))

	err := b.session.Submit(ctx, prompt)
	if err != nil {
		// Timeout or error - don't block
		return nil
	}

	response := b.session.LastResponse()
	if strings.Contains(response, "No issues found") {
		return nil
	}

	// Parse response into diagnostics (simplified)
	// In a full implementation, we'd parse line numbers and severities
	return nil // For now, return empty (parsing would be complex)
}

// SuggestCompletion provides intelligent completion suggestions.
func (b *Bridge) SuggestCompletion(ctx context.Context, code, language string, line, character int) []protocol.CompletionItem {
	if !b.enabled {
		return nil
	}

	// Only use agent for complex completions (timeout quickly)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	lines := strings.Split(code, "\n")
	if line >= len(lines) {
		return nil
	}

	currentLine := lines[line][:min(character, len(lines[line]))]

	prompt := fmt.Sprintf(`Complete this %s code at the cursor position (|):

%s
|%s

Provide 1-2 completion suggestions as a simple list.`, language, truncateCode(code, 30), currentLine)

	err := b.session.Submit(ctx, prompt)
	if err != nil {
		return nil
	}

	response := b.session.LastResponse()

	// Parse suggestions (simplified)
	var items []protocol.CompletionItem
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "*") {
			kind := protocol.CompletionItemKindText
			items = append(items, protocol.CompletionItem{
				Label: line,
				Kind:  &kind,
			})
		}
	}

	return items
}

// truncateCode truncates code to maxLines lines.
func truncateCode(code string, maxLines int) string {
	lines := strings.Split(code, "\n")
	if len(lines) <= maxLines {
		return code
	}
	return strings.Join(lines[:maxLines], "\n") + "\n..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

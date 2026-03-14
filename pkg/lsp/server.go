// Package lsp provides the Language Server Protocol implementation.
package lsp

import (
	"context"
	"fmt"
	"os"

	"github.com/rhystic/attractor/pkg/lsp/agent"
	"github.com/rhystic/attractor/pkg/lsp/document"
	"github.com/rhystic/attractor/pkg/lsp/index"
	"github.com/rhystic/attractor/pkg/lsp/parser/symbols"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
	"github.com/tliron/glsp/server"
)

// Config holds server configuration.
type Config struct {
	LogLevel    string
	LogFile     string
	EnableAgent bool
}

// Server wraps the GLSP server with Attractor-specific logic.
type Server struct {
	config      Config
	glsp        *server.Server
	ctx         context.Context
	docs        *document.Manager
	index       *index.Index
	agentBridge *agent.Bridge
}

// NewServer creates a new LSP server instance.
func NewServer(cfg Config) *Server {
	s := &Server{
		config:      cfg,
		ctx:         context.Background(),
		docs:        document.NewManager(),
		index:       index.New(),
		agentBridge: agent.NewBridge(cfg.EnableAgent),
	}

	// Register parsers
	s.docs.RegisterParser("go", symbols.GoParser())
	s.docs.RegisterParser("python", symbols.PythonParser())
	s.docs.RegisterParser("typescript", symbols.TypeScriptParser())

	return s
}

// Run starts the LSP server with stdio transport.
func (s *Server) Run() error {
	// Setup logging
	if s.config.LogFile != "" {
		f, err := os.OpenFile(s.config.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("open log file: %w", err)
		}
		defer f.Close()
		os.Stderr = f
	}

	// Create GLSP server with handler functions
	s.glsp = server.NewServer(&protocol.Handler{
		Initialize:  s.handleInitialize,
		Initialized: s.handleInitialized,
		Shutdown:    s.handleShutdown,
		SetTrace:    s.handleSetTrace,

		// Text document synchronization
		TextDocumentDidOpen:   s.handleTextDocumentDidOpen,
		TextDocumentDidChange: s.handleTextDocumentDidChange,
		TextDocumentDidClose:  s.handleTextDocumentDidClose,

		// Navigation
		TextDocumentDefinition: s.handleTextDocumentDefinition,
		TextDocumentReferences: s.handleTextDocumentReferences,
		TextDocumentHover:      s.handleTextDocumentHover,

		// Document outline
		TextDocumentDocumentSymbol: s.handleTextDocumentDocumentSymbol,
	}, "attractor-lsp", false)

	// Run with stdio transport
	return s.glsp.RunStdio()
}

// handleInitialize is called when the client connects.
func (s *Server) handleInitialize(ctx *glsp.Context, params *protocol.InitializeParams) (any, error) {
	var handler protocol.Handler // empty handler for capabilities

	capabilities := handler.CreateServerCapabilities()

	// Enable features we support
	openClose := true
	change := protocol.TextDocumentSyncKindFull
	capabilities.TextDocumentSync = &protocol.TextDocumentSyncOptions{
		OpenClose: &openClose,
		Change:    &change,
	}

	capabilities.DefinitionProvider = true
	capabilities.ReferencesProvider = true
	capabilities.HoverProvider = true
	capabilities.DocumentSymbolProvider = true

	return protocol.InitializeResult{
		Capabilities: capabilities,
		ServerInfo: &protocol.InitializeResultServerInfo{
			Name:    "attractor-lsp",
			Version: strPtr("0.1.0"),
		},
	}, nil
}

// handleInitialized is called after initialization is complete.
func (s *Server) handleInitialized(ctx *glsp.Context, params *protocol.InitializedParams) error {
	return nil
}

// handleShutdown is called when the client requests shutdown.
func (s *Server) handleShutdown(ctx *glsp.Context) error {
	return nil
}

// handleSetTrace handles trace configuration changes.
func (s *Server) handleSetTrace(ctx *glsp.Context, params *protocol.SetTraceParams) error {
	protocol.SetTraceValue(params.Value)
	return nil
}

// handleTextDocumentDidOpen handles document open.
func (s *Server) handleTextDocumentDidOpen(ctx *glsp.Context, params *protocol.DidOpenTextDocumentParams) error {
	doc := params.TextDocument

	// Detect language from file extension if not provided
	languageID := doc.LanguageID
	if languageID == "" {
		languageID = document.GetLanguageID(doc.URI)
	}

	// Open document (this parses it)
	_, err := s.docs.OpenDocument(doc.URI, languageID, doc.Text, doc.Version)
	if err != nil {
		return fmt.Errorf("open document: %w", err)
	}

	// Extract and index symbols
	s.indexDocument(doc.URI, languageID)

	return nil
}

// handleTextDocumentDidChange handles document changes.
func (s *Server) handleTextDocumentDidChange(ctx *glsp.Context, params *protocol.DidChangeTextDocumentParams) error {
	if len(params.ContentChanges) == 0 {
		return nil
	}

	// Get the new content (full document sync)
	change := params.ContentChanges[0]
	if text, ok := change.(string); ok {
		err := s.docs.UpdateDocument(params.TextDocument.URI, text, params.TextDocument.Version)
		if err != nil {
			return fmt.Errorf("update document: %w", err)
		}

		// Re-index symbols
		doc, _ := s.docs.GetDocument(params.TextDocument.URI)
		if doc != nil {
			s.indexDocument(params.TextDocument.URI, doc.LanguageID)
		}
	}

	return nil
}

// handleTextDocumentDidClose handles document close.
func (s *Server) handleTextDocumentDidClose(ctx *glsp.Context, params *protocol.DidCloseTextDocumentParams) error {
	s.docs.CloseDocument(params.TextDocument.URI)
	s.index.RemoveFile(params.TextDocument.URI)
	return nil
}

// indexDocument extracts and indexes symbols for a document.
func (s *Server) indexDocument(uri, languageID string) {
	doc, ok := s.docs.GetDocument(uri)
	if !ok || doc.Tree == nil {
		return
	}

	var syms []symbols.Symbol
	content := []byte(doc.Content)

	switch languageID {
	case "go":
		syms = symbols.ExtractGo(content, doc.Tree)
	case "python":
		syms = symbols.ExtractPython(content, doc.Tree)
	case "typescript":
		syms = symbols.ExtractTypeScript(content, doc.Tree)
	}

	s.index.UpdateFile(uri, syms)
}

// handleTextDocumentDefinition handles go-to-definition.
func (s *Server) handleTextDocumentDefinition(ctx *glsp.Context, params *protocol.DefinitionParams) (any, error) {
	// Find symbol at position
	sym := s.index.FindSymbolAt(params.TextDocument.URI, params.Position)
	if sym == nil {
		return nil, nil
	}

	// Find definition of that symbol
	locs := s.index.FindDefinition(sym.Name)
	if len(locs) == 0 {
		return nil, nil
	}

	// Convert to LSP locations
	var result []protocol.Location
	for _, loc := range locs {
		result = append(result, protocol.Location{
			URI:   loc.URI,
			Range: loc.Range,
		})
	}

	return result, nil
}

// handleTextDocumentReferences handles find-references.
func (s *Server) handleTextDocumentReferences(ctx *glsp.Context, params *protocol.ReferenceParams) ([]protocol.Location, error) {
	// Find symbol at position
	sym := s.index.FindSymbolAt(params.TextDocument.URI, params.Position)
	if sym == nil {
		return nil, nil
	}

	// Find references
	locs := s.index.FindReferences(sym.Name)

	// Convert to LSP locations
	var result []protocol.Location
	for _, loc := range locs {
		result = append(result, protocol.Location{
			URI:   loc.URI,
			Range: loc.Range,
		})
	}

	return result, nil
}

// handleTextDocumentHover handles hover information.
func (s *Server) handleTextDocumentHover(ctx *glsp.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	// Find symbol at position
	sym := s.index.FindSymbolAt(params.TextDocument.URI, params.Position)
	if sym == nil {
		return nil, nil
	}

	// Build base hover content
	content := fmt.Sprintf("**%s**\n\n%s", sym.Name, sym.Detail)
	if sym.Detail == "" {
		content = sym.Name
	}

	// Add agent explanation if enabled
	if s.agentBridge != nil && s.agentBridge.IsEnabled() {
		doc, ok := s.docs.GetDocument(params.TextDocument.URI)
		if ok {
			explanation, err := s.agentBridge.ExplainSymbol(context.Background(), sym.Name, doc.Content, doc.LanguageID)
			if err == nil && explanation != "" {
				content += "\n\n---\n\n" + explanation
			}
		}
	}

	return &protocol.Hover{
		Contents: protocol.MarkupContent{
			Kind:  protocol.MarkupKindMarkdown,
			Value: content,
		},
		Range: &sym.Range,
	}, nil
}

// handleTextDocumentDocumentSymbol handles document symbols (outline).
func (s *Server) handleTextDocumentDocumentSymbol(ctx *glsp.Context, params *protocol.DocumentSymbolParams) (any, error) {
	syms := s.index.GetDocumentSymbols(params.TextDocument.URI)

	// Convert to LSP DocumentSymbols
	var result []protocol.DocumentSymbol
	for _, sym := range syms {
		result = append(result, sym.ToLSP())
	}

	return result, nil
}

func strPtr(s string) *string {
	return &s
}

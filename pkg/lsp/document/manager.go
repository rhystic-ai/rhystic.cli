// Package document manages open text documents in the LSP server.
package document

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	ts "github.com/smacker/go-tree-sitter"
)

// Document represents an open text document.
type Document struct {
	URI        string
	LanguageID string
	Content    string
	Version    int32
	Tree       *ts.Tree
	mtime      int64
}

// Position converts LSP position to byte offset in the document.
func (d *Document) Position(line, character int) (int, error) {
	lines := strings.Split(d.Content, "\n")
	if line >= len(lines) {
		return 0, fmt.Errorf("line %d out of range", line)
	}

	offset := 0
	for i := 0; i < line; i++ {
		offset += len(lines[i]) + 1 // +1 for newline
	}

	if character > len(lines[line]) {
		character = len(lines[line])
	}

	return offset + character, nil
}

// LineAtPosition returns the text at a given position.
func (d *Document) LineAtPosition(line int) string {
	lines := strings.Split(d.Content, "\n")
	if line < 0 || line >= len(lines) {
		return ""
	}
	return lines[line]
}

// Manager manages open documents.
type Manager struct {
	docs map[string]*Document
	mu   sync.RWMutex

	// Parser for each language
	parsers map[string]*ts.Parser
}

// NewManager creates a new document manager.
func NewManager() *Manager {
	return &Manager{
		docs:    make(map[string]*Document),
		parsers: make(map[string]*ts.Parser),
	}
}

// RegisterParser registers a parser for a language.
func (m *Manager) RegisterParser(languageID string, parser *ts.Parser) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.parsers[languageID] = parser
}

// OpenDocument opens a new document.
func (m *Manager) OpenDocument(uri, languageID, content string, version int32) (*Document, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Create document
	doc := &Document{
		URI:        uri,
		LanguageID: languageID,
		Content:    content,
		Version:    version,
	}

	// Parse if we have a parser
	if parser, ok := m.parsers[languageID]; ok {
		tree, err := parser.ParseCtx(context.Background(), nil, []byte(content))
		if err != nil {
			return nil, fmt.Errorf("parse document: %w", err)
		}
		doc.Tree = tree
	}

	m.docs[uri] = doc
	return doc, nil
}

// GetDocument gets a document by URI.
func (m *Manager) GetDocument(uri string) (*Document, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	doc, ok := m.docs[uri]
	return doc, ok
}

// UpdateDocument updates a document with new content.
func (m *Manager) UpdateDocument(uri string, content string, version int32) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	doc, ok := m.docs[uri]
	if !ok {
		return fmt.Errorf("document not found: %s", uri)
	}

	doc.Content = content
	doc.Version = version

	// Re-parse if we have a parser
	if parser, ok := m.parsers[doc.LanguageID]; ok {
		tree, err := parser.ParseCtx(context.Background(), nil, []byte(content))
		if err != nil {
			return fmt.Errorf("re-parse document: %w", err)
		}
		if doc.Tree != nil {
			doc.Tree.Close()
		}
		doc.Tree = tree
	}

	return nil
}

// CloseDocument closes a document.
func (m *Manager) CloseDocument(uri string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if doc, ok := m.docs[uri]; ok {
		if doc.Tree != nil {
			doc.Tree.Close()
		}
		delete(m.docs, uri)
	}
}

// GetLanguageID detects language from file extension.
func GetLanguageID(path string) string {
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
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".c", ".h":
		return "c"
	case ".cpp", ".hpp", ".cc":
		return "cpp"
	default:
		return ""
	}
}

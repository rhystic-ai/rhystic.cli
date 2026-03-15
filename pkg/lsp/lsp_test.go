// Package lsp_test provides tests for the LSP server.
package lsp_test

import (
	"context"
	"testing"

	"github.com/rhystic/attractor/pkg/lsp/document"
	"github.com/rhystic/attractor/pkg/lsp/index"
	"github.com/rhystic/attractor/pkg/lsp/parser/symbols"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// TestDocumentManager tests document state management.
func TestDocumentManager(t *testing.T) {
	mgr := document.NewManager()

	// Register Go parser
	mgr.RegisterParser("go", symbols.GoParser())

	// Test OpenDocument
	t.Run("OpenDocument", func(t *testing.T) {
		content := `package main
func Hello() string {
	return "Hello"
}`

		doc, err := mgr.OpenDocument("file:///test.go", "go", content, 1)
		if err != nil {
			t.Fatalf("OpenDocument failed: %v", err)
		}

		if doc.URI != "file:///test.go" {
			t.Errorf("Expected URI file:///test.go, got %s", doc.URI)
		}

		if doc.LanguageID != "go" {
			t.Errorf("Expected language go, got %s", doc.LanguageID)
		}

		if doc.Tree == nil {
			t.Error("Expected tree to be parsed")
		}
	})

	// Test GetDocument
	t.Run("GetDocument", func(t *testing.T) {
		doc, ok := mgr.GetDocument("file:///test.go")
		if !ok {
			t.Fatal("Expected document to exist")
		}

		if doc.Version != 1 {
			t.Errorf("Expected version 1, got %d", doc.Version)
		}
	})

	// Test UpdateDocument
	t.Run("UpdateDocument", func(t *testing.T) {
		newContent := `package main
func Hello() string {
	return "Hello, World!"
}`

		err := mgr.UpdateDocument("file:///test.go", newContent, 2)
		if err != nil {
			t.Fatalf("UpdateDocument failed: %v", err)
		}

		doc, _ := mgr.GetDocument("file:///test.go")
		if doc.Version != 2 {
			t.Errorf("Expected version 2, got %d", doc.Version)
		}

		if doc.Content != newContent {
			t.Errorf("Content not updated")
		}
	})

	// Test CloseDocument
	t.Run("CloseDocument", func(t *testing.T) {
		mgr.CloseDocument("file:///test.go")

		_, ok := mgr.GetDocument("file:///test.go")
		if ok {
			t.Error("Expected document to be closed")
		}
	})
}

// TestSymbolExtraction tests symbol extraction for different languages.
func TestSymbolExtraction(t *testing.T) {
	tests := []struct {
		name     string
		language string
		content  string
		expected []struct {
			name string
			kind symbols.Kind
		}
	}{
		{
			name:     "Go function",
			language: "go",
			content: `package main
func Hello() string {
	return "Hello"
}`,
			expected: []struct {
				name string
				kind symbols.Kind
			}{
				{name: "Hello", kind: symbols.KindFunction},
			},
		},
		{
			name:     "Go method",
			language: "go",
			content: `package main
type Greeter struct{}
func (g Greeter) SayHello() string {
	return "Hello"
}`,
			expected: []struct {
				name string
				kind symbols.Kind
			}{
				// Note: Greeter struct may be in a different node type depending on tree-sitter version
				{name: "SayHello", kind: symbols.KindMethod},
			},
		},
		{
			name:     "Python function",
			language: "python",
			content: `def hello():
	return "Hello"`,
			expected: []struct {
				name string
				kind symbols.Kind
			}{
				{name: "hello", kind: symbols.KindFunction},
			},
		},
		{
			name:     "TypeScript class",
			language: "typescript",
			content: `class Greeter {
	greet() {
		return "Hello"
	}
}`,
			expected: []struct {
				name string
				kind symbols.Kind
			}{
				{name: "Greeter", kind: symbols.KindClass},
				{name: "greet", kind: symbols.KindMethod},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var syms []symbols.Symbol

			ctx := context.Background()
			switch tt.language {
			case "go":
				parser := symbols.GoParser()
				tree, err := parser.ParseCtx(ctx, nil, []byte(tt.content))
				if err != nil {
					t.Fatalf("Parse failed: %v", err)
				}
				defer tree.Close()
				syms = symbols.ExtractGo([]byte(tt.content), tree)

			case "python":
				parser := symbols.PythonParser()
				tree, err := parser.ParseCtx(ctx, nil, []byte(tt.content))
				if err != nil {
					t.Fatalf("Parse failed: %v", err)
				}
				defer tree.Close()
				syms = symbols.ExtractPython([]byte(tt.content), tree)

			case "typescript":
				parser := symbols.TypeScriptParser()
				tree, err := parser.ParseCtx(ctx, nil, []byte(tt.content))
				if err != nil {
					t.Fatalf("Parse failed: %v", err)
				}
				defer tree.Close()
				syms = symbols.ExtractTypeScript([]byte(tt.content), tree)
			}

			if len(syms) != len(tt.expected) {
				t.Errorf("Expected %d symbols, got %d", len(tt.expected), len(syms))
				for i, s := range syms {
					t.Logf("  Symbol %d: %s (%v)", i, s.Name, s.Kind)
				}
				return
			}

			for i, exp := range tt.expected {
				if syms[i].Name != exp.name {
					t.Errorf("Expected symbol name %s, got %s", exp.name, syms[i].Name)
				}
				if syms[i].Kind != exp.kind {
					t.Errorf("Expected symbol kind %v, got %v", exp.kind, syms[i].Kind)
				}
			}
		})
	}
}

// TestIndex tests the symbol index.
func TestIndex(t *testing.T) {
	idx := index.New()

	// Test UpdateFile
	t.Run("UpdateFile", func(t *testing.T) {
		syms := []symbols.Symbol{
			{Name: "Hello", Kind: symbols.KindFunction, Range: protocol.Range{
				Start: protocol.Position{Line: 0, Character: 0},
				End:   protocol.Position{Line: 2, Character: 1},
			}},
			{Name: "World", Kind: symbols.KindFunction, Range: protocol.Range{
				Start: protocol.Position{Line: 3, Character: 0},
				End:   protocol.Position{Line: 5, Character: 1},
			}},
		}

		idx.UpdateFile("file:///test.go", syms)

		// Test GetDocumentSymbols
		docSyms := idx.GetDocumentSymbols("file:///test.go")
		if len(docSyms) != 2 {
			t.Errorf("Expected 2 symbols, got %d", len(docSyms))
		}

		// Test FindDefinition
		locs := idx.FindDefinition("Hello")
		if len(locs) != 1 {
			t.Errorf("Expected 1 location for Hello, got %d", len(locs))
		}

		// Test FindSymbolAt
		sym := idx.FindSymbolAt("file:///test.go", protocol.Position{Line: 1, Character: 5})
		if sym == nil {
			t.Error("Expected to find symbol at position")
		} else if sym.Name != "Hello" {
			t.Errorf("Expected symbol Hello, got %s", sym.Name)
		}
	})

	// Test RemoveFile
	t.Run("RemoveFile", func(t *testing.T) {
		idx.RemoveFile("file:///test.go")

		docSyms := idx.GetDocumentSymbols("file:///test.go")
		if len(docSyms) != 0 {
			t.Errorf("Expected 0 symbols after removal, got %d", len(docSyms))
		}

		locs := idx.FindDefinition("Hello")
		if len(locs) != 0 {
			t.Errorf("Expected 0 locations after removal, got %d", len(locs))
		}
	})
}

// TestLanguageDetection tests language ID detection from file paths.
func TestLanguageDetection(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/path/to/file.go", "go"},
		{"/path/to/file.py", "python"},
		{"/path/to/file.ts", "typescript"},
		{"/path/to/file.tsx", "typescript"},
		{"/path/to/file.js", "javascript"},
		{"/path/to/file.jsx", "javascript"},
		{"/path/to/file.rs", "rust"},
		{"/path/to/file.java", "java"},
		{"/path/to/file.c", "c"},
		{"/path/to/file.cpp", "cpp"},
		{"/path/to/file.txt", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := document.GetLanguageID(tt.path)
			if result != tt.expected {
				t.Errorf("GetLanguageID(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

// TestPositionInRange tests position containment in ranges.
func TestPositionInRange(t *testing.T) {
	r := protocol.Range{
		Start: protocol.Position{Line: 1, Character: 0},
		End:   protocol.Position{Line: 3, Character: 10},
	}

	tests := []struct {
		pos      protocol.Position
		expected bool
	}{
		{protocol.Position{Line: 0, Character: 5}, false},  // Before range
		{protocol.Position{Line: 1, Character: 0}, true},   // At start
		{protocol.Position{Line: 2, Character: 5}, true},   // Inside
		{protocol.Position{Line: 3, Character: 10}, true},  // At end
		{protocol.Position{Line: 3, Character: 11}, false}, // After end
		{protocol.Position{Line: 4, Character: 0}, false},  // After range
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := isPositionInRange(tt.pos, r)
			if result != tt.expected {
				t.Errorf("isPositionInRange(%+v, %+v) = %v, want %v",
					tt.pos, r, result, tt.expected)
			}
		})
	}
}

func isPositionInRange(pos protocol.Position, r protocol.Range) bool {
	if pos.Line < r.Start.Line || pos.Line > r.End.Line {
		return false
	}
	if pos.Line == r.Start.Line && pos.Character < r.Start.Character {
		return false
	}
	if pos.Line == r.End.Line && pos.Character > r.End.Character {
		return false
	}
	return true
}

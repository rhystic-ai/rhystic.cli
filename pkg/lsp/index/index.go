// Package index provides symbol indexing for the LSP server.
package index

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/rhystic/attractor/pkg/lsp/parser/symbols"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// Index maintains an in-memory index of symbols across all open documents.
type Index struct {
	mu    sync.RWMutex
	files map[string][]symbols.Symbol // URI -> symbols in that file
	names map[string][]Location       // symbol name -> locations
}

// Location represents where a symbol is defined or referenced.
type Location struct {
	URI   string
	Range protocol.Range
	Kind  symbols.Kind
}

// New creates a new symbol index.
func New() *Index {
	return &Index{
		files: make(map[string][]symbols.Symbol),
		names: make(map[string][]Location),
	}
}

// UpdateFile updates or adds a file's symbols to the index.
func (idx *Index) UpdateFile(uri string, syms []symbols.Symbol) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Remove old symbols for this file from names index
	if oldSyms, ok := idx.files[uri]; ok {
		for _, sym := range oldSyms {
			idx.removeFromNames(sym.Name, uri)
		}
	}

	// Update file symbols
	idx.files[uri] = syms

	// Add new symbols to names index
	for _, sym := range syms {
		idx.names[sym.Name] = append(idx.names[sym.Name], Location{
			URI:   uri,
			Range: sym.Range,
			Kind:  sym.Kind,
		})
	}
}

// RemoveFile removes a file from the index.
func (idx *Index) RemoveFile(uri string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if syms, ok := idx.files[uri]; ok {
		for _, sym := range syms {
			idx.removeFromNames(sym.Name, uri)
		}
		delete(idx.files, uri)
	}
}

// removeFromNames removes all entries for a symbol name in a specific URI.
func (idx *Index) removeFromNames(name, uri string) {
	if locs, ok := idx.names[name]; ok {
		var newLocs []Location
		for _, loc := range locs {
			if loc.URI != uri {
				newLocs = append(newLocs, loc)
			}
		}
		if len(newLocs) == 0 {
			delete(idx.names, name)
		} else {
			idx.names[name] = newLocs
		}
	}
}

// FindDefinition finds the definition of a symbol by name.
func (idx *Index) FindDefinition(name string) []Location {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return idx.names[name]
}

// FindReferences finds all references to a symbol by name.
func (idx *Index) FindReferences(name string) []Location {
	// For now, same as FindDefinition
	// In the future, this would find both definitions and usages
	return idx.FindDefinition(name)
}

// GetDocumentSymbols returns all symbols in a document.
func (idx *Index) GetDocumentSymbols(uri string) []symbols.Symbol {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return idx.files[uri]
}

// FindSymbolAt finds the symbol at a specific position in a file.
func (idx *Index) FindSymbolAt(uri string, pos protocol.Position) *symbols.Symbol {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	for _, sym := range idx.files[uri] {
		if isPositionInRange(pos, sym.Range) {
			return &sym
		}
	}
	return nil
}

// SearchSymbols searches for symbols by name pattern (case-insensitive).
func (idx *Index) SearchSymbols(pattern string) []Location {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	pattern = strings.ToLower(pattern)
	var results []Location

	for name, locs := range idx.names {
		if strings.Contains(strings.ToLower(name), pattern) {
			results = append(results, locs...)
		}
	}

	return results
}

// GetSymbolsByKind returns all symbols of a specific kind.
func (idx *Index) GetSymbolsByKind(kind symbols.Kind) []Location {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var results []Location
	for _, locs := range idx.names {
		for _, loc := range locs {
			if loc.Kind == kind {
				results = append(results, loc)
			}
		}
	}
	return results
}

// isPositionInRange checks if a position is within a range.
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

// URIToPath converts a file URI to a path.
func URIToPath(uri string) string {
	if strings.HasPrefix(uri, "file://") {
		return uri[7:]
	}
	return uri
}

// PathToURI converts a path to a file URI.
func PathToURI(path string) string {
	if strings.HasPrefix(path, "file://") {
		return path
	}
	return "file://" + filepath.ToSlash(path)
}

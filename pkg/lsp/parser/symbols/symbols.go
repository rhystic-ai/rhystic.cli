// Package symbols provides symbol extraction for various languages.
package symbols

import (
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// Kind represents a symbol kind (matches LSP SymbolKind).
type Kind protocol.SymbolKind

const (
	KindFile          = Kind(protocol.SymbolKindFile)
	KindModule        = Kind(protocol.SymbolKindModule)
	KindNamespace     = Kind(protocol.SymbolKindNamespace)
	KindPackage       = Kind(protocol.SymbolKindPackage)
	KindClass         = Kind(protocol.SymbolKindClass)
	KindMethod        = Kind(protocol.SymbolKindMethod)
	KindProperty      = Kind(protocol.SymbolKindProperty)
	KindField         = Kind(protocol.SymbolKindField)
	KindConstructor   = Kind(protocol.SymbolKindConstructor)
	KindEnum          = Kind(protocol.SymbolKindEnum)
	KindInterface     = Kind(protocol.SymbolKindInterface)
	KindFunction      = Kind(protocol.SymbolKindFunction)
	KindVariable      = Kind(protocol.SymbolKindVariable)
	KindConstant      = Kind(protocol.SymbolKindConstant)
	KindString        = Kind(protocol.SymbolKindString)
	KindNumber        = Kind(protocol.SymbolKindNumber)
	KindBoolean       = Kind(protocol.SymbolKindBoolean)
	KindArray         = Kind(protocol.SymbolKindArray)
	KindObject        = Kind(protocol.SymbolKindObject)
	KindKey           = Kind(protocol.SymbolKindKey)
	KindNull          = Kind(protocol.SymbolKindNull)
	KindEnumMember    = Kind(protocol.SymbolKindEnumMember)
	KindStruct        = Kind(protocol.SymbolKindStruct)
	KindEvent         = Kind(protocol.SymbolKindEvent)
	KindOperator      = Kind(protocol.SymbolKindOperator)
	KindTypeParameter = Kind(protocol.SymbolKindTypeParameter)
)

// Symbol represents a symbol extracted from code.
type Symbol struct {
	Name      string
	Kind      Kind
	Range     protocol.Range
	Selection protocol.Range
	Detail    string
	Container string
}

// ToLSP converts our symbol to LSP DocumentSymbol.
func (s Symbol) ToLSP() protocol.DocumentSymbol {
	return protocol.DocumentSymbol{
		Name:           s.Name,
		Detail:         &s.Detail,
		Kind:           protocol.SymbolKind(s.Kind),
		Range:          s.Range,
		SelectionRange: s.Selection,
	}
}

// Location represents a symbol location for references/definitions.
type Location struct {
	URI   string
	Range protocol.Range
}

// Package symbols provides symbol extraction for Go.
package symbols

import (
	"fmt"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// ExtractGo extracts symbols from Go code.
func ExtractGo(content []byte, tree *sitter.Tree) []Symbol {
	var symbols []Symbol

	root := tree.RootNode()

	// Walk the root's children
	for i := 0; i < int(root.ChildCount()); i++ {
		node := root.Child(i)
		if node == nil {
			continue
		}

		// Extract symbol based on node type
		switch node.Type() {
		case "function_declaration":
			if name := getNodeText(node, content, "name"); name != "" {
				symbols = append(symbols, Symbol{
					Name:      name,
					Kind:      KindFunction,
					Range:     nodeToRange(node),
					Selection: getChildRange(node, "name"),
					Detail:    getGoFunctionSignature(node, content),
				})
			}

		case "method_declaration":
			if name := getNodeText(node, content, "name"); name != "" {
				receiver := getGoMethodReceiver(node, content)
				symbols = append(symbols, Symbol{
					Name:      name,
					Kind:      KindMethod,
					Range:     nodeToRange(node),
					Selection: getChildRange(node, "name"),
					Detail:    fmt.Sprintf("(%s) %s", receiver, getGoFunctionSignature(node, content)),
				})
			}

		case "type_declaration":
			if spec := node.ChildByFieldName("spec"); spec != nil {
				if name := getNodeText(spec, content, "name"); name != "" {
					// Check if it's a struct or interface
					kind := KindStruct
					if spec.ChildByFieldName("type") != nil {
						if typeNode := getChildByType(spec, "interface_type"); typeNode != nil {
							kind = KindInterface
						}
					}
					symbols = append(symbols, Symbol{
						Name:      name,
						Kind:      kind,
						Range:     nodeToRange(node),
						Selection: getChildRange(spec, "name"),
					})
				}
			}

		case "var_declaration", "const_declaration":
			if spec := node.ChildByFieldName("spec"); spec != nil {
				if nameList := spec.ChildByFieldName("name"); nameList != nil {
					// Multiple names possible
					for j := 0; j < int(nameList.ChildCount()); j++ {
						if nameNode := nameList.Child(j); nameNode != nil && nameNode.Type() == "identifier" {
							kind := KindVariable
							if node.Type() == "const_declaration" {
								kind = KindConstant
							}
							symbols = append(symbols, Symbol{
								Name:      nameNode.Content(content),
								Kind:      kind,
								Range:     nodeToRange(node),
								Selection: nodeToRange(nameNode),
							})
						}
					}
				}
			}
		}
	}

	return symbols
}

// getNodeText gets text from a child node by field name.
func getNodeText(parent *sitter.Node, content []byte, fieldName string) string {
	if child := parent.ChildByFieldName(fieldName); child != nil {
		return child.Content(content)
	}
	return ""
}

// getChildByType finds a child node by type.
func getChildByType(parent *sitter.Node, typ string) *sitter.Node {
	for i := 0; i < int(parent.ChildCount()); i++ {
		if child := parent.Child(i); child != nil && child.Type() == typ {
			return child
		}
	}
	return nil
}

// getChildRange gets the range of a child node by field name.
func getChildRange(parent *sitter.Node, fieldName string) protocol.Range {
	if child := parent.ChildByFieldName(fieldName); child != nil {
		return nodeToRange(child)
	}
	return nodeToRange(parent)
}

// nodeToRange converts a tree-sitter node to LSP Range.
func nodeToRange(node *sitter.Node) protocol.Range {
	start := node.StartPoint()
	end := node.EndPoint()
	return protocol.Range{
		Start: protocol.Position{
			Line:      start.Row,
			Character: start.Column,
		},
		End: protocol.Position{
			Line:      end.Row,
			Character: end.Column,
		},
	}
}

// getGoFunctionSignature extracts function signature.
func getGoFunctionSignature(node *sitter.Node, content []byte) string {
	if params := node.ChildByFieldName("parameters"); params != nil {
		result := params.Content(content)
		if results := node.ChildByFieldName("result"); results != nil {
			result += " " + results.Content(content)
		}
		return result
	}
	return "()"
}

// getGoMethodReceiver extracts method receiver.
func getGoMethodReceiver(node *sitter.Node, content []byte) string {
	if recv := node.ChildByFieldName("receiver"); recv != nil {
		return recv.Content(content)
	}
	return ""
}

// GoParser returns a tree-sitter parser configured for Go.
func GoParser() *sitter.Parser {
	parser := sitter.NewParser()
	parser.SetLanguage(golang.GetLanguage())
	return parser
}

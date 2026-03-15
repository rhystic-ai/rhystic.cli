// Package symbols provides symbol extraction for Python.
package symbols

import (
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/python"
)

// ExtractPython extracts symbols from Python code.
func ExtractPython(content []byte, tree *sitter.Tree) []Symbol {
	var symbols []Symbol

	root := tree.RootNode()

	// Walk the root's children
	for i := 0; i < int(root.ChildCount()); i++ {
		node := root.Child(i)
		if node == nil {
			continue
		}

		switch node.Type() {
		case "function_definition":
			if name := getNodeText(node, content, "name"); name != "" {
				symbols = append(symbols, Symbol{
					Name:      name,
					Kind:      KindFunction,
					Range:     nodeToRange(node),
					Selection: getChildRange(node, "name"),
					Detail:    getPythonFunctionSignature(node, content),
				})
			}

		case "class_definition":
			if name := getNodeText(node, content, "name"); name != "" {
				symbols = append(symbols, Symbol{
					Name:      name,
					Kind:      KindClass,
					Range:     nodeToRange(node),
					Selection: getChildRange(node, "name"),
				})

				// Extract methods inside class
				if body := node.ChildByFieldName("body"); body != nil {
					for j := 0; j < int(body.ChildCount()); j++ {
						if child := body.Child(j); child != nil && child.Type() == "function_definition" {
							if methodName := getNodeText(child, content, "name"); methodName != "" {
								symbols = append(symbols, Symbol{
									Name:      methodName,
									Kind:      KindMethod,
									Range:     nodeToRange(child),
									Selection: getChildRange(child, "name"),
									Container: name,
									Detail:    getPythonFunctionSignature(child, content),
								})
							}
						}
					}
				}
			}

		case "assignment":
			if left := node.ChildByFieldName("left"); left != nil {
				if left.Type() == "identifier" {
					name := left.Content(content)
					symbols = append(symbols, Symbol{
						Name:      name,
						Kind:      KindVariable,
						Range:     nodeToRange(node),
						Selection: nodeToRange(left),
					})
				}
			}
		}
	}

	return symbols
}

// getPythonFunctionSignature extracts function signature.
func getPythonFunctionSignature(node *sitter.Node, content []byte) string {
	if params := node.ChildByFieldName("parameters"); params != nil {
		return params.Content(content)
	}
	return "()"
}

// PythonParser returns a tree-sitter parser configured for Python.
func PythonParser() *sitter.Parser {
	parser := sitter.NewParser()
	parser.SetLanguage(python.GetLanguage())
	return parser
}

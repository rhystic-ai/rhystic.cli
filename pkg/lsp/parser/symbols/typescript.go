// Package symbols provides symbol extraction for TypeScript/JavaScript.
package symbols

import (
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

// ExtractTypeScript extracts symbols from TypeScript/JavaScript code.
func ExtractTypeScript(content []byte, tree *sitter.Tree) []Symbol {
	var symbols []Symbol

	root := tree.RootNode()

	// Walk the root's children
	for i := 0; i < int(root.ChildCount()); i++ {
		node := root.Child(i)
		if node == nil {
			continue
		}

		switch node.Type() {
		case "function_declaration":
			if name := getNodeText(node, content, "name"); name != "" {
				symbols = append(symbols, Symbol{
					Name:      name,
					Kind:      KindFunction,
					Range:     nodeToRange(node),
					Selection: getChildRange(node, "name"),
					Detail:    getTSFunctionSignature(node, content),
				})
			}

		case "class_declaration":
			if name := getNodeText(node, content, "name"); name != "" {
				symbols = append(symbols, Symbol{
					Name:      name,
					Kind:      KindClass,
					Range:     nodeToRange(node),
					Selection: getChildRange(node, "name"),
				})

				// Extract methods and properties
				if body := node.ChildByFieldName("body"); body != nil {
					for j := 0; j < int(body.ChildCount()); j++ {
						if child := body.Child(j); child != nil {
							switch child.Type() {
							case "method_definition":
								if methodName := getNodeText(child, content, "name"); methodName != "" {
									symbols = append(symbols, Symbol{
										Name:      methodName,
										Kind:      KindMethod,
										Range:     nodeToRange(child),
										Selection: getChildRange(child, "name"),
										Container: name,
										Detail:    getTSFunctionSignature(child, content),
									})
								}
							case "property_definition":
								if propName := getNodeText(child, content, "name"); propName != "" {
									symbols = append(symbols, Symbol{
										Name:      propName,
										Kind:      KindProperty,
										Range:     nodeToRange(child),
										Selection: getChildRange(child, "name"),
										Container: name,
									})
								}
							}
						}
					}
				}
			}

		case "interface_declaration":
			if name := getNodeText(node, content, "name"); name != "" {
				symbols = append(symbols, Symbol{
					Name:      name,
					Kind:      KindInterface,
					Range:     nodeToRange(node),
					Selection: getChildRange(node, "name"),
				})

				// Extract interface members
				if body := node.ChildByFieldName("body"); body != nil {
					for j := 0; j < int(body.ChildCount()); j++ {
						if child := body.Child(j); child != nil && child.Type() == "property_signature" {
							if propName := getNodeText(child, content, "name"); propName != "" {
								symbols = append(symbols, Symbol{
									Name:      propName,
									Kind:      KindProperty,
									Range:     nodeToRange(child),
									Selection: getChildRange(child, "name"),
									Container: name,
								})
							}
						}
					}
				}
			}

		case "type_alias_declaration":
			if name := getNodeText(node, content, "name"); name != "" {
				symbols = append(symbols, Symbol{
					Name:      name,
					Kind:      KindTypeParameter,
					Range:     nodeToRange(node),
					Selection: getChildRange(node, "name"),
				})
			}

		case "lexical_declaration", "variable_declaration":
			// const/let/var declarations
			if declList := node.ChildByFieldName("declaration"); declList != nil {
				for j := 0; j < int(declList.ChildCount()); j++ {
					if child := declList.Child(j); child != nil && child.Type() == "variable_declarator" {
						if name := getNodeText(child, content, "name"); name != "" {
							symbols = append(symbols, Symbol{
								Name:      name,
								Kind:      KindVariable,
								Range:     nodeToRange(node),
								Selection: getChildRange(child, "name"),
							})
						}
					}
				}
			}
		}
	}

	return symbols
}

// getTSFunctionSignature extracts function signature.
func getTSFunctionSignature(node *sitter.Node, content []byte) string {
	if params := node.ChildByFieldName("parameters"); params != nil {
		return params.Content(content)
	}
	return "()"
}

// TypeScriptParser returns a tree-sitter parser configured for TypeScript.
func TypeScriptParser() *sitter.Parser {
	parser := sitter.NewParser()
	parser.SetLanguage(typescript.GetLanguage())
	return parser
}

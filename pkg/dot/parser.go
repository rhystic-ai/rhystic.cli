package dot

import (
	"fmt"
	"regexp"
	"strings"
)

// Parser parses DOT graph definitions.
type Parser struct {
	input  string
	pos    int
	tokens []token
	tokIdx int
}

type tokenType int

const (
	tokEOF tokenType = iota
	tokIdent
	tokString
	tokNumber
	tokDigraph
	tokGraph
	tokNode
	tokEdge
	tokSubgraph
	tokLBrace
	tokRBrace
	tokLBracket
	tokRBracket
	tokEquals
	tokComma
	tokSemicolon
	tokArrow
)

type token struct {
	typ   tokenType
	value string
	line  int
	col   int
}

// Parse parses a DOT string into a Graph.
func Parse(input string) (*Graph, error) {
	p := &Parser{input: input}
	if err := p.tokenize(); err != nil {
		return nil, err
	}
	return p.parse()
}

// ParseFile parses a DOT file.
func ParseFile(filename string) (*Graph, error) {
	// This would read the file - for now just return an error
	return nil, fmt.Errorf("ParseFile not implemented; use Parse with file contents")
}

func (p *Parser) tokenize() error {
	p.input = stripComments(p.input)

	line, col := 1, 1
	i := 0
	for i < len(p.input) {
		c := p.input[i]

		// Track line and column
		if c == '\n' {
			line++
			col = 1
			i++
			continue
		}

		// Skip whitespace
		if c == ' ' || c == '\t' || c == '\r' {
			col++
			i++
			continue
		}

		// String literal
		if c == '"' {
			start := i
			i++
			col++
			for i < len(p.input) && p.input[i] != '"' {
				if p.input[i] == '\\' && i+1 < len(p.input) {
					i += 2
					col += 2
				} else {
					if p.input[i] == '\n' {
						line++
						col = 1
					} else {
						col++
					}
					i++
				}
			}
			if i >= len(p.input) {
				return fmt.Errorf("unterminated string at line %d", line)
			}
			i++ // closing quote
			value := p.input[start+1 : i-1]
			value = unescapeString(value)
			p.tokens = append(p.tokens, token{tokString, value, line, col})
			continue
		}

		// Arrow
		if i+1 < len(p.input) && p.input[i:i+2] == "->" {
			p.tokens = append(p.tokens, token{tokArrow, "->", line, col})
			i += 2
			col += 2
			continue
		}

		// Single character tokens
		switch c {
		case '{':
			p.tokens = append(p.tokens, token{tokLBrace, "{", line, col})
			i++
			col++
			continue
		case '}':
			p.tokens = append(p.tokens, token{tokRBrace, "}", line, col})
			i++
			col++
			continue
		case '[':
			p.tokens = append(p.tokens, token{tokLBracket, "[", line, col})
			i++
			col++
			continue
		case ']':
			p.tokens = append(p.tokens, token{tokRBracket, "]", line, col})
			i++
			col++
			continue
		case '=':
			p.tokens = append(p.tokens, token{tokEquals, "=", line, col})
			i++
			col++
			continue
		case ',':
			p.tokens = append(p.tokens, token{tokComma, ",", line, col})
			i++
			col++
			continue
		case ';':
			p.tokens = append(p.tokens, token{tokSemicolon, ";", line, col})
			i++
			col++
			continue
		}

		// Identifier or keyword
		if isIdentStart(c) {
			start := i
			for i < len(p.input) && isIdentChar(p.input[i]) {
				i++
				col++
			}
			value := p.input[start:i]
			typ := tokIdent
			switch strings.ToLower(value) {
			case "digraph":
				typ = tokDigraph
			case "graph":
				typ = tokGraph
			case "node":
				typ = tokNode
			case "edge":
				typ = tokEdge
			case "subgraph":
				typ = tokSubgraph
			}
			p.tokens = append(p.tokens, token{typ, value, line, col})
			continue
		}

		// Number (including negative)
		if c == '-' || (c >= '0' && c <= '9') {
			start := i
			if c == '-' {
				i++
				col++
			}
			for i < len(p.input) && (p.input[i] >= '0' && p.input[i] <= '9') {
				i++
				col++
			}
			if i < len(p.input) && p.input[i] == '.' {
				i++
				col++
				for i < len(p.input) && (p.input[i] >= '0' && p.input[i] <= '9') {
					i++
					col++
				}
			}
			p.tokens = append(p.tokens, token{tokNumber, p.input[start:i], line, col})
			continue
		}

		return fmt.Errorf("unexpected character '%c' at line %d, col %d", c, line, col)
	}

	p.tokens = append(p.tokens, token{tokEOF, "", line, col})
	return nil
}

func stripComments(input string) string {
	// Remove // line comments
	re1 := regexp.MustCompile(`//[^\n]*`)
	input = re1.ReplaceAllString(input, "")

	// Remove /* block */ comments
	re2 := regexp.MustCompile(`/\*[\s\S]*?\*/`)
	input = re2.ReplaceAllString(input, "")

	return input
}

func unescapeString(s string) string {
	s = strings.ReplaceAll(s, `\\`, "\x00") // temp placeholder
	s = strings.ReplaceAll(s, `\"`, `"`)
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\t`, "\t")
	s = strings.ReplaceAll(s, "\x00", `\`)
	return s
}

func isIdentStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

func isIdentChar(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

func (p *Parser) peek() token {
	if p.tokIdx >= len(p.tokens) {
		return token{tokEOF, "", 0, 0}
	}
	return p.tokens[p.tokIdx]
}

func (p *Parser) advance() token {
	tok := p.peek()
	p.tokIdx++
	return tok
}

func (p *Parser) expect(typ tokenType) (token, error) {
	tok := p.advance()
	if tok.typ != typ {
		return tok, fmt.Errorf("expected %v, got %v (%q) at line %d", typ, tok.typ, tok.value, tok.line)
	}
	return tok, nil
}

func (p *Parser) parse() (*Graph, error) {
	// Expect: digraph NAME { ... }
	if _, err := p.expect(tokDigraph); err != nil {
		return nil, fmt.Errorf("expected 'digraph': %w", err)
	}

	nameTok := p.advance()
	if nameTok.typ != tokIdent && nameTok.typ != tokString {
		return nil, fmt.Errorf("expected graph name, got %v", nameTok.typ)
	}

	if _, err := p.expect(tokLBrace); err != nil {
		return nil, err
	}

	graph := &Graph{
		Name:       nameTok.value,
		Attributes: make(map[string]string),
		Nodes:      make(map[string]*Node),
	}

	// Parse statements until closing brace
	nodeDefaults := make(map[string]string)
	edgeDefaults := make(map[string]string)

	for p.peek().typ != tokRBrace && p.peek().typ != tokEOF {
		if err := p.parseStatement(graph, nodeDefaults, edgeDefaults, ""); err != nil {
			return nil, err
		}
	}

	if _, err := p.expect(tokRBrace); err != nil {
		return nil, err
	}

	return graph, nil
}

func (p *Parser) parseStatement(graph *Graph, nodeDefaults, edgeDefaults map[string]string, subgraphName string) error {
	tok := p.peek()

	switch tok.typ {
	case tokGraph:
		// graph [attrs]
		p.advance()
		if p.peek().typ == tokLBracket {
			attrs, err := p.parseAttrBlock()
			if err != nil {
				return err
			}
			for k, v := range attrs {
				graph.Attributes[k] = v
			}
		}
		p.skipSemicolon()
		return nil

	case tokNode:
		// node [defaults]
		p.advance()
		if p.peek().typ == tokLBracket {
			attrs, err := p.parseAttrBlock()
			if err != nil {
				return err
			}
			for k, v := range attrs {
				nodeDefaults[k] = v
			}
		}
		p.skipSemicolon()
		return nil

	case tokEdge:
		// edge [defaults]
		p.advance()
		if p.peek().typ == tokLBracket {
			attrs, err := p.parseAttrBlock()
			if err != nil {
				return err
			}
			for k, v := range attrs {
				edgeDefaults[k] = v
			}
		}
		p.skipSemicolon()
		return nil

	case tokSubgraph:
		return p.parseSubgraph(graph, nodeDefaults, edgeDefaults)

	case tokIdent, tokString:
		return p.parseNodeOrEdge(graph, nodeDefaults, edgeDefaults, subgraphName)

	case tokSemicolon:
		p.advance()
		return nil

	default:
		return fmt.Errorf("unexpected token %v (%q) at line %d", tok.typ, tok.value, tok.line)
	}
}

func (p *Parser) parseSubgraph(graph *Graph, parentNodeDefaults, parentEdgeDefaults map[string]string) error {
	p.advance() // consume 'subgraph'

	var name string
	if p.peek().typ == tokIdent || p.peek().typ == tokString {
		name = p.advance().value
	}

	if _, err := p.expect(tokLBrace); err != nil {
		return err
	}

	// Create subgraph with inherited defaults
	sg := &Subgraph{
		Name:         name,
		NodeDefaults: make(map[string]string),
		EdgeDefaults: make(map[string]string),
	}
	for k, v := range parentNodeDefaults {
		sg.NodeDefaults[k] = v
	}
	for k, v := range parentEdgeDefaults {
		sg.EdgeDefaults[k] = v
	}

	// Parse subgraph statements
	for p.peek().typ != tokRBrace && p.peek().typ != tokEOF {
		tok := p.peek()

		switch tok.typ {
		case tokGraph:
			// Subgraph attributes
			p.advance()
			if p.peek().typ == tokLBracket {
				attrs, err := p.parseAttrBlock()
				if err != nil {
					return err
				}
				if label, ok := attrs["label"]; ok {
					sg.Label = label
					sg.DerivedClasses = append(sg.DerivedClasses, DeriveClass(label))
				}
			}
			p.skipSemicolon()

		case tokNode:
			p.advance()
			if p.peek().typ == tokLBracket {
				attrs, err := p.parseAttrBlock()
				if err != nil {
					return err
				}
				for k, v := range attrs {
					sg.NodeDefaults[k] = v
				}
			}
			p.skipSemicolon()

		case tokEdge:
			p.advance()
			if p.peek().typ == tokLBracket {
				attrs, err := p.parseAttrBlock()
				if err != nil {
					return err
				}
				for k, v := range attrs {
					sg.EdgeDefaults[k] = v
				}
			}
			p.skipSemicolon()

		case tokIdent, tokString:
			// Parse node or edge within subgraph
			if err := p.parseNodeOrEdge(graph, sg.NodeDefaults, sg.EdgeDefaults, name); err != nil {
				return err
			}

		case tokSemicolon:
			p.advance()

		default:
			return fmt.Errorf("unexpected token in subgraph: %v", tok.typ)
		}
	}

	if _, err := p.expect(tokRBrace); err != nil {
		return err
	}

	graph.Subgraphs = append(graph.Subgraphs, sg)
	return nil
}

func (p *Parser) parseNodeOrEdge(graph *Graph, nodeDefaults, edgeDefaults map[string]string, subgraphName string) error {
	// Get the first identifier
	nameTok := p.advance()
	name := nameTok.value

	// Check if this is a graph attribute assignment (name = value at top level)
	if p.peek().typ == tokEquals {
		p.advance() // consume '='
		valueTok := p.advance()
		graph.Attributes[name] = valueTok.value
		p.skipSemicolon()
		return nil
	}

	// Check if this is an edge (has ->)
	if p.peek().typ == tokArrow {
		return p.parseEdgeChain(graph, edgeDefaults, name)
	}

	// It's a node definition
	node := &Node{
		ID:         name,
		Attributes: make(map[string]string),
		Subgraph:   subgraphName,
	}

	// Apply defaults
	for k, v := range nodeDefaults {
		node.Attributes[k] = v
	}

	// Parse attributes if present
	if p.peek().typ == tokLBracket {
		attrs, err := p.parseAttrBlock()
		if err != nil {
			return err
		}
		for k, v := range attrs {
			node.Attributes[k] = v
		}
	}

	graph.Nodes[name] = node
	p.skipSemicolon()
	return nil
}

func (p *Parser) parseEdgeChain(graph *Graph, edgeDefaults map[string]string, firstNode string) error {
	nodes := []string{firstNode}

	// Collect all nodes in the chain
	for p.peek().typ == tokArrow {
		p.advance() // consume '->'
		tok := p.advance()
		if tok.typ != tokIdent && tok.typ != tokString {
			return fmt.Errorf("expected node name after ->, got %v", tok.typ)
		}
		nodes = append(nodes, tok.value)
	}

	// Parse optional attributes
	var attrs map[string]string
	if p.peek().typ == tokLBracket {
		var err error
		attrs, err = p.parseAttrBlock()
		if err != nil {
			return err
		}
	}

	// Create edges for each pair
	for i := 0; i < len(nodes)-1; i++ {
		edge := &Edge{
			From:       nodes[i],
			To:         nodes[i+1],
			Attributes: make(map[string]string),
		}

		// Apply defaults
		for k, v := range edgeDefaults {
			edge.Attributes[k] = v
		}

		// Apply explicit attributes
		for k, v := range attrs {
			edge.Attributes[k] = v
		}

		graph.Edges = append(graph.Edges, edge)

		// Ensure source and target nodes exist
		if _, ok := graph.Nodes[edge.From]; !ok {
			graph.Nodes[edge.From] = &Node{
				ID:         edge.From,
				Attributes: make(map[string]string),
			}
		}
		if _, ok := graph.Nodes[edge.To]; !ok {
			graph.Nodes[edge.To] = &Node{
				ID:         edge.To,
				Attributes: make(map[string]string),
			}
		}
	}

	p.skipSemicolon()
	return nil
}

func (p *Parser) parseAttrBlock() (map[string]string, error) {
	if _, err := p.expect(tokLBracket); err != nil {
		return nil, err
	}

	attrs := make(map[string]string)

	for p.peek().typ != tokRBracket && p.peek().typ != tokEOF {
		// Parse key
		keyTok := p.advance()
		if keyTok.typ != tokIdent && keyTok.typ != tokString {
			return nil, fmt.Errorf("expected attribute key, got %v", keyTok.typ)
		}
		key := keyTok.value

		// Expect '='
		if _, err := p.expect(tokEquals); err != nil {
			return nil, err
		}

		// Parse value
		valueTok := p.advance()
		var value string
		switch valueTok.typ {
		case tokString:
			value = valueTok.value
		case tokIdent:
			value = valueTok.value
		case tokNumber:
			value = valueTok.value
		default:
			return nil, fmt.Errorf("expected attribute value, got %v", valueTok.typ)
		}

		attrs[key] = value

		// Skip comma or semicolon between attributes
		if p.peek().typ == tokComma || p.peek().typ == tokSemicolon {
			p.advance()
		}
	}

	if _, err := p.expect(tokRBracket); err != nil {
		return nil, err
	}

	return attrs, nil
}

func (p *Parser) skipSemicolon() {
	if p.peek().typ == tokSemicolon {
		p.advance()
	}
}

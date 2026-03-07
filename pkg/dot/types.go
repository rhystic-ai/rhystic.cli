// Package dot provides a DOT graph parser for Attractor pipeline definitions.
package dot

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Graph represents a parsed DOT digraph.
type Graph struct {
	Name       string            `json:"name"`
	Attributes map[string]string `json:"attributes"`
	Nodes      map[string]*Node  `json:"nodes"`
	Edges      []*Edge           `json:"edges"`
	Subgraphs  []*Subgraph       `json:"subgraphs,omitempty"`
}

// Node represents a node in the graph.
type Node struct {
	ID         string            `json:"id"`
	Attributes map[string]string `json:"attributes"`
	Subgraph   string            `json:"subgraph,omitempty"` // Parent subgraph name
}

// Edge represents a directed edge between nodes.
type Edge struct {
	From       string            `json:"from"`
	To         string            `json:"to"`
	Attributes map[string]string `json:"attributes"`
}

// Subgraph represents a subgraph (cluster) in the DOT file.
type Subgraph struct {
	Name           string            `json:"name"`
	Label          string            `json:"label,omitempty"`
	NodeDefaults   map[string]string `json:"node_defaults,omitempty"`
	EdgeDefaults   map[string]string `json:"edge_defaults,omitempty"`
	DerivedClasses []string          `json:"derived_classes,omitempty"`
}

// Node attribute accessors

// Label returns the display label for the node.
func (n *Node) Label() string {
	if label, ok := n.Attributes["label"]; ok {
		return label
	}
	return n.ID
}

// Shape returns the node shape, defaulting to "box".
func (n *Node) Shape() string {
	if shape, ok := n.Attributes["shape"]; ok {
		return shape
	}
	return "box"
}

// Type returns the explicit handler type, if set.
func (n *Node) Type() string {
	return n.Attributes["type"]
}

// Prompt returns the prompt attribute.
func (n *Node) Prompt() string {
	return n.Attributes["prompt"]
}

// MaxRetries returns the max_retries attribute.
func (n *Node) MaxRetries() int {
	if v, ok := n.Attributes["max_retries"]; ok {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return 0
}

// GoalGate returns whether this node is a goal gate.
func (n *Node) GoalGate() bool {
	return n.Attributes["goal_gate"] == "true"
}

// RetryTarget returns the retry target node ID.
func (n *Node) RetryTarget() string {
	return n.Attributes["retry_target"]
}

// FallbackRetryTarget returns the fallback retry target node ID.
func (n *Node) FallbackRetryTarget() string {
	return n.Attributes["fallback_retry_target"]
}

// Timeout returns the timeout duration, or 0 if not set.
func (n *Node) Timeout() time.Duration {
	if v, ok := n.Attributes["timeout"]; ok {
		return ParseDuration(v)
	}
	return 0
}

// Fidelity returns the context fidelity mode.
func (n *Node) Fidelity() string {
	return n.Attributes["fidelity"]
}

// ThreadID returns the explicit thread identifier.
func (n *Node) ThreadID() string {
	return n.Attributes["thread_id"]
}

// Class returns the comma-separated class names.
func (n *Node) Class() []string {
	if c, ok := n.Attributes["class"]; ok {
		return strings.Split(c, ",")
	}
	return nil
}

// LLMModel returns the LLM model identifier.
func (n *Node) LLMModel() string {
	return n.Attributes["llm_model"]
}

// LLMProvider returns the LLM provider key.
func (n *Node) LLMProvider() string {
	return n.Attributes["llm_provider"]
}

// ReasoningEffort returns the reasoning effort level.
func (n *Node) ReasoningEffort() string {
	if v, ok := n.Attributes["reasoning_effort"]; ok {
		return v
	}
	return "high"
}

// AutoStatus returns whether to auto-generate status on completion.
func (n *Node) AutoStatus() bool {
	return n.Attributes["auto_status"] == "true"
}

// AllowPartial returns whether to accept partial success on retry exhaustion.
func (n *Node) AllowPartial() bool {
	return n.Attributes["allow_partial"] == "true"
}

// Edge attribute accessors

// Label returns the edge label.
func (e *Edge) Label() string {
	return e.Attributes["label"]
}

// Condition returns the condition expression.
func (e *Edge) Condition() string {
	return e.Attributes["condition"]
}

// Weight returns the edge weight, defaulting to 0.
func (e *Edge) Weight() int {
	if v, ok := e.Attributes["weight"]; ok {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return 0
}

// Fidelity returns the override fidelity mode for the target node.
func (e *Edge) Fidelity() string {
	return e.Attributes["fidelity"]
}

// ThreadID returns the override thread ID for the target node.
func (e *Edge) ThreadID() string {
	return e.Attributes["thread_id"]
}

// LoopRestart returns whether this edge triggers a loop restart.
func (e *Edge) LoopRestart() bool {
	return e.Attributes["loop_restart"] == "true"
}

// Graph attribute accessors

// Goal returns the pipeline goal.
func (g *Graph) Goal() string {
	return g.Attributes["goal"]
}

// ModelStylesheet returns the model stylesheet content.
func (g *Graph) ModelStylesheet() string {
	return g.Attributes["model_stylesheet"]
}

// DefaultMaxRetry returns the default max retry count.
func (g *Graph) DefaultMaxRetry() int {
	if v, ok := g.Attributes["default_max_retry"]; ok {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return 50
}

// RetryTarget returns the graph-level retry target.
func (g *Graph) RetryTarget() string {
	return g.Attributes["retry_target"]
}

// FallbackRetryTarget returns the graph-level fallback retry target.
func (g *Graph) FallbackRetryTarget() string {
	return g.Attributes["fallback_retry_target"]
}

// DefaultFidelity returns the default context fidelity mode.
func (g *Graph) DefaultFidelity() string {
	return g.Attributes["default_fidelity"]
}

// Graph utility methods

// OutgoingEdges returns all edges leaving the given node.
func (g *Graph) OutgoingEdges(nodeID string) []*Edge {
	var edges []*Edge
	for _, e := range g.Edges {
		if e.From == nodeID {
			edges = append(edges, e)
		}
	}
	return edges
}

// IncomingEdges returns all edges entering the given node.
func (g *Graph) IncomingEdges(nodeID string) []*Edge {
	var edges []*Edge
	for _, e := range g.Edges {
		if e.To == nodeID {
			edges = append(edges, e)
		}
	}
	return edges
}

// FindStartNode finds the start node (shape=Mdiamond or id="start").
func (g *Graph) FindStartNode() *Node {
	// First, look for Mdiamond shape
	for _, node := range g.Nodes {
		if node.Shape() == "Mdiamond" {
			return node
		}
	}
	// Fallback to id="start" or "Start"
	if node, ok := g.Nodes["start"]; ok {
		return node
	}
	if node, ok := g.Nodes["Start"]; ok {
		return node
	}
	return nil
}

// FindExitNode finds the exit node (shape=Msquare or id="exit").
func (g *Graph) FindExitNode() *Node {
	// First, look for Msquare shape
	for _, node := range g.Nodes {
		if node.Shape() == "Msquare" {
			return node
		}
	}
	// Fallback to id="exit" or "Exit"
	if node, ok := g.Nodes["exit"]; ok {
		return node
	}
	if node, ok := g.Nodes["Exit"]; ok {
		return node
	}
	return nil
}

// IsTerminal returns true if the node is a terminal (exit) node.
func (g *Graph) IsTerminal(nodeID string) bool {
	node, ok := g.Nodes[nodeID]
	if !ok {
		return false
	}
	return node.Shape() == "Msquare"
}

// ParseDuration parses a duration string with units (ms, s, m, h, d).
func ParseDuration(s string) time.Duration {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	// Remove quotes if present
	s = strings.Trim(s, "\"'")

	// Try standard Go duration parsing first
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}

	// Parse custom formats: 10s, 15m, 2h, 250ms, 1d
	re := regexp.MustCompile(`^(-?\d+(?:\.\d+)?)(ms|s|m|h|d)$`)
	matches := re.FindStringSubmatch(s)
	if len(matches) != 3 {
		return 0
	}

	value, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0
	}

	unit := matches[2]
	switch unit {
	case "ms":
		return time.Duration(value * float64(time.Millisecond))
	case "s":
		return time.Duration(value * float64(time.Second))
	case "m":
		return time.Duration(value * float64(time.Minute))
	case "h":
		return time.Duration(value * float64(time.Hour))
	case "d":
		return time.Duration(value * 24 * float64(time.Hour))
	}

	return 0
}

// DeriveClass derives a CSS-like class name from a label.
func DeriveClass(label string) string {
	// Lowercase, replace spaces with hyphens, strip non-alphanumeric (except hyphens)
	label = strings.ToLower(label)
	label = strings.ReplaceAll(label, " ", "-")
	re := regexp.MustCompile(`[^a-z0-9-]`)
	return re.ReplaceAllString(label, "")
}

// Package context provides state and context management for pipeline execution.
package context

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Context holds the shared state for a pipeline run.
type Context struct {
	mu     sync.RWMutex
	values map[string]any
}

// New creates a new empty context.
func New() *Context {
	return &Context{
		values: make(map[string]any),
	}
}

// Get retrieves a value from the context.
func (c *Context) Get(key string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.values[key]
	return v, ok
}

// GetString retrieves a string value from the context.
func (c *Context) GetString(key string) string {
	v, ok := c.Get(key)
	if !ok {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// GetInt retrieves an integer value from the context.
func (c *Context) GetInt(key string) int {
	v, ok := c.Get(key)
	if !ok {
		return 0
	}
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	default:
		return 0
	}
}

// GetBool retrieves a boolean value from the context.
func (c *Context) GetBool(key string) bool {
	v, ok := c.Get(key)
	if !ok {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

// Set stores a value in the context.
func (c *Context) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values[key] = value
}

// Delete removes a value from the context.
func (c *Context) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.values, key)
}

// Clone creates a deep copy of the context.
func (c *Context) Clone() *Context {
	c.mu.RLock()
	defer c.mu.RUnlock()

	clone := New()
	for k, v := range c.values {
		clone.values[k] = v
	}
	return clone
}

// Merge merges another context's values into this one.
func (c *Context) Merge(other *Context) {
	if other == nil {
		return
	}
	other.mu.RLock()
	defer other.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	for k, v := range other.values {
		c.values[k] = v
	}
}

// All returns all values in the context.
func (c *Context) All() map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]any)
	for k, v := range c.values {
		result[k] = v
	}
	return result
}

// MarshalJSON implements json.Marshaler.
func (c *Context) MarshalJSON() ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return json.Marshal(c.values)
}

// UnmarshalJSON implements json.Unmarshaler.
func (c *Context) UnmarshalJSON(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return json.Unmarshal(data, &c.values)
}

// OutcomeStatus represents the status of a node execution.
type OutcomeStatus string

const (
	StatusSuccess        OutcomeStatus = "success"
	StatusPartialSuccess OutcomeStatus = "partial_success"
	StatusFail           OutcomeStatus = "fail"
	StatusRetry          OutcomeStatus = "retry"
)

// Outcome represents the result of executing a node.
type Outcome struct {
	Status         OutcomeStatus  `json:"status"`
	Notes          string         `json:"notes,omitempty"`
	FailureReason  string         `json:"failure_reason,omitempty"`
	ContextUpdates map[string]any `json:"context_updates,omitempty"`
	PreferredLabel string         `json:"preferred_label,omitempty"`
	SuggestedNexts []string       `json:"suggested_next_ids,omitempty"`
}

// NewSuccessOutcome creates a successful outcome.
func NewSuccessOutcome(notes string) Outcome {
	return Outcome{
		Status: StatusSuccess,
		Notes:  notes,
	}
}

// NewFailOutcome creates a failure outcome.
func NewFailOutcome(reason string) Outcome {
	return Outcome{
		Status:        StatusFail,
		FailureReason: reason,
	}
}

// NewRetryOutcome creates a retry outcome.
func NewRetryOutcome(reason string) Outcome {
	return Outcome{
		Status:        StatusRetry,
		FailureReason: reason,
	}
}

// WithContextUpdate adds a context update to the outcome.
func (o Outcome) WithContextUpdate(key string, value any) Outcome {
	if o.ContextUpdates == nil {
		o.ContextUpdates = make(map[string]any)
	}
	o.ContextUpdates[key] = value
	return o
}

// WithPreferredLabel sets the preferred label for edge selection.
func (o Outcome) WithPreferredLabel(label string) Outcome {
	o.PreferredLabel = label
	return o
}

// WithSuggestedNexts sets the suggested next node IDs.
func (o Outcome) WithSuggestedNexts(nodeIDs ...string) Outcome {
	o.SuggestedNexts = nodeIDs
	return o
}

// Checkpoint represents a serializable snapshot of the execution state.
type Checkpoint struct {
	RunID          string             `json:"run_id"`
	CurrentNodeID  string             `json:"current_node_id"`
	CompletedNodes []string           `json:"completed_nodes"`
	NodeOutcomes   map[string]Outcome `json:"node_outcomes"`
	Context        *Context           `json:"context"`
	CreatedAt      time.Time          `json:"created_at"`
	GraphHash      string             `json:"graph_hash,omitempty"`
}

// NewCheckpoint creates a new checkpoint.
func NewCheckpoint(runID, currentNodeID string, completedNodes []string, ctx *Context) *Checkpoint {
	return &Checkpoint{
		RunID:          runID,
		CurrentNodeID:  currentNodeID,
		CompletedNodes: completedNodes,
		NodeOutcomes:   make(map[string]Outcome),
		Context:        ctx,
		CreatedAt:      time.Now(),
	}
}

// Save writes the checkpoint to a file.
func (cp *Checkpoint) Save(logsRoot string) error {
	if err := os.MkdirAll(logsRoot, 0755); err != nil {
		return fmt.Errorf("create logs directory: %w", err)
	}

	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal checkpoint: %w", err)
	}

	path := filepath.Join(logsRoot, "checkpoint.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write checkpoint: %w", err)
	}

	return nil
}

// LoadCheckpoint loads a checkpoint from a file.
func LoadCheckpoint(logsRoot string) (*Checkpoint, error) {
	path := filepath.Join(logsRoot, "checkpoint.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read checkpoint: %w", err)
	}

	var cp Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, fmt.Errorf("unmarshal checkpoint: %w", err)
	}

	return &cp, nil
}

// StatusFile represents the status.json file written by handlers.
type StatusFile struct {
	Status         OutcomeStatus  `json:"status"`
	Notes          string         `json:"notes,omitempty"`
	FailureReason  string         `json:"failure_reason,omitempty"`
	ContextUpdates map[string]any `json:"context_updates,omitempty"`
	PreferredLabel string         `json:"preferred_label,omitempty"`
	SuggestedNexts []string       `json:"suggested_next_ids,omitempty"`
	CompletedAt    time.Time      `json:"completed_at"`
}

// WriteStatus writes an outcome as a status file.
func WriteStatus(stageDir string, outcome Outcome) error {
	sf := StatusFile{
		Status:         outcome.Status,
		Notes:          outcome.Notes,
		FailureReason:  outcome.FailureReason,
		ContextUpdates: outcome.ContextUpdates,
		PreferredLabel: outcome.PreferredLabel,
		SuggestedNexts: outcome.SuggestedNexts,
		CompletedAt:    time.Now(),
	}

	data, err := json.MarshalIndent(sf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal status: %w", err)
	}

	path := filepath.Join(stageDir, "status.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write status: %w", err)
	}

	return nil
}

// ReadStatus reads an outcome from a status file.
func ReadStatus(stageDir string) (*Outcome, error) {
	path := filepath.Join(stageDir, "status.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read status: %w", err)
	}

	var sf StatusFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil, fmt.Errorf("unmarshal status: %w", err)
	}

	outcome := &Outcome{
		Status:         sf.Status,
		Notes:          sf.Notes,
		FailureReason:  sf.FailureReason,
		ContextUpdates: sf.ContextUpdates,
		PreferredLabel: sf.PreferredLabel,
		SuggestedNexts: sf.SuggestedNexts,
	}

	return outcome, nil
}

package context

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestContextGetSet(t *testing.T) {
	ctx := New()

	// Test string
	ctx.Set("name", "test")
	if ctx.GetString("name") != "test" {
		t.Errorf("Expected 'test', got '%s'", ctx.GetString("name"))
	}

	// Test int
	ctx.Set("count", 42)
	if ctx.GetInt("count") != 42 {
		t.Errorf("Expected 42, got %d", ctx.GetInt("count"))
	}

	// Test bool
	ctx.Set("enabled", true)
	if !ctx.GetBool("enabled") {
		t.Error("Expected true")
	}

	// Test missing key
	if ctx.GetString("missing") != "" {
		t.Error("Expected empty string for missing key")
	}
	if ctx.GetInt("missing") != 0 {
		t.Error("Expected 0 for missing key")
	}
	if ctx.GetBool("missing") {
		t.Error("Expected false for missing key")
	}
}

func TestContextDelete(t *testing.T) {
	ctx := New()
	ctx.Set("key", "value")

	if _, ok := ctx.Get("key"); !ok {
		t.Error("Key should exist")
	}

	ctx.Delete("key")

	if _, ok := ctx.Get("key"); ok {
		t.Error("Key should not exist after delete")
	}
}

func TestContextClone(t *testing.T) {
	ctx := New()
	ctx.Set("a", 1)
	ctx.Set("b", "two")

	clone := ctx.Clone()

	// Modify original
	ctx.Set("a", 100)
	ctx.Set("c", "three")

	// Clone should be unaffected
	if clone.GetInt("a") != 1 {
		t.Errorf("Clone should have original value, got %d", clone.GetInt("a"))
	}
	if _, ok := clone.Get("c"); ok {
		t.Error("Clone should not have new key")
	}
}

func TestContextMerge(t *testing.T) {
	ctx1 := New()
	ctx1.Set("a", 1)
	ctx1.Set("b", 2)

	ctx2 := New()
	ctx2.Set("b", 20)
	ctx2.Set("c", 3)

	ctx1.Merge(ctx2)

	if ctx1.GetInt("a") != 1 {
		t.Errorf("Expected a=1, got %d", ctx1.GetInt("a"))
	}
	if ctx1.GetInt("b") != 20 {
		t.Errorf("Expected b=20 (overwritten), got %d", ctx1.GetInt("b"))
	}
	if ctx1.GetInt("c") != 3 {
		t.Errorf("Expected c=3, got %d", ctx1.GetInt("c"))
	}
}

func TestContextAll(t *testing.T) {
	ctx := New()
	ctx.Set("x", 10)
	ctx.Set("y", 20)

	all := ctx.All()

	if len(all) != 2 {
		t.Errorf("Expected 2 items, got %d", len(all))
	}
	if all["x"] != 10 || all["y"] != 20 {
		t.Errorf("Unexpected values: %v", all)
	}
}

func TestContextJSON(t *testing.T) {
	ctx := New()
	ctx.Set("name", "test")
	ctx.Set("count", 42)

	// Marshal
	data, err := json.Marshal(ctx)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Unmarshal
	ctx2 := New()
	if err := json.Unmarshal(data, ctx2); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if ctx2.GetString("name") != "test" {
		t.Errorf("Expected 'test', got '%s'", ctx2.GetString("name"))
	}
}

func TestOutcome(t *testing.T) {
	// Success outcome
	success := NewSuccessOutcome("Done")
	if success.Status != StatusSuccess {
		t.Errorf("Expected success status")
	}
	if success.Notes != "Done" {
		t.Errorf("Expected notes 'Done'")
	}

	// Fail outcome
	fail := NewFailOutcome("Error occurred")
	if fail.Status != StatusFail {
		t.Errorf("Expected fail status")
	}
	if fail.FailureReason != "Error occurred" {
		t.Errorf("Expected failure reason")
	}

	// Retry outcome
	retry := NewRetryOutcome("Temporary failure")
	if retry.Status != StatusRetry {
		t.Errorf("Expected retry status")
	}

	// Chaining
	outcome := NewSuccessOutcome("Test").
		WithContextUpdate("key", "value").
		WithPreferredLabel("Next").
		WithSuggestedNexts("node1", "node2")

	if outcome.ContextUpdates["key"] != "value" {
		t.Error("Expected context update")
	}
	if outcome.PreferredLabel != "Next" {
		t.Error("Expected preferred label")
	}
	if len(outcome.SuggestedNexts) != 2 {
		t.Error("Expected 2 suggested nexts")
	}
}

func TestCheckpoint(t *testing.T) {
	tmpDir := t.TempDir()

	ctx := New()
	ctx.Set("stage", "test")

	cp := NewCheckpoint("run-1", "node-a", []string{"node-start", "node-a"}, ctx)
	cp.NodeOutcomes = map[string]Outcome{
		"node-start": NewSuccessOutcome("Started"),
		"node-a":     NewSuccessOutcome("Completed A"),
	}

	// Save
	if err := cp.Save(tmpDir); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load
	loaded, err := LoadCheckpoint(tmpDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.RunID != "run-1" {
		t.Errorf("Expected run ID 'run-1', got '%s'", loaded.RunID)
	}
	if loaded.CurrentNodeID != "node-a" {
		t.Errorf("Expected current node 'node-a', got '%s'", loaded.CurrentNodeID)
	}
	if len(loaded.CompletedNodes) != 2 {
		t.Errorf("Expected 2 completed nodes, got %d", len(loaded.CompletedNodes))
	}
	if loaded.Context.GetString("stage") != "test" {
		t.Error("Context not preserved")
	}
	if loaded.NodeOutcomes["node-a"].Status != StatusSuccess {
		t.Error("Node outcome not preserved")
	}
}

func TestStatusFile(t *testing.T) {
	tmpDir := t.TempDir()
	stageDir := filepath.Join(tmpDir, "stage-1")
	if err := os.MkdirAll(stageDir, 0755); err != nil {
		t.Fatal(err)
	}

	outcome := NewSuccessOutcome("Stage completed").
		WithContextUpdate("result", "ok").
		WithPreferredLabel("Continue")

	// Write
	if err := WriteStatus(stageDir, outcome); err != nil {
		t.Fatalf("WriteStatus failed: %v", err)
	}

	// Read
	read, err := ReadStatus(stageDir)
	if err != nil {
		t.Fatalf("ReadStatus failed: %v", err)
	}

	if read.Status != StatusSuccess {
		t.Errorf("Expected success status")
	}
	if read.Notes != "Stage completed" {
		t.Errorf("Expected notes")
	}
	if read.ContextUpdates["result"] != "ok" {
		t.Errorf("Expected context update")
	}
	if read.PreferredLabel != "Continue" {
		t.Errorf("Expected preferred label")
	}
}

func TestContextConcurrency(t *testing.T) {
	ctx := New()

	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			ctx.Set("counter", i)
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			_ = ctx.GetInt("counter")
		}
		done <- true
	}()

	// Wait for both
	<-done
	<-done

	// Should not panic
}

package roles

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseValidRole(t *testing.T) {
	content := `---
name: tester
description: A test role
tools:
  read_file: true
  shell: true
---
You are a test role.

## Guidelines
- Do testing things.`

	role, err := Parse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if role.Name != "tester" {
		t.Errorf("name: got %q, want %q", role.Name, "tester")
	}
	if role.Description != "A test role" {
		t.Errorf("description: got %q, want %q", role.Description, "A test role")
	}
	if len(role.Tools) != 2 {
		t.Errorf("tools count: got %d, want 2", len(role.Tools))
	}
	if !role.Tools["read_file"] {
		t.Error("expected read_file to be true")
	}
	if !role.Tools["shell"] {
		t.Error("expected shell to be true")
	}
	if role.Tools["write_file"] {
		t.Error("expected write_file to be false/absent")
	}
	if role.SystemPrompt == "" {
		t.Error("expected non-empty system prompt")
	}
}

func TestParseMissingFrontmatter(t *testing.T) {
	_, err := Parse("Just some text without frontmatter")
	if err == nil {
		t.Error("expected error for missing frontmatter")
	}
}

func TestParseMissingClosingDelimiter(t *testing.T) {
	_, err := Parse("---\nname: broken\n")
	if err == nil {
		t.Error("expected error for missing closing delimiter")
	}
}

func TestParseMissingName(t *testing.T) {
	content := `---
description: No name provided
---
Body text.`

	_, err := Parse(content)
	if err == nil {
		t.Error("expected error for missing name")
	}
}

func TestToolAllowed(t *testing.T) {
	role := RoleDefinition{
		Name: "restricted",
		Tools: map[string]bool{
			"read_file": true,
			"shell":     true,
		},
	}

	tests := []struct {
		tool    string
		allowed bool
	}{
		{"read_file", true},
		{"shell", true},
		{"write_file", false},
		{"edit_file", false},
		{"grep", false},
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			if got := role.ToolAllowed(tt.tool); got != tt.allowed {
				t.Errorf("ToolAllowed(%q) = %v, want %v", tt.tool, got, tt.allowed)
			}
		})
	}
}

func TestAllToolsAllowed(t *testing.T) {
	role := RoleDefinition{Name: "unrestricted"}
	if !role.AllToolsAllowed() {
		t.Error("expected AllToolsAllowed with nil tools map")
	}
	if !role.ToolAllowed("anything") {
		t.Error("expected ToolAllowed to return true with nil tools map")
	}
}

func TestExpandPrompt(t *testing.T) {
	role := RoleDefinition{
		Name:         "test",
		SystemPrompt: "Platform: {{.Platform}}, Dir: {{.WorkDir}}",
	}

	expanded, err := role.ExpandPrompt("/test/dir")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if expanded == role.SystemPrompt {
		t.Error("expected template to be expanded")
	}
	if !contains(expanded, "/test/dir") {
		t.Errorf("expected expanded prompt to contain /test/dir, got: %s", expanded)
	}
}

func TestLoadDefault(t *testing.T) {
	for _, name := range DefaultRoleNames {
		t.Run(name, func(t *testing.T) {
			role, err := LoadDefault(name)
			if err != nil {
				t.Fatalf("LoadDefault(%q): %v", name, err)
			}
			if role.Name != name {
				t.Errorf("name: got %q, want %q", role.Name, name)
			}
			if role.Description == "" {
				t.Errorf("expected non-empty description for %q", name)
			}
			if role.SystemPrompt == "" {
				t.Errorf("expected non-empty system prompt for %q", name)
			}
			if len(role.Tools) == 0 {
				t.Errorf("expected non-empty tools map for %q", name)
			}
		})
	}
}

func TestLoadDefaultUnknown(t *testing.T) {
	_, err := LoadDefault("nonexistent")
	if err == nil {
		t.Error("expected error for unknown role")
	}
}

func TestListDefaults(t *testing.T) {
	roles, err := ListDefaults()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(roles) != len(DefaultRoleNames) {
		t.Errorf("got %d roles, want %d", len(roles), len(DefaultRoleNames))
	}
}

func TestLoadProjectLocal(t *testing.T) {
	// Create a temp project-local role file
	dir := t.TempDir()
	roleDir := filepath.Join(dir, ".attractor", "roles")
	if err := os.MkdirAll(roleDir, 0755); err != nil {
		t.Fatal(err)
	}

	content := `---
name: custom
description: A custom project role
tools:
  read_file: true
---
Custom project role prompt.`

	if err := os.WriteFile(filepath.Join(roleDir, "custom.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Change to temp dir so .attractor/roles is found
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	role, err := Load("custom")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if role.Name != "custom" {
		t.Errorf("name: got %q, want %q", role.Name, "custom")
	}
}

func TestLoadFallsBackToDefault(t *testing.T) {
	role, err := Load("developer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if role.Name != "developer" {
		t.Errorf("name: got %q, want %q", role.Name, "developer")
	}
}

func TestExpandFileRef(t *testing.T) {
	dir := t.TempDir()
	promptFile := filepath.Join(dir, "prompt.txt")
	if err := os.WriteFile(promptFile, []byte("expanded content"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ExpandFileRef("{file:./prompt.txt}", dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "expanded content" {
		t.Errorf("got %q, want %q", result, "expanded content")
	}
}

func TestExpandFileRefNoRef(t *testing.T) {
	result, err := ExpandFileRef("plain text", "/tmp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "plain text" {
		t.Errorf("got %q, want %q", result, "plain text")
	}
}

func TestAllowedToolNames(t *testing.T) {
	role := RoleDefinition{
		Name: "test",
		Tools: map[string]bool{
			"read_file":  true,
			"write_file": true,
			"shell":      false,
		},
	}

	names := role.AllowedToolNames()
	if len(names) != 2 {
		t.Errorf("got %d names, want 2", len(names))
	}

	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	if !nameSet["read_file"] || !nameSet["write_file"] {
		t.Errorf("expected read_file and write_file, got %v", names)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

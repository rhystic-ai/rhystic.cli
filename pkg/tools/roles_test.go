package tools

import (
	"sort"
	"testing"

	"github.com/rhystic/attractor/pkg/roles"
)

func TestRegistryForRoleDeveloper(t *testing.T) {
	role, err := roles.LoadDefault("developer")
	if err != nil {
		t.Fatalf("load developer role: %v", err)
	}

	reg := RegistryForRole(role)

	// Developer should have all tools
	allTools := CreateDefaultRegistry()
	if len(reg.All()) != len(allTools.All()) {
		t.Errorf("developer: got %d tools, want %d", len(reg.All()), len(allTools.All()))
	}
}

func TestRegistryForRoleReviewer(t *testing.T) {
	role, err := roles.LoadDefault("reviewer")
	if err != nil {
		t.Fatalf("load reviewer role: %v", err)
	}

	reg := RegistryForRole(role)

	// Reviewer should only have read_file, grep, glob, list_dir
	names := reg.Names()
	sort.Strings(names)

	expected := []string{"glob", "grep", "list_dir", "read_file"}
	sort.Strings(expected)

	if len(names) != len(expected) {
		t.Fatalf("reviewer: got %d tools %v, want %d %v", len(names), names, len(expected), expected)
	}

	for i, name := range names {
		if name != expected[i] {
			t.Errorf("reviewer: tool[%d] = %q, want %q", i, name, expected[i])
		}
	}

	// Verify restricted tools are absent
	for _, blocked := range []string{"write_file", "edit_file", "shell"} {
		if _, ok := reg.Get(blocked); ok {
			t.Errorf("reviewer should not have %q", blocked)
		}
	}
}

func TestRegistryForRoleProjectManager(t *testing.T) {
	role, err := roles.LoadDefault("project-manager")
	if err != nil {
		t.Fatalf("load project-manager role: %v", err)
	}

	reg := RegistryForRole(role)

	// PM should have read_file, shell, glob, list_dir
	expected := map[string]bool{
		"read_file": true,
		"shell":     true,
		"glob":      true,
		"list_dir":  true,
	}

	for _, tool := range reg.All() {
		if !expected[tool.Name()] {
			t.Errorf("project-manager: unexpected tool %q", tool.Name())
		}
	}

	if len(reg.All()) != len(expected) {
		t.Errorf("project-manager: got %d tools, want %d", len(reg.All()), len(expected))
	}
}

func TestRegistryForRoleResearcher(t *testing.T) {
	role, err := roles.LoadDefault("researcher")
	if err != nil {
		t.Fatalf("load researcher role: %v", err)
	}

	reg := RegistryForRole(role)

	expected := map[string]bool{
		"read_file":  true,
		"write_file": true,
		"shell":      true,
		"glob":       true,
		"list_dir":   true,
	}

	for _, tool := range reg.All() {
		if !expected[tool.Name()] {
			t.Errorf("researcher: unexpected tool %q", tool.Name())
		}
	}

	if len(reg.All()) != len(expected) {
		t.Errorf("researcher: got %d tools, want %d", len(reg.All()), len(expected))
	}
}

func TestRegistryForRoleUnrestricted(t *testing.T) {
	// Role with no tools map = all tools
	role := roles.RoleDefinition{Name: "open"}
	reg := RegistryForRole(role)

	allTools := CreateDefaultRegistry()
	if len(reg.All()) != len(allTools.All()) {
		t.Errorf("unrestricted: got %d tools, want %d", len(reg.All()), len(allTools.All()))
	}
}

func TestFilterRegistry(t *testing.T) {
	full := CreateDefaultRegistry()
	role := roles.RoleDefinition{
		Name: "readonly",
		Tools: map[string]bool{
			"read_file": true,
			"glob":      true,
		},
	}

	filtered := FilterRegistry(full, role)
	if len(filtered.All()) != 2 {
		t.Errorf("got %d tools, want 2", len(filtered.All()))
	}

	if _, ok := filtered.Get("read_file"); !ok {
		t.Error("expected read_file in filtered registry")
	}
	if _, ok := filtered.Get("glob"); !ok {
		t.Error("expected glob in filtered registry")
	}
	if _, ok := filtered.Get("shell"); ok {
		t.Error("shell should not be in filtered registry")
	}
}

func TestFilterRegistryUnrestricted(t *testing.T) {
	full := CreateDefaultRegistry()
	role := roles.RoleDefinition{Name: "open"}

	filtered := FilterRegistry(full, role)
	// Should return the same registry when unrestricted
	if len(filtered.All()) != len(full.All()) {
		t.Errorf("got %d tools, want %d", len(filtered.All()), len(full.All()))
	}
}

// Package roles provides role definitions for pipeline nodes.
//
// Roles determine the system prompt and allowed tool set for a node's
// LLM session. They are defined as markdown files with YAML frontmatter
// and resolved in order: node attribute > project-local > global > embedded default.
package roles

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
	"time"

	"gopkg.in/yaml.v3"
)

// RoleDefinition holds a parsed role with its metadata and system prompt.
type RoleDefinition struct {
	Name         string          `yaml:"name"`
	Description  string          `yaml:"description"`
	Tools        map[string]bool `yaml:"tools"`
	SystemPrompt string          `yaml:"-"` // Parsed from markdown body
}

// TemplateData holds the variables available for system prompt expansion.
type TemplateData struct {
	Platform string
	WorkDir  string
	Time     string
}

// Parse parses a role definition from markdown with YAML frontmatter.
// The format is:
//
//	---
//	name: developer
//	description: Full-stack developer
//	tools:
//	  read_file: true
//	  shell: true
//	---
//	You are an expert developer...
func Parse(content string) (RoleDefinition, error) {
	var role RoleDefinition

	// Trim leading whitespace/BOM
	content = strings.TrimSpace(content)

	if !strings.HasPrefix(content, "---") {
		return role, fmt.Errorf("missing YAML frontmatter delimiter")
	}

	// Find closing delimiter
	rest := content[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return role, fmt.Errorf("missing closing frontmatter delimiter")
	}

	frontmatter := rest[:idx]
	body := strings.TrimSpace(rest[idx+4:]) // skip past "\n---"

	// Parse YAML frontmatter
	if err := yaml.Unmarshal([]byte(frontmatter), &role); err != nil {
		return role, fmt.Errorf("parse frontmatter: %w", err)
	}

	if role.Name == "" {
		return role, fmt.Errorf("role name is required")
	}

	role.SystemPrompt = body
	return role, nil
}

// ExpandPrompt expands template variables in the role's system prompt.
func (r *RoleDefinition) ExpandPrompt(workDir string) (string, error) {
	if r.SystemPrompt == "" {
		return "", nil
	}

	data := TemplateData{
		Platform: runtime.GOOS,
		WorkDir:  workDir,
		Time:     time.Now().Format(time.RFC3339),
	}

	tmpl, err := template.New("prompt").Parse(r.SystemPrompt)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("expand template: %w", err)
	}

	return buf.String(), nil
}

// AllToolsAllowed returns true if the role has no tool restrictions
// (empty or nil tools map means all tools are permitted).
func (r *RoleDefinition) AllToolsAllowed() bool {
	return len(r.Tools) == 0
}

// ToolAllowed checks whether a specific tool is permitted for this role.
func (r *RoleDefinition) ToolAllowed(toolName string) bool {
	if r.AllToolsAllowed() {
		return true
	}
	return r.Tools[toolName]
}

// AllowedToolNames returns the list of tool names this role permits.
// If the tools map is empty, returns nil (meaning all tools allowed).
func (r *RoleDefinition) AllowedToolNames() []string {
	if r.AllToolsAllowed() {
		return nil
	}
	var names []string
	for name, allowed := range r.Tools {
		if allowed {
			names = append(names, name)
		}
	}
	return names
}

// Load resolves and loads a role definition by name.
// Resolution order:
//  1. Project-local: .attractor/roles/<name>.md
//  2. Global: ~/.attractor/roles/<name>.md
//  3. Embedded defaults
//
// Returns an error if the role is not found at any level.
func Load(name string) (RoleDefinition, error) {
	// Normalize name
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return RoleDefinition{}, fmt.Errorf("empty role name")
	}

	// 1. Project-local
	projectPath := filepath.Join(".attractor", "roles", name+".md")
	if content, err := os.ReadFile(projectPath); err == nil {
		return Parse(string(content))
	}

	// 2. Global (~/.attractor/roles/)
	if home, err := os.UserHomeDir(); err == nil {
		globalPath := filepath.Join(home, ".attractor", "roles", name+".md")
		if content, err := os.ReadFile(globalPath); err == nil {
			return Parse(string(content))
		}
	}

	// 3. Embedded defaults
	return LoadDefault(name)
}

// LoadFromFile loads a role definition from a specific file path.
func LoadFromFile(path string) (RoleDefinition, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return RoleDefinition{}, fmt.Errorf("read role file: %w", err)
	}
	return Parse(string(content))
}

// ExpandFileRef expands a {file:./path} reference by reading the file content.
// If the input doesn't contain a file reference, it is returned unchanged.
func ExpandFileRef(s string, baseDir string) (string, error) {
	const prefix = "{file:"
	const suffix = "}"

	idx := strings.Index(s, prefix)
	if idx < 0 {
		return s, nil
	}

	end := strings.Index(s[idx:], suffix)
	if end < 0 {
		return s, nil
	}

	refPath := s[idx+len(prefix) : idx+end]
	refPath = strings.TrimSpace(refPath)

	// Resolve relative to baseDir
	if !filepath.IsAbs(refPath) {
		refPath = filepath.Join(baseDir, refPath)
	}

	content, err := os.ReadFile(refPath)
	if err != nil {
		return "", fmt.Errorf("expand file ref %q: %w", refPath, err)
	}

	// Replace the entire {file:...} token with file contents
	result := s[:idx] + string(content) + s[idx+end+1:]
	return result, nil
}

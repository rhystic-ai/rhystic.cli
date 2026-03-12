package roles

import (
	"embed"
	"fmt"
	"strings"
)

//go:embed defaults/*.md
var defaultRoles embed.FS

// DefaultRoleNames lists the names of all built-in roles.
var DefaultRoleNames = []string{
	"researcher",
	"project-manager",
	"developer",
	"quality",
	"reviewer",
	"devops",
}

// LoadDefault loads a built-in role definition by name.
func LoadDefault(name string) (RoleDefinition, error) {
	name = strings.ToLower(strings.TrimSpace(name))

	content, err := defaultRoles.ReadFile("defaults/" + name + ".md")
	if err != nil {
		return RoleDefinition{}, fmt.Errorf("unknown role %q: no default definition found", name)
	}

	return Parse(string(content))
}

// ListDefaults returns all built-in role definitions.
func ListDefaults() ([]RoleDefinition, error) {
	var roles []RoleDefinition
	for _, name := range DefaultRoleNames {
		role, err := LoadDefault(name)
		if err != nil {
			return nil, fmt.Errorf("load default %q: %w", name, err)
		}
		roles = append(roles, role)
	}
	return roles, nil
}

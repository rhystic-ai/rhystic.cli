package tools

import (
	"github.com/rhystic/attractor/pkg/roles"
)

// RegistryForRole creates a new Registry containing only the tools
// permitted by the given role. If the role has no tool restrictions
// (empty tools map), all default tools are included.
func RegistryForRole(role roles.RoleDefinition) *Registry {
	if role.AllToolsAllowed() {
		return CreateDefaultRegistry()
	}

	all := CreateDefaultRegistry()
	filtered := NewRegistry()

	for _, tool := range all.All() {
		if role.ToolAllowed(tool.Name()) {
			filtered.Register(tool)
		}
	}

	return filtered
}

// FilterRegistry creates a new Registry from an existing one, keeping
// only tools permitted by the role.
func FilterRegistry(reg *Registry, role roles.RoleDefinition) *Registry {
	if role.AllToolsAllowed() {
		return reg
	}

	filtered := NewRegistry()
	for _, tool := range reg.All() {
		if role.ToolAllowed(tool.Name()) {
			filtered.Register(tool)
		}
	}
	return filtered
}

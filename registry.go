package harness

import (
	"errors"
	"fmt"
	"sort"
)

// toolRegistry is intentionally private: registration is configuration, while
// Tool is the stable extension boundary exposed to users.
type toolRegistry struct {
	tools map[string]Tool
}

// newToolRegistry creates an empty registry.
// @param none.
// @return new tool registry.
func newToolRegistry() *toolRegistry { return &toolRegistry{tools: make(map[string]Tool)} }

// add validates and registers one tool.
// @param tool tool to register.
// @return validation or duplicate-name error.
func (r *toolRegistry) add(tool Tool) error {
	if tool == nil {
		return errors.New("harness: nil tool")
	}
	name := tool.Definition().Name
	if !validName(name) {
		return fmt.Errorf("harness: invalid tool name %q", name)
	}
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("harness: tool %q already registered", name)
	}
	r.tools[name] = tool
	return nil
}

// selectTools resolves a skill allowlist and sorted definitions.
// @param allowed allowed tool names; empty selects all.
// @return selected tools, definitions, and validation error.
func (r *toolRegistry) selectTools(allowed []string) (map[string]Tool, []ToolDefinition, error) {
	selected := make(map[string]Tool)
	if len(allowed) == 0 {
		for name, tool := range r.tools {
			selected[name] = tool
		}
	} else {
		for _, name := range allowed {
			tool, ok := r.tools[name]
			if !ok {
				return nil, nil, fmt.Errorf("harness: skill references unknown tool %q", name)
			}
			selected[name] = tool
		}
	}
	names := make([]string, 0, len(selected))
	for name := range selected {
		names = append(names, name)
	}
	sort.Strings(names)
	definitions := make([]ToolDefinition, 0, len(names))
	for _, name := range names {
		definitions = append(definitions, selected[name].Definition())
	}
	return selected, definitions, nil
}

// validName checks portable tool and skill names.
// @param name candidate name.
// @return true when valid.
func validName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(i > 0 && r >= '0' && r <= '9') || (i > 0 && (r == '_' || r == '-')) {
			continue
		}
		return false
	}
	return true
}

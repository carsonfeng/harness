package harness

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

const skillToolName = "load_skill"

var (
	// ErrReservedToolName indicates an attempt to replace an internal tool.
	ErrReservedToolName = errors.New("harness: tool name is reserved")
)

type skillActivation struct {
	Name         string   `json:"name"`
	Instructions string   `json:"instructions"`
	Tools        []string `json:"tools,omitempty"`
	MaxSteps     int      `json:"max_steps,omitempty"`
}

type loadSkillArgs struct {
	Name string `json:"name"`
}

// skillSnapshot returns an independent view of registered skills.
// @param h configured harness.
// @return skills by name.
func (h *Harness) skillSnapshot() map[string]Skill {
	h.mu.RLock()
	defer h.mu.RUnlock()
	result := make(map[string]Skill, len(h.skills))
	for name, skill := range h.skills {
		skill.Tools = append([]string(nil), skill.Tools...)
		result[name] = skill
	}
	return result
}

// skillToolDefinition describes available skills without exposing instructions.
// @param skills registered skills.
// @return model-visible load_skill definition.
func skillToolDefinition(skills map[string]Skill) ToolDefinition {
	names := make([]string, 0, len(skills))
	for name := range skills {
		names = append(names, name)
	}
	sort.Strings(names)
	lines := []string{"Load the instructions for one relevant skill before handling a matching task. Call this tool only when a listed skill clearly applies, and call it alone without other tool calls in the same response."}
	for _, name := range names {
		description := strings.TrimSpace(skills[name].Description)
		if description == "" {
			description = "No description provided."
		}
		lines = append(lines, "- "+name+": "+description)
	}
	schema, err := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "Skill to load.",
				"enum":        names,
			},
		},
		"required":             []string{"name"},
		"additionalProperties": false,
	})
	if err != nil {
		panic("harness: encode skill schema: " + err.Error())
	}
	return ToolDefinition{Name: skillToolName, Description: strings.Join(lines, "\n"), Parameters: schema}
}

// activeSkill restores the most recently loaded skill from Thread history.
// @param messages conversation messages.
// @return active skill or history validation error.
func activeSkill(messages []Message) (*Skill, error) {
	for i := len(messages) - 1; i >= 0; i-- {
		message := messages[i]
		if message.Role != RoleTool || message.Name != skillToolName {
			continue
		}
		call, ok := skillCallForResult(messages, i)
		if !ok {
			continue
		}
		var failure struct {
			Error string `json:"error"`
		}
		if json.Unmarshal([]byte(message.Content), &failure) == nil && failure.Error != "" {
			continue
		}
		activation, err := decodeSkillActivation(message.Content)
		if err != nil {
			return nil, fmt.Errorf("harness: decode active skill: %w", err)
		}
		args, err := decodeLoadSkillArgs(call.Arguments)
		if err != nil {
			return nil, fmt.Errorf("harness: decode active skill call: %w", err)
		}
		if activation.Name != args.Name {
			return nil, fmt.Errorf("harness: active skill %q does not match requested skill %q", activation.Name, args.Name)
		}
		if !validName(activation.Name) {
			return nil, fmt.Errorf("harness: active skill has invalid name %q", activation.Name)
		}
		for _, name := range activation.Tools {
			if !validName(name) {
				return nil, fmt.Errorf("harness: active skill contains invalid tool name %q", name)
			}
		}
		if activation.MaxSteps < 0 {
			return nil, errors.New("harness: active skill contains an invalid max_steps value")
		}
		return &Skill{
			Name:         activation.Name,
			Instructions: activation.Instructions,
			Tools:        append([]string(nil), activation.Tools...),
			MaxSteps:     activation.MaxSteps,
		}, nil
	}
	return nil, nil
}

// skillCallForResult finds the load_skill call for one adjacent tool result.
// @param messages conversation messages.
// @param resultIndex index of a load_skill result.
// @return matching call and whether it exists.
func skillCallForResult(messages []Message, resultIndex int) (ToolCall, bool) {
	result := messages[resultIndex]
	i := resultIndex - 1
	for i >= 0 && messages[i].Role == RoleTool {
		i--
	}
	if i < 0 || messages[i].Role != RoleAssistant {
		return ToolCall{}, false
	}
	for _, call := range messages[i].ToolCalls {
		if call.ID == result.ToolCallID && call.Name == skillToolName {
			return call, true
		}
	}
	return ToolCall{}, false
}

// decodeSkillActivation parses one strict activation result.
// @param content serialized load_skill result.
// @return activation or validation error.
func decodeSkillActivation(content string) (skillActivation, error) {
	var activation skillActivation
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&activation); err != nil {
		return skillActivation{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return skillActivation{}, errors.New("expected one JSON object")
	}
	return activation, nil
}

// decodeLoadSkillArgs parses one strict load_skill argument object.
// @param raw serialized model arguments.
// @return arguments or validation error.
func decodeLoadSkillArgs(raw json.RawMessage) (loadSkillArgs, error) {
	var args loadSkillArgs
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&args); err != nil {
		return loadSkillArgs{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return loadSkillArgs{}, errors.New("expected one JSON object")
	}
	return args, nil
}

// loadSkill validates one model-selected skill and serializes its instructions.
// @param raw model-supplied JSON arguments.
// @param skills registered skills.
// @return selected skill, JSON result, and validation error.
func loadSkill(raw json.RawMessage, skills map[string]Skill) (*Skill, string, error) {
	args, err := decodeLoadSkillArgs(raw)
	if err != nil {
		return nil, "", fmt.Errorf("decode skill arguments: %w", err)
	}
	skill, ok := skills[args.Name]
	if !ok {
		return nil, "", fmt.Errorf("unknown skill %q", args.Name)
	}
	encoded, err := json.Marshal(skillActivation{
		Name:         skill.Name,
		Instructions: skill.Instructions,
		Tools:        skill.Tools,
		MaxSteps:     skill.MaxSteps,
	})
	if err != nil {
		return nil, "", fmt.Errorf("encode skill instructions: %w", err)
	}
	return &skill, string(encoded), nil
}

// definitionsForRun resolves tools for the active skill and adds discovery.
// @param registry registered application tools.
// @param skills registered skills.
// @param active currently loaded skill, if any.
// @return executable tools, model definitions, and validation error.
func definitionsForRun(registry *toolRegistry, skills map[string]Skill, active *Skill) (map[string]Tool, []ToolDefinition, error) {
	var allowed []string
	if active != nil {
		allowed = active.Tools
	}
	selected, definitions, err := registry.selectTools(allowed)
	if err != nil {
		return nil, nil, err
	}
	if active == nil && len(skills) > 0 {
		definitions = append(definitions, skillToolDefinition(skills))
		sort.Slice(definitions, func(i, j int) bool { return definitions[i].Name < definitions[j].Name })
	}
	return selected, definitions, nil
}

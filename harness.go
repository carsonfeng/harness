package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

var (
	// ErrNoModel indicates that Run was called without configuring a Model.
	ErrNoModel = errors.New("harness: no model configured")
	// ErrMaxSteps indicates that the model did not finish within the run limit.
	ErrMaxSteps = errors.New("harness: maximum steps reached")
)

const defaultMaxSteps = 20

// Option configures a Harness.
// @param harness harness being configured.
// @return none.
type Option func(*Harness)

// WithModel configures the model used for future runs.
// @param model provider adapter.
// @return harness option.
func WithModel(model Model) Option { return func(h *Harness) { h.model = model } }

// WithSystem sets instructions prepended to every new thread.
// @param instructions system instructions.
// @return harness option.
func WithSystem(instructions string) Option {
	return func(h *Harness) { h.system = instructions }
}

// WithMaxSteps sets the maximum model turns per run. Values below one are
// ignored.
// @param steps maximum model turns.
// @return harness option.
func WithMaxSteps(steps int) Option {
	return func(h *Harness) {
		if steps > 0 {
			h.maxSteps = steps
		}
	}
}

// WithTools registers tools during construction. Invalid or duplicate tools
// are reported when Run starts. Use Tool when registration errors are needed
// immediately.
// @param tools tools to register.
// @return harness option.
func WithTools(tools ...Tool) Option {
	return func(h *Harness) {
		for _, tool := range tools {
			if err := h.tools.add(tool); err != nil && h.setupErr == nil {
				h.setupErr = err
			}
		}
	}
}

// Harness coordinates a model, tools, skills, and conversation threads.
// Configure it before running it concurrently.
type Harness struct {
	model    Model
	system   string
	maxSteps int
	tools    *toolRegistry

	mu       sync.RWMutex
	skills   map[string]Skill
	setupErr error
}

// New creates a Harness. A model may be configured now or later with SetModel.
// @param options harness options.
// @return configured harness.
func New(options ...Option) *Harness {
	h := &Harness{maxSteps: defaultMaxSteps, tools: newToolRegistry(), skills: make(map[string]Skill)}
	for _, option := range options {
		option(h)
	}
	return h
}

// SetModel changes the model used by subsequent runs.
// @param model provider adapter.
// @return none.
func (h *Harness) SetModel(model Model) { h.model = model }

// Tool registers one tool. Names must be unique.
// @param tool tool to register.
// @return validation or duplicate-name error.
func (h *Harness) Tool(tool Tool) error { return h.tools.add(tool) }

// Tools registers several tools and stops at the first error.
// @param tools tools to register.
// @return first registration error.
func (h *Harness) Tools(tools ...Tool) error {
	for _, tool := range tools {
		if err := h.Tool(tool); err != nil {
			return err
		}
	}
	return nil
}

// Skill registers or replaces one in-memory skill.
// @param skill skill to register.
// @return name validation error.
func (h *Harness) Skill(skill Skill) error {
	if !validName(skill.Name) {
		return fmt.Errorf("harness: invalid skill name %q", skill.Name)
	}
	h.mu.Lock()
	h.skills[skill.Name] = skill
	h.mu.Unlock()
	return nil
}

// SkillDir loads skills from root and registers them by name.
// @param root directory containing skill folders.
// @return loading or parsing error.
func (h *Harness) SkillDir(root string) error {
	skills, err := LoadSkills(root)
	if err != nil {
		return err
	}
	h.mu.Lock()
	for name, skill := range skills {
		h.skills[name] = skill
	}
	h.mu.Unlock()
	return nil
}

// Result is the final answer and complete conversation from a run.
type Result struct {
	Text   string
	Thread *Thread
	Steps  int
}

// Run starts a new thread and runs until the model answers without tool calls.
// @param ctx run cancellation context.
// @param input user prompt.
// @return final result or run error.
func (h *Harness) Run(ctx context.Context, input string) (*Result, error) {
	return h.run(ctx, input, Skill{})
}

// RunSkill runs with a registered skill's instructions, tool allowlist, and
// optional step limit.
// @param ctx run cancellation context.
// @param name registered skill name.
// @param input user prompt.
// @return final result or run error.
func (h *Harness) RunSkill(ctx context.Context, name, input string) (*Result, error) {
	h.mu.RLock()
	skill, ok := h.skills[name]
	h.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("harness: unknown skill %q", name)
	}
	return h.run(ctx, input, skill)
}

// run executes the shared agent loop.
// @param ctx run cancellation context.
// @param input user prompt.
// @param skill optional run policy.
// @return final result or run error.
func (h *Harness) run(ctx context.Context, input string, skill Skill) (*Result, error) {
	if h.setupErr != nil {
		return nil, h.setupErr
	}
	if h.model == nil {
		return nil, ErrNoModel
	}
	selected, definitions, err := h.tools.selectTools(skill.Tools)
	if err != nil {
		return nil, err
	}
	thread := NewThread()
	system := h.system
	if skill.Instructions != "" {
		if system != "" {
			system += "\n\n"
		}
		system += skill.Instructions
	}
	if system != "" {
		thread.Add(Message{Role: RoleSystem, Content: system})
	}
	thread.Add(Message{Role: RoleUser, Content: input})

	maxSteps := h.maxSteps
	if skill.MaxSteps > 0 {
		maxSteps = skill.MaxSteps
	}
	for step := 1; step <= maxSteps; step++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		response, err := h.model.Generate(ctx, ModelRequest{Messages: thread.Messages(), Tools: definitions})
		if err != nil {
			return nil, fmt.Errorf("harness: generate step %d: %w", step, err)
		}
		thread.Add(Message{Role: RoleAssistant, Content: response.Text, ToolCalls: response.ToolCalls})
		if len(response.ToolCalls) == 0 {
			return &Result{Text: response.Text, Thread: thread, Steps: step}, nil
		}
		for _, call := range response.ToolCalls {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			content := executeTool(ctx, selected, call)
			thread.Add(Message{Role: RoleTool, Content: content, ToolCallID: call.ID, Name: call.Name})
		}
	}
	return nil, ErrMaxSteps
}

// executeTool invokes one tool and serializes its result.
// @param ctx tool cancellation context.
// @param tools available tools by name.
// @param call requested tool call.
// @return JSON result or error object.
func executeTool(ctx context.Context, tools map[string]Tool, call ToolCall) string {
	tool, ok := tools[call.Name]
	if !ok {
		return errorResult(fmt.Errorf("tool %q is not available", call.Name))
	}
	result, err := tool.Execute(ctx, call.Arguments)
	if err != nil {
		return errorResult(err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return errorResult(fmt.Errorf("encode result: %w", err))
	}
	return string(encoded)
}

// errorResult serializes a tool error.
// @param err tool failure.
// @return JSON error object.
func errorResult(err error) string {
	encoded, marshalErr := json.Marshal(map[string]any{"error": err.Error()})
	if marshalErr != nil {
		return `{"error":"tool failed"}`
	}
	return string(encoded)
}

package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"

	corethread "github.com/carsonfeng/harness/thread"
)

var (
	// ErrNoModel indicates that Run was called without configuring a Model.
	ErrNoModel = errors.New("harness: no model configured")
	// ErrMaxSteps indicates that the model did not finish within the run limit.
	ErrMaxSteps = errors.New("harness: maximum steps reached")
	// ErrNilThread indicates that RunThread received a nil thread.
	ErrNilThread = errors.New("harness: thread is nil")
	// ErrThreadBusy indicates that another run is using the same thread.
	ErrThreadBusy = corethread.ErrBusy
)

const defaultMaxSteps = 20
const debugPreviewLimit = 500

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

// WithDebug writes incremental agent-loop, Tool, Skill, and MCP messages.
// A nil output disables it. Debug output may contain sensitive application data.
// @param output debug log destination.
// @return harness option.
func WithDebug(output io.Writer) Option {
	return func(h *Harness) {
		if output == nil {
			h.debug = nil
			h.debugOutput = nil
			return
		}
		h.debug = log.New(output, "harness: ", log.LstdFlags)
		h.debugOutput = output
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
	model       Model
	system      string
	maxSteps    int
	tools       *toolRegistry
	debug       *log.Logger
	debugOutput io.Writer

	mu       sync.RWMutex
	skills   map[string]Skill
	setupErr error
}

// MCPServer discovers remote tools for registration.
type MCPServer interface {
	// Tools returns the server's currently available tools.
	// @param ctx discovery cancellation context.
	// @return discovered tools or connection error.
	Tools(context.Context) ([]Tool, error)
}

type mcpDebugServer interface {
	// SetDebug configures the MCP debug destination.
	// @param output debug log destination.
	// @return none.
	SetDebug(io.Writer)
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

// MCP discovers and registers tools from one MCP server.
// @param ctx discovery cancellation context.
// @param server configured MCP server.
// @return discovery or registration error.
func (h *Harness) MCP(ctx context.Context, server MCPServer) error {
	if server == nil {
		return errors.New("harness: nil MCP server")
	}
	if debugServer, ok := server.(mcpDebugServer); ok && h.debugOutput != nil {
		debugServer.SetDebug(h.debugOutput)
	}
	tools, err := server.Tools(ctx)
	if err != nil {
		return err
	}
	return h.Tools(tools...)
}

// Skill registers or replaces one in-memory skill.
// @param skill skill to register.
// @return configuration validation error.
func (h *Harness) Skill(skill Skill) error {
	if !validName(skill.Name) {
		return fmt.Errorf("harness: invalid skill name %q", skill.Name)
	}
	if skill.MaxSteps < 0 {
		return errors.New("harness: skill max steps must not be negative")
	}
	for _, name := range skill.Tools {
		if !validName(name) {
			return fmt.Errorf("harness: invalid skill tool name %q", name)
		}
	}
	skill.Tools = append([]string(nil), skill.Tools...)
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
		skill.Tools = append([]string(nil), skill.Tools...)
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
	return h.RunThread(ctx, NewThread(), input)
}

// RunThread appends one user turn to an existing conversation and runs it.
// @param ctx run cancellation context.
// @param thread conversation to continue.
// @param input user prompt.
// @return final result or run error.
func (h *Harness) RunThread(ctx context.Context, thread *Thread, input string) (*Result, error) {
	return h.runThread(ctx, thread, input)
}

// runThread executes the shared agent loop against one conversation.
// @param ctx run cancellation context.
// @param thread conversation to continue.
// @param input user prompt.
// @return final result or run error.
func (h *Harness) runThread(ctx context.Context, thread *Thread, input string) (*Result, error) {
	if h.setupErr != nil {
		return nil, h.setupErr
	}
	if h.model == nil {
		return nil, ErrNoModel
	}
	if thread == nil {
		return nil, ErrNilThread
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	release, err := thread.BeginRun()
	if err != nil {
		return nil, err
	}
	defer release()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	messages := thread.Messages()
	debugMessageOffset := len(messages)
	skills := h.skillSnapshot()
	active, err := activeSkill(messages)
	if err != nil {
		return nil, err
	}
	selected, definitions, err := definitionsForRun(h.tools, skills, active)
	if err != nil {
		return nil, err
	}
	maxSteps := h.maxSteps
	if active != nil && active.MaxSteps > 0 {
		maxSteps = active.MaxSteps
	}
	if h.system != "" && len(messages) == 0 {
		thread.Add(Message{Role: RoleSystem, Content: h.system})
	}
	thread.Add(Message{Role: RoleUser, Content: input})

	for step := 1; step <= maxSteps; step++ {
		if err := ctx.Err(); err != nil {
			h.debugf("step=%d/%d event=cancel error=%q", step, maxSteps, err)
			return nil, err
		}
		skillName := ""
		if active != nil {
			skillName = active.Name
		}
		requestMessages := thread.Messages()
		request := ModelRequest{Messages: requestMessages, Tools: definitions}
		addedMessages := requestMessages[debugMessageOffset:]
		debugMessageOffset = len(requestMessages)
		h.debugf("step=%d/%d event=model_request skill=%q added_messages=%d total_messages=%d tools=%q", step, maxSteps, skillName, len(addedMessages), len(requestMessages), toolDefinitionNames(definitions))
		h.debugJSON(step, maxSteps, "model_request_delta", compactDebugMessages(addedMessages))
		response, err := h.model.Generate(ctx, request)
		if err != nil {
			h.debugf("step=%d/%d event=model_error error=%q", step, maxSteps, err)
			return nil, fmt.Errorf("harness: generate step %d: %w", step, err)
		}
		h.debugf("step=%d/%d event=model_response tool_calls=%d final=%t text=%q", step, maxSteps, len(response.ToolCalls), len(response.ToolCalls) == 0, debugPreview(response.Text))
		for _, call := range response.ToolCalls {
			h.debugf("step=%d/%d event=tool_call tool=%q call_id=%q", step, maxSteps, call.Name, call.ID)
			h.debugJSON(step, maxSteps, "tool_arguments", map[string]any{
				"arguments": call.Arguments,
				"call_id":   call.ID,
				"tool":      call.Name,
			})
		}
		thread.Add(Message{Role: RoleAssistant, Content: response.Text, ToolCalls: response.ToolCalls})
		if len(response.ToolCalls) == 0 {
			h.debugf("step=%d/%d event=complete", step, maxSteps)
			return &Result{Text: response.Text, Thread: thread, Steps: step}, nil
		}
		hasSkillCall := false
		for _, call := range response.ToolCalls {
			if call.Name == skillToolName {
				hasSkillCall = true
				break
			}
		}
		if hasSkillCall && len(response.ToolCalls) != 1 {
			h.debugf("step=%d/%d event=skill_rejected reason=mixed_tool_calls", step, maxSteps)
			content := errorResult(errors.New("load_skill must be the only tool call in its model turn"))
			for _, call := range response.ToolCalls {
				thread.Add(Message{Role: RoleTool, Content: content, ToolCallID: call.ID, Name: call.Name})
				h.debugToolResult(step, maxSteps, call, content)
			}
			continue
		}
		for _, call := range response.ToolCalls {
			if err := ctx.Err(); err != nil {
				h.debugf("step=%d/%d event=cancel error=%q", step, maxSteps, err)
				return nil, err
			}
			h.debugf("step=%d/%d event=tool_start tool=%q call_id=%q", step, maxSteps, call.Name, call.ID)
			var content string
			if call.Name == skillToolName {
				if active != nil {
					content = errorResult(errors.New("a skill is already active in this thread"))
				} else {
					loaded, result, loadErr := loadSkill(call.Arguments, skills)
					if loadErr != nil {
						content = errorResult(loadErr)
					} else {
						newSelected, newDefinitions, selectErr := definitionsForRun(h.tools, skills, loaded)
						if selectErr != nil {
							content = errorResult(selectErr)
						} else {
							active = loaded
							selected = newSelected
							definitions = newDefinitions
							content = result
							budget := h.maxSteps
							if active.MaxSteps > 0 {
								budget = active.MaxSteps
							}
							maxSteps = step + budget
							h.debugf("step=%d/%d event=skill_activated skill=%q allowed_tools=%q budget=%d", step, maxSteps, active.Name, active.Tools, budget)
						}
					}
				}
			} else {
				content = executeTool(ctx, selected, call)
			}
			thread.Add(Message{Role: RoleTool, Content: content, ToolCallID: call.ID, Name: call.Name})
			h.debugToolResult(step, maxSteps, call, content)
			h.debugf("step=%d/%d event=tool_finish tool=%q call_id=%q", step, maxSteps, call.Name, call.ID)
		}
	}
	h.debugf("step=%d/%d event=max_steps", maxSteps, maxSteps)
	return nil, ErrMaxSteps
}

type debugMessage struct {
	Role       Role               `json:"role"`
	Content    string             `json:"content,omitempty"`
	ToolCalls  []debugToolCallRef `json:"tool_calls,omitempty"`
	ToolCallID string             `json:"tool_call_id,omitempty"`
	Name       string             `json:"name,omitempty"`
}

type debugToolCallRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// compactDebugMessages removes repeated payloads already logged elsewhere.
// @param messages newly added conversation messages.
// @return compact message summaries.
func compactDebugMessages(messages []Message) []debugMessage {
	result := make([]debugMessage, 0, len(messages))
	for _, message := range messages {
		item := debugMessage{
			Role:       message.Role,
			ToolCallID: message.ToolCallID,
			Name:       message.Name,
		}
		if message.Role == RoleSystem || message.Role == RoleUser {
			item.Content = message.Content
		}
		for _, call := range message.ToolCalls {
			item.ToolCalls = append(item.ToolCalls, debugToolCallRef{ID: call.ID, Name: call.Name})
		}
		result = append(result, item)
	}
	return result
}

// toolDefinitionNames returns model-visible tool names.
// @param definitions available tool definitions.
// @return names in request order.
func toolDefinitionNames(definitions []ToolDefinition) []string {
	names := make([]string, len(definitions))
	for i, definition := range definitions {
		names[i] = definition.Name
	}
	return names
}

// debugPreview truncates verbose text while preserving valid UTF-8.
// @param value text to summarize.
// @return original or truncated text.
func debugPreview(value string) string {
	runes := []rune(value)
	if len(runes) <= debugPreviewLimit {
		return value
	}
	return string(runes[:debugPreviewLimit]) + fmt.Sprintf("… [truncated %d chars]", len(runes)-debugPreviewLimit)
}

// debugf writes one debug event when logging is enabled.
// @param format printf-style event format.
// @param args event values.
// @return none.
func (h *Harness) debugf(format string, args ...any) {
	if h.debug != nil {
		h.debug.Printf(format, args...)
	}
}

// debugJSON writes one pretty-printed debug payload.
// @param step current model step.
// @param maxSteps current model-step limit.
// @param event debug event name.
// @param value payload to encode.
// @return none.
func (h *Harness) debugJSON(step, maxSteps int, event string, value any) {
	if h.debug == nil {
		return
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		h.debug.Printf("step=%d/%d event=%s encode_error=%q", step, maxSteps, event, err)
		return
	}
	h.debug.Printf("step=%d/%d event=%s\n%s", step, maxSteps, event, encoded)
}

// debugToolResult writes one decoded tool result payload.
// @param step current model step.
// @param maxSteps current model-step limit.
// @param call completed tool call.
// @param content serialized tool result.
// @return none.
func (h *Harness) debugToolResult(step, maxSteps int, call ToolCall, content string) {
	if call.Name == skillToolName {
		var activation skillActivation
		if json.Unmarshal([]byte(content), &activation) == nil && activation.Name != "" {
			h.debugf("step=%d/%d event=skill_result skill=%q allowed_tools=%q max_steps=%d", step, maxSteps, activation.Name, activation.Tools, activation.MaxSteps)
			return
		}
	}
	h.debugf("step=%d/%d event=tool_result tool=%q call_id=%q result=%s", step, maxSteps, call.Name, call.ID, debugPreview(content))
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

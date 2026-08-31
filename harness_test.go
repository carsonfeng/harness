package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	corethread "github.com/carsonfeng/harness/thread"
)

type mcpServerFunc func(context.Context) ([]Tool, error)

// Tools invokes a test MCP discovery function.
// @param ctx discovery cancellation context.
// @return discovered tools or error.
func (f mcpServerFunc) Tools(ctx context.Context) ([]Tool, error) { return f(ctx) }

// TestMCPRegistersDiscoveredTools verifies MCP registration in the root API.
// @param t test state.
// @return none.
func TestMCPRegistersDiscoveredTools(t *testing.T) {
	type empty struct{}
	server := mcpServerFunc(func(context.Context) ([]Tool, error) {
		return []Tool{Func("remote", "Remote tool", func(context.Context, empty) (string, error) {
			return "ok", nil
		})}, nil
	})
	h := New()
	if err := h.MCP(context.Background(), server); err != nil {
		t.Fatal(err)
	}
	if len(h.tools.tools) != 1 || h.tools.tools["remote"] == nil {
		t.Fatalf("registered tools = %#v", h.tools.tools)
	}
}

// TestRunToolLoop verifies a complete model-tool-model cycle.
// @param t test state.
// @return none.
func TestRunToolLoop(t *testing.T) {
	requests := 0
	model := ModelFunc(func(_ context.Context, req ModelRequest) (ModelResponse, error) {
		requests++
		if requests == 1 {
			return ModelResponse{ToolCalls: []ToolCall{{ID: "call-1", Name: "greet", Arguments: json.RawMessage(`{"name":"Go"}`)}}}, nil
		}
		last := req.Messages[len(req.Messages)-1]
		if last.Role != RoleTool || last.Content != `"Hello, Go"` {
			t.Fatalf("unexpected tool message: %#v", last)
		}
		return ModelResponse{Text: "Done"}, nil
	})
	type args struct {
		Name string `json:"name"`
	}
	h := New(WithModel(model), WithTools(Func("greet", "", func(_ context.Context, in args) (string, error) {
		return "Hello, " + in.Name, nil
	})))
	result, err := h.Run(context.Background(), "Say hello")
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "Done" || result.Steps != 2 || requests != 2 {
		t.Fatalf("result = %#v, requests = %d", result, requests)
	}
}

// TestToolErrorReturnsToModel verifies recoverable tool failures.
// @param t test state.
// @return none.
func TestToolErrorReturnsToModel(t *testing.T) {
	turn := 0
	model := ModelFunc(func(_ context.Context, req ModelRequest) (ModelResponse, error) {
		turn++
		if turn == 1 {
			return ModelResponse{ToolCalls: []ToolCall{{ID: "1", Name: "fail", Arguments: json.RawMessage(`{}`)}}}, nil
		}
		var got map[string]string
		if err := json.Unmarshal([]byte(req.Messages[len(req.Messages)-1].Content), &got); err != nil || got["error"] != "boom" {
			t.Fatalf("tool error = %#v, decode error = %v", got, err)
		}
		return ModelResponse{Text: "recovered"}, nil
	})
	type empty struct{}
	h := New(WithModel(model), WithTools(Func("fail", "", func(context.Context, empty) (any, error) {
		return nil, errors.New("boom")
	})))
	if _, err := h.Run(context.Background(), "go"); err != nil {
		t.Fatal(err)
	}
}

// TestMaxSteps verifies loop limit enforcement.
// @param t test state.
// @return none.
func TestMaxSteps(t *testing.T) {
	model := ModelFunc(func(context.Context, ModelRequest) (ModelResponse, error) {
		return ModelResponse{ToolCalls: []ToolCall{{ID: "1", Name: "noop", Arguments: json.RawMessage(`{}`)}}}, nil
	})
	type empty struct{}
	h := New(WithModel(model), WithMaxSteps(2), WithTools(Func("noop", "", func(context.Context, empty) (string, error) { return "ok", nil })))
	if _, err := h.Run(context.Background(), "go"); !errors.Is(err, ErrMaxSteps) {
		t.Fatalf("error = %v", err)
	}
}

// TestDebugLogsLoopProgress verifies incremental messages and tool payloads.
// @param t test state.
// @return none.
func TestDebugLogsLoopProgress(t *testing.T) {
	var logs bytes.Buffer
	turn := 0
	model := ModelFunc(func(context.Context, ModelRequest) (ModelResponse, error) {
		turn++
		if turn == 1 {
			return ModelResponse{ToolCalls: []ToolCall{{ID: "call-1", Name: "inspect", Arguments: json.RawMessage(`{"token":"tool-argument-value"}`)}}}, nil
		}
		if turn == 2 {
			return ModelResponse{Text: "first-answer-value"}, nil
		}
		return ModelResponse{Text: "second-answer-value"}, nil
	})
	type args struct {
		Token string `json:"token"`
	}
	agent := New(
		WithModel(model),
		WithDebug(&logs),
		WithTools(Func("inspect", "Inspect input", func(context.Context, args) (string, error) {
			return "tool-result-value", nil
		})),
	)
	first, err := agent.Run(context.Background(), "first-prompt-value")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := agent.RunThread(context.Background(), first.Thread, "second-prompt-value"); err != nil {
		t.Fatal(err)
	}
	output := logs.String()
	for _, want := range []string{
		"step=1/20 event=model_request",
		"step=1/20 event=model_request_delta",
		`tools=["inspect"]`,
		"step=1/20 event=model_response tool_calls=1 final=false text=\"\"",
		`event=tool_call tool="inspect" call_id="call-1"`,
		`event=tool_start tool="inspect" call_id="call-1"`,
		"step=1/20 event=tool_arguments",
		`event=tool_result tool="inspect" call_id="call-1" result="tool-result-value"`,
		"step=2/20 event=model_request",
		"step=2/20 event=complete",
		"first-prompt-value",
		"second-prompt-value",
		"tool-argument-value",
		"tool-result-value",
		"first-answer-value",
		"second-answer-value",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("debug output missing %q:\n%s", want, output)
		}
	}
	for _, value := range []string{"first-prompt-value", "second-prompt-value", "tool-argument-value", "tool-result-value", "first-answer-value", "second-answer-value"} {
		if count := strings.Count(output, value); count != 1 {
			t.Fatalf("debug output contains %q %d times, want 1:\n%s", value, count, output)
		}
	}
}

// TestRunAutomaticallyLoadsSkillAndPersistsPolicy verifies skill discovery.
// @param t test state.
// @return none.
func TestRunAutomaticallyLoadsSkillAndPersistsPolicy(t *testing.T) {
	turn := 0
	var logs bytes.Buffer
	model := ModelFunc(func(_ context.Context, req ModelRequest) (ModelResponse, error) {
		turn++
		switch turn {
		case 1:
			if len(req.Tools) != 3 || req.Tools[0].Name != "allowed" || req.Tools[1].Name != "hidden" || req.Tools[2].Name != skillToolName {
				t.Fatalf("discovery tools = %#v", req.Tools)
			}
			if !strings.Contains(req.Tools[2].Description, "review: Review code") || strings.Contains(req.Tools[2].Description, "review carefully") {
				t.Fatalf("skill discovery leaked or omitted metadata: %q", req.Tools[2].Description)
			}
			if len(req.Messages) != 2 || req.Messages[0].Content != "base" {
				t.Fatalf("initial messages = %#v", req.Messages)
			}
			return ModelResponse{ToolCalls: []ToolCall{{ID: "skill-1", Name: skillToolName, Arguments: json.RawMessage(`{"name":"review"}`)}}}, nil
		case 2:
			if len(req.Tools) != 1 || req.Tools[0].Name != "allowed" {
				t.Fatalf("skill tools = %#v", req.Tools)
			}
			last := req.Messages[len(req.Messages)-1]
			var activation skillActivation
			if last.Name != skillToolName || json.Unmarshal([]byte(last.Content), &activation) != nil || activation.Instructions != "review carefully" || len(activation.Tools) != 1 || activation.Tools[0] != "allowed" || activation.MaxSteps != 2 {
				t.Fatalf("skill activation = %#v", last)
			}
			return ModelResponse{ToolCalls: []ToolCall{{ID: "allowed-1", Name: "allowed", Arguments: json.RawMessage(`{}`)}}}, nil
		case 3:
			return ModelResponse{Text: "reviewed"}, nil
		case 4:
			if len(req.Tools) != 1 || req.Tools[0].Name != "allowed" {
				t.Fatalf("continued tools = %#v", req.Tools)
			}
			return ModelResponse{Text: "continued"}, nil
		default:
			t.Fatalf("unexpected model turn %d", turn)
		}
		return ModelResponse{}, nil
	})
	type empty struct{}
	h := New(
		WithModel(model),
		WithDebug(&logs),
		WithSystem("base"),
		WithTools(
			Func("allowed", "", func(context.Context, empty) (string, error) { return "ok", nil }),
			Func("hidden", "", func(context.Context, empty) (string, error) { return "ok", nil }),
		),
	)
	if err := h.Skill(Skill{Name: "review", Description: "Review code", Instructions: "review carefully", Tools: []string{"allowed"}, MaxSteps: 2}); err != nil {
		t.Fatal(err)
	}
	thread := NewThread()
	result, err := h.RunThread(context.Background(), thread, "review this change")
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "reviewed" || result.Steps != 3 {
		t.Fatalf("result = %#v", result)
	}
	if err := h.Skill(Skill{Name: "review", Description: "Changed", Instructions: "new policy", Tools: []string{"hidden"}, MaxSteps: 1}); err != nil {
		t.Fatal(err)
	}
	continued, err := h.RunThread(context.Background(), thread, "check one more file")
	if err != nil {
		t.Fatal(err)
	}
	if continued.Text != "continued" || continued.Steps != 1 {
		t.Fatalf("continued = %#v", continued)
	}
	output := logs.String()
	for _, want := range []string{
		`event=tool_call tool="load_skill" call_id="skill-1"`,
		`"name": "review"`,
		`event=skill_activated skill="review" allowed_tools=["allowed"] budget=2`,
		`event=skill_result skill="review" allowed_tools=["allowed"] max_steps=2`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("skill debug output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "review carefully") {
		t.Fatalf("skill debug output leaked instructions:\n%s", output)
	}
}

// TestSkillLoadFailureCanBeRetried verifies model-visible selection errors.
// @param t test state.
// @return none.
func TestSkillLoadFailureCanBeRetried(t *testing.T) {
	turn := 0
	model := ModelFunc(func(_ context.Context, req ModelRequest) (ModelResponse, error) {
		turn++
		last := req.Messages[len(req.Messages)-1]
		switch turn {
		case 1:
			return ModelResponse{ToolCalls: []ToolCall{{ID: "bad", Name: skillToolName, Arguments: json.RawMessage(`{"name":"missing"}`)}}}, nil
		case 2:
			if last.Role != RoleTool || last.Name != skillToolName || !strings.Contains(last.Content, "unknown skill") {
				t.Fatalf("selection error = %#v", last)
			}
			if len(req.Tools) != 1 || req.Tools[0].Name != skillToolName {
				t.Fatalf("retry tools = %#v", req.Tools)
			}
			return ModelResponse{ToolCalls: []ToolCall{{ID: "good", Name: skillToolName, Arguments: json.RawMessage(`{"name":"review"}`)}}}, nil
		case 3:
			if last.Role != RoleTool || last.Name != skillToolName || !strings.Contains(last.Content, "Review carefully") {
				t.Fatalf("activation result = %#v", last)
			}
			return ModelResponse{Text: "done"}, nil
		default:
			t.Fatalf("unexpected model turn %d", turn)
			return ModelResponse{}, nil
		}
	})
	agent := New(WithModel(model))
	if err := agent.Skill(Skill{Name: "review", Description: "Review code", Instructions: "Review carefully"}); err != nil {
		t.Fatal(err)
	}
	result, err := agent.Run(context.Background(), "review this")
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "done" || result.Steps != 3 {
		t.Fatalf("result = %#v", result)
	}
}

// TestSkillLoadReceivesDefaultBudget verifies selection is not charged to a skill.
// @param t test state.
// @return none.
func TestSkillLoadReceivesDefaultBudget(t *testing.T) {
	turn := 0
	model := ModelFunc(func(_ context.Context, req ModelRequest) (ModelResponse, error) {
		turn++
		if turn == 1 {
			return ModelResponse{ToolCalls: []ToolCall{{ID: "skill-1", Name: skillToolName, Arguments: json.RawMessage(`{"name":"review"}`)}}}, nil
		}
		if len(req.Tools) != 0 {
			t.Fatalf("tools after activation = %#v", req.Tools)
		}
		return ModelResponse{Text: "done"}, nil
	})
	agent := New(WithModel(model), WithMaxSteps(1))
	if err := agent.Skill(Skill{Name: "review", Description: "Review code", Instructions: "Review carefully"}); err != nil {
		t.Fatal(err)
	}
	result, err := agent.Run(context.Background(), "review this")
	if err != nil {
		t.Fatal(err)
	}
	if result.Steps != 2 {
		t.Fatalf("steps = %d, want 2", result.Steps)
	}
}

// TestSkillRegistrationValidatesAndCopiesConfig verifies safe configuration.
// @param t test state.
// @return none.
func TestSkillRegistrationValidatesAndCopiesConfig(t *testing.T) {
	agent := New()
	if err := agent.Skill(Skill{Name: "review", MaxSteps: -1}); err == nil {
		t.Fatal("negative max steps were accepted")
	}
	if err := agent.Skill(Skill{Name: "review", Tools: []string{"not valid"}}); err == nil {
		t.Fatal("invalid tool name was accepted")
	}
	tools := []string{"read"}
	if err := agent.Skill(Skill{Name: "review", Tools: tools}); err != nil {
		t.Fatal(err)
	}
	tools[0] = "mutated"
	if got := agent.skillSnapshot()["review"].Tools[0]; got != "read" {
		t.Fatalf("stored tool = %q", got)
	}
}

// TestSkillLoadMustBeCalledAlone prevents sibling tool execution.
// @param t test state.
// @return none.
func TestSkillLoadMustBeCalledAlone(t *testing.T) {
	executed := false
	turn := 0
	model := ModelFunc(func(_ context.Context, req ModelRequest) (ModelResponse, error) {
		turn++
		if turn == 1 {
			return ModelResponse{ToolCalls: []ToolCall{
				{ID: "hidden-1", Name: "hidden", Arguments: json.RawMessage(`{}`)},
				{ID: "skill-1", Name: skillToolName, Arguments: json.RawMessage(`{"name":"review"}`)},
			}}, nil
		}
		messages := req.Messages
		if len(messages) < 2 || !strings.Contains(messages[len(messages)-1].Content, "only tool call") || !strings.Contains(messages[len(messages)-2].Content, "only tool call") {
			t.Fatalf("mixed-call results = %#v", messages)
		}
		return ModelResponse{Text: "recovered"}, nil
	})
	type empty struct{}
	agent := New(WithModel(model), WithTools(Func("hidden", "Hidden tool", func(context.Context, empty) (string, error) {
		executed = true
		return "unexpected", nil
	})))
	if err := agent.Skill(Skill{Name: "review", Description: "Review code", Instructions: "Review carefully"}); err != nil {
		t.Fatal(err)
	}
	if _, err := agent.Run(context.Background(), "review this"); err != nil {
		t.Fatal(err)
	}
	if executed {
		t.Fatal("sibling tool executed before skill activation")
	}
}

// TestSkillActivationRequiresMatchingCall verifies history validation.
// @param t test state.
// @return none.
func TestSkillActivationRequiresMatchingCall(t *testing.T) {
	encoded, err := json.Marshal(skillActivation{Name: "review", Instructions: "Review carefully", Tools: []string{"read"}})
	if err != nil {
		t.Fatal(err)
	}
	messages := []Message{
		{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "skill-1", Name: skillToolName}}},
		{Role: RoleUser, Content: "unrelated boundary"},
		{Role: RoleTool, Name: skillToolName, ToolCallID: "skill-1", Content: string(encoded)},
	}
	active, err := activeSkill(messages)
	if err != nil {
		t.Fatal(err)
	}
	if active != nil {
		t.Fatalf("unmatched activation = %#v", active)
	}
	mismatched := []Message{
		{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "skill-1", Name: skillToolName, Arguments: json.RawMessage(`{"name":"security"}`)}}},
		{Role: RoleTool, Name: skillToolName, ToolCallID: "skill-1", Content: string(encoded)},
	}
	if _, err := activeSkill(mismatched); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("mismatched activation error = %v", err)
	}
}

// TestInvalidPersistedSkillDoesNotMutateThread verifies policy preflight.
// @param t test state.
// @return none.
func TestInvalidPersistedSkillDoesNotMutateThread(t *testing.T) {
	encoded, err := json.Marshal(skillActivation{Name: "review", Instructions: "Review carefully", Tools: []string{"missing"}})
	if err != nil {
		t.Fatal(err)
	}
	thread := NewThread(
		Message{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "skill-1", Name: skillToolName, Arguments: json.RawMessage(`{"name":"review"}`)}}},
		Message{Role: RoleTool, Name: skillToolName, ToolCallID: "skill-1", Content: string(encoded)},
	)
	before := thread.Messages()
	agent := New(WithModel(ModelFunc(func(context.Context, ModelRequest) (ModelResponse, error) {
		t.Fatal("model must not be called")
		return ModelResponse{}, nil
	})))
	if _, err := agent.RunThread(context.Background(), thread, "new turn"); err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Fatalf("policy error = %v", err)
	}
	if got := thread.Messages(); len(got) != len(before) {
		t.Fatalf("thread mutated: before = %#v, after = %#v", before, got)
	}
}

// TestRunWithoutSkillsDoesNotExposeLoader verifies opt-in discovery.
// @param t test state.
// @return none.
func TestRunWithoutSkillsDoesNotExposeLoader(t *testing.T) {
	model := ModelFunc(func(_ context.Context, req ModelRequest) (ModelResponse, error) {
		if len(req.Tools) != 0 {
			t.Fatalf("unexpected tools = %#v", req.Tools)
		}
		return ModelResponse{Text: "done"}, nil
	})
	if _, err := New(WithModel(model)).Run(context.Background(), "hello"); err != nil {
		t.Fatal(err)
	}
}

// TestRunThreadContinuesUserAndToolTurns verifies reusable conversation state.
// @param t test state.
// @return none.
func TestRunThreadContinuesUserAndToolTurns(t *testing.T) {
	modelCalls := 0
	model := ModelFunc(func(_ context.Context, req ModelRequest) (ModelResponse, error) {
		modelCalls++
		last := req.Messages[len(req.Messages)-1]
		if last.Role == RoleUser {
			if modelCalls == 3 {
				foundPreviousAnswer := false
				for _, message := range req.Messages {
					if message.Role == RoleAssistant && message.Content == `answer:"Guangzhou"` {
						foundPreviousAnswer = true
					}
				}
				if !foundPreviousAnswer {
					t.Fatal("second turn did not include the first answer")
				}
			}
			arguments, err := json.Marshal(map[string]string{"city": last.Content})
			if err != nil {
				t.Fatal(err)
			}
			return ModelResponse{ToolCalls: []ToolCall{{ID: last.Content, Name: "weather", Arguments: arguments}}}, nil
		}
		if last.Role != RoleTool {
			t.Fatalf("last message = %#v", last)
		}
		return ModelResponse{Text: "answer:" + last.Content}, nil
	})
	type weatherArgs struct {
		City string `json:"city"`
	}
	agent := New(
		WithModel(model),
		WithSystem("Be concise"),
		WithTools(Func("weather", "", func(_ context.Context, args weatherArgs) (string, error) {
			return args.City, nil
		})),
	)
	thread := NewThread()
	first, err := agent.RunThread(context.Background(), thread, "Guangzhou")
	if err != nil {
		t.Fatal(err)
	}
	second, err := agent.RunThread(context.Background(), thread, "Shenzhen")
	if err != nil {
		t.Fatal(err)
	}
	if first.Thread != thread || second.Thread != thread || first.Steps != 2 || second.Steps != 2 || second.Text != `answer:"Shenzhen"` {
		t.Fatalf("first = %#v, second = %#v", first, second)
	}
	messages := thread.Messages()
	if len(messages) != 9 || messages[0].Role != RoleSystem {
		t.Fatalf("messages = %#v", messages)
	}
	wantRoles := []Role{RoleSystem, RoleUser, RoleAssistant, RoleTool, RoleAssistant, RoleUser, RoleAssistant, RoleTool, RoleAssistant}
	for i, want := range wantRoles {
		if messages[i].Role != want {
			t.Fatalf("message %d role = %q, want %q", i, messages[i].Role, want)
		}
	}
	if messages[2].ToolCalls[0].ID != messages[3].ToolCallID || messages[6].ToolCalls[0].ID != messages[7].ToolCallID {
		t.Fatalf("tool call IDs were not preserved: %#v", messages)
	}
}

// TestRunThreadRejectsNilAndOverlappingRuns verifies thread ownership.
// @param t test state.
// @return none.
func TestRunThreadRejectsNilAndOverlappingRuns(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	model := ModelFunc(func(context.Context, ModelRequest) (ModelResponse, error) {
		once.Do(func() { close(started) })
		<-release
		return ModelResponse{Text: "done"}, nil
	})
	agent := New(WithModel(model))
	if _, err := agent.RunThread(context.Background(), nil, "input"); !errors.Is(err, ErrNilThread) {
		t.Fatalf("nil thread error = %v", err)
	}
	thread := NewThread()
	done := make(chan error, 1)
	go func() {
		_, err := agent.RunThread(context.Background(), thread, "first")
		done <- err
	}()
	<-started
	if _, err := agent.RunThread(context.Background(), thread, "second"); !errors.Is(err, ErrThreadBusy) {
		t.Fatalf("overlapping run error = %v", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// TestReservedSkillToolName protects the internal discovery tool.
// @param t test state.
// @return none.
func TestReservedSkillToolName(t *testing.T) {
	type empty struct{}
	agent := New()
	err := agent.Tool(Func(skillToolName, "reserved", func(context.Context, empty) (string, error) {
		return "", nil
	}))
	if !errors.Is(err, ErrReservedToolName) {
		t.Fatalf("reserved name error = %v", err)
	}
}

// TestRunThreadCanceledContextDoesNotMutate verifies preflight cancellation.
// @param t test state.
// @return none.
func TestRunThreadCanceledContextDoesNotMutate(t *testing.T) {
	modelCalled := false
	model := ModelFunc(func(context.Context, ModelRequest) (ModelResponse, error) {
		modelCalled = true
		return ModelResponse{}, nil
	})
	agent := New(WithModel(model), WithSystem("system"))
	thread := NewThread(Message{Role: RoleUser, Content: "existing"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := agent.RunThread(ctx, thread, "new"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled run error = %v", err)
	}
	if modelCalled || len(thread.Messages()) != 1 {
		t.Fatalf("modelCalled = %v, messages = %#v", modelCalled, thread.Messages())
	}
}

// TestThreadSubpackageCompatibility verifies the root type remains an alias.
// @param t test state.
// @return none.
func TestThreadSubpackageCompatibility(t *testing.T) {
	var rootThread *Thread = corethread.New()
	if rootThread == nil {
		t.Fatal("thread alias is nil")
	}
}

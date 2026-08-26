package harness

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

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

// TestRunSkillAppliesInstructionsAndToolAllowlist verifies skill policy.
// @param t test state.
// @return none.
func TestRunSkillAppliesInstructionsAndToolAllowlist(t *testing.T) {
	model := ModelFunc(func(_ context.Context, req ModelRequest) (ModelResponse, error) {
		if len(req.Tools) != 1 || req.Tools[0].Name != "allowed" {
			t.Fatalf("tools = %#v", req.Tools)
		}
		if len(req.Messages) != 2 || req.Messages[0].Role != RoleSystem || req.Messages[0].Content != "base\n\nreview carefully" {
			t.Fatalf("messages = %#v", req.Messages)
		}
		return ModelResponse{Text: "ok"}, nil
	})
	type empty struct{}
	h := New(
		WithModel(model),
		WithSystem("base"),
		WithTools(
			Func("allowed", "", func(context.Context, empty) (string, error) { return "ok", nil }),
			Func("hidden", "", func(context.Context, empty) (string, error) { return "ok", nil }),
		),
	)
	if err := h.Skill(Skill{Name: "review", Instructions: "review carefully", Tools: []string{"allowed"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.RunSkill(context.Background(), "review", "input"); err != nil {
		t.Fatal(err)
	}
}

package tool

import (
	"context"
	"encoding/json"
	"testing"
)

// TestFuncSchemaAndExecution verifies typed schema and invocation.
// @param t test state.
// @return none.
func TestFuncSchemaAndExecution(t *testing.T) {
	type input struct {
		City string `json:"city" description:"City name"`
		Unit string `json:"unit,omitempty"`
	}
	tool := Func("weather", "Get weather", func(_ context.Context, in input) (string, error) {
		return in.City + ": 24C", nil
	})
	var schema map[string]any
	if err := json.Unmarshal(tool.Definition().Parameters, &schema); err != nil {
		t.Fatal(err)
	}
	required := schema["required"].([]any)
	if len(required) != 1 || required[0] != "city" {
		t.Fatalf("required = %#v", required)
	}
	got, err := tool.Execute(context.Background(), json.RawMessage(`{"city":"Guangzhou"}`))
	if err != nil {
		t.Fatal(err)
	}
	if got != "Guangzhou: 24C" {
		t.Fatalf("result = %v", got)
	}
}

// TestFuncRejectsUnknownArguments verifies strict argument decoding.
// @param t test state.
// @return none.
func TestFuncRejectsUnknownArguments(t *testing.T) {
	type input struct {
		City string `json:"city"`
	}
	tool := Func("weather", "", func(_ context.Context, in input) (input, error) { return in, nil })
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"city":"GZ","typo":true}`)); err == nil {
		t.Fatal("expected unknown field error")
	}
}

// Package tool turns typed Go functions into model-callable capabilities.
package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/carsonfeng/harness/model"
)

// Tool is a capability that a model can invoke.
type Tool interface {
	// Definition returns the model-visible tool schema.
	// @return tool definition.
	Definition() model.ToolDefinition

	// Execute invokes the tool with JSON arguments.
	// @param ctx tool cancellation context.
	// @param arguments JSON arguments.
	// @return tool result or execution error.
	Execute(context.Context, json.RawMessage) (any, error)
}

type function[In, Out any] struct {
	definition model.ToolDefinition
	fn         func(context.Context, In) (Out, error)
}

// Func exposes a typed Go function as a model tool. Fields tagged with
// omitempty are optional; a description tag documents a schema property.
// @param name model-visible tool name.
// @param description model-visible purpose.
// @param fn typed Go function.
// @return callable tool.
func Func[In, Out any](name, description string, fn func(context.Context, In) (Out, error)) Tool {
	if fn == nil {
		panic("harness: nil tool function")
	}
	schema, err := schemaFor(reflect.TypeOf((*In)(nil)).Elem())
	if err != nil {
		panic("harness: tool " + name + ": " + err.Error())
	}
	encoded, err := json.Marshal(schema)
	if err != nil {
		panic("harness: encode tool schema: " + err.Error())
	}
	return &function[In, Out]{
		definition: model.ToolDefinition{Name: name, Description: description, Parameters: encoded},
		fn:         fn,
	}
}

// Definition returns the model-visible tool schema.
// @param t function tool.
// @return tool definition.
func (t *function[In, Out]) Definition() model.ToolDefinition { return t.definition }

// Execute decodes arguments and invokes the typed function.
// @param ctx tool cancellation context.
// @param raw JSON arguments.
// @return function result or decoding/execution error.
func (t *function[In, Out]) Execute(ctx context.Context, raw json.RawMessage) (any, error) {
	var input In
	if len(raw) == 0 {
		raw = json.RawMessage("{}")
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return nil, fmt.Errorf("decode arguments: %w", err)
	}
	return t.fn(ctx, input)
}

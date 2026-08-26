// Package model defines the provider-neutral boundary used by Harness.
package model

import (
	"context"
	"encoding/json"
)

// Role identifies the author of a message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is one provider-neutral conversation item.
type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// ToolCall asks the host application to invoke a named tool.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolDefinition describes an available function to a model provider.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

// Request contains the complete conversation and available tools.
type Request struct {
	Messages []Message        `json:"messages"`
	Tools    []ToolDefinition `json:"tools,omitempty"`
}

// Response is one assistant turn. ToolCalls may contain multiple calls.
type Response struct {
	Text      string     `json:"text,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// Model adapts any chat model provider to Harness.
type Model interface {
	// Generate produces one assistant turn.
	// @param ctx request cancellation context.
	// @param req conversation and available tools.
	// @return assistant response or provider error.
	Generate(context.Context, Request) (Response, error)
}

// Func turns a function into a Model, primarily for tests and small adapters.
// @param ctx request cancellation context.
// @param req conversation and available tools.
// @return assistant response or provider error.
type Func func(context.Context, Request) (Response, error)

// Generate invokes the wrapped function.
// @param ctx request cancellation context.
// @param req model request.
// @return model response or error.
func (f Func) Generate(ctx context.Context, req Request) (Response, error) {
	return f(ctx, req)
}

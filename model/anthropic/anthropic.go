// Package anthropic adapts the Anthropic Messages API.
package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/carsonfeng/harness/internal/httpjson"
	"github.com/carsonfeng/harness/model"
)

const (
	defaultBaseURL   = "https://api.anthropic.com/v1"
	defaultVersion   = "2023-06-01"
	defaultMaxTokens = 4096
)

// Config configures an Anthropic-compatible Messages endpoint.
type Config struct {
	APIKey     string
	Model      string
	BaseURL    string
	Version    string
	MaxTokens  int
	HTTPClient *http.Client
	Headers    map[string]string
}

// Messages implements model.Model with POST /messages.
type Messages struct{ config Config }

// New creates an Anthropic Messages adapter.
// @param config endpoint, credentials, model, and generation limits.
// @return configured model adapter.
func New(config Config) *Messages { return &Messages{config: normalize(config)} }

// Generate performs one Anthropic Messages turn.
// @param ctx request cancellation context.
// @param req provider-neutral messages and tools.
// @return assistant response or provider error.
func (c *Messages) Generate(ctx context.Context, req model.Request) (model.Response, error) {
	if err := validate(c.config); err != nil {
		return model.Response{}, err
	}
	system, messages := convertMessages(req.Messages)
	input := request{Model: c.config.Model, MaxTokens: c.config.MaxTokens, System: system, Messages: messages, Tools: convertTools(req.Tools)}
	var output response
	if err := httpjson.Post(ctx, c.config.HTTPClient, httpjson.Endpoint(c.config.BaseURL, "messages"), headers(c.config), input, &output); err != nil {
		return model.Response{}, err
	}
	if output.Error != nil {
		return model.Response{}, errors.New(output.Error.Message)
	}
	result := model.Response{}
	var texts []string
	for _, block := range output.Content {
		switch block.Type {
		case "text":
			texts = append(texts, block.Text)
		case "tool_use":
			result.ToolCalls = append(result.ToolCalls, model.ToolCall{ID: block.ID, Name: block.Name, Arguments: block.Input})
		}
	}
	result.Text = strings.Join(texts, "")
	return result, nil
}

type request struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system,omitempty"`
	Messages  []message `json:"messages"`
	Tools     []tool    `json:"tools,omitempty"`
}

type message struct {
	Role    string    `json:"role"`
	Content []content `json:"content"`
}
type content struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}
type tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}
type response struct {
	Content []content `json:"content"`
	Error   *apiError `json:"error,omitempty"`
}
type apiError struct {
	Message string `json:"message"`
}

// normalize applies protocol defaults and copies mutable maps.
// @param config caller-supplied configuration.
// @return normalized configuration.
func normalize(config Config) Config {
	if config.BaseURL == "" {
		config.BaseURL = defaultBaseURL
	}
	if config.Version == "" {
		config.Version = defaultVersion
	}
	if config.MaxTokens == 0 {
		config.MaxTokens = defaultMaxTokens
	}
	copy := make(map[string]string, len(config.Headers)+2)
	for name, value := range config.Headers {
		copy[name] = value
	}
	config.Headers = copy
	return config
}

// validate checks values required before an API call.
// @param config normalized configuration.
// @return configuration error or nil.
func validate(config Config) error {
	if config.Model == "" {
		return errors.New("anthropic: model is required")
	}
	if config.BaseURL == "" {
		return errors.New("anthropic: base URL is required")
	}
	if config.MaxTokens < 1 {
		return errors.New("anthropic: max tokens must be positive")
	}
	return nil
}

// headers builds Anthropic authentication and version headers.
// @param config provider configuration.
// @return request headers.
func headers(config Config) map[string]string {
	result := make(map[string]string, len(config.Headers)+2)
	for name, value := range config.Headers {
		result[name] = value
	}
	if config.APIKey != "" {
		result["x-api-key"] = config.APIKey
	}
	result["anthropic-version"] = config.Version
	return result
}

// convertMessages converts system, text, tool-use, and tool-result history.
// @param source provider-neutral messages.
// @return top-level system text and Anthropic messages.
func convertMessages(source []model.Message) (string, []message) {
	var systems []string
	var result []message
	for i := 0; i < len(source); {
		current := source[i]
		if current.Role == model.RoleSystem {
			if current.Content != "" {
				systems = append(systems, current.Content)
			}
			i++
			continue
		}
		if current.Role == model.RoleTool {
			item := message{Role: "user"}
			for i < len(source) && source[i].Role == model.RoleTool {
				toolResult := source[i]
				item.Content = append(item.Content, content{Type: "tool_result", ToolUseID: toolResult.ToolCallID, Content: toolResult.Content, IsError: isToolError(toolResult.Content)})
				i++
			}
			result = append(result, item)
			continue
		}
		item := message{Role: string(current.Role)}
		if current.Content != "" {
			item.Content = append(item.Content, content{Type: "text", Text: current.Content})
		}
		for _, call := range current.ToolCalls {
			item.Content = append(item.Content, content{Type: "tool_use", ID: call.ID, Name: call.Name, Input: call.Arguments})
		}
		result = append(result, item)
		i++
	}
	return strings.Join(systems, "\n\n"), result
}

// convertTools converts JSON Schema definitions to Anthropic tools.
// @param tools provider-neutral definitions.
// @return Anthropic tool definitions.
func convertTools(tools []model.ToolDefinition) []tool {
	result := make([]tool, 0, len(tools))
	for _, definition := range tools {
		result = append(result, tool{Name: definition.Name, Description: definition.Description, InputSchema: definition.Parameters})
	}
	return result
}

// isToolError recognizes Harness error results.
// @param value serialized tool output.
// @return true when output contains a top-level error string.
func isToolError(value string) bool {
	var body map[string]any
	if json.Unmarshal([]byte(value), &body) != nil {
		return false
	}
	_, ok := body["error"].(string)
	return ok
}

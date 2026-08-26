// Package openai adapts OpenAI-style Chat Completions and Responses APIs.
package openai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/carsonfeng/harness/internal/httpjson"
	"github.com/carsonfeng/harness/model"
)

const defaultBaseURL = "https://api.openai.com/v1"

// Config configures an OpenAI-compatible endpoint.
type Config struct {
	APIKey     string
	Model      string
	BaseURL    string
	HTTPClient *http.Client
	Headers    map[string]string
}

// ChatCompletions implements model.Model with POST /chat/completions.
type ChatCompletions struct{ config Config }

// NewChatCompletions creates a Chat Completions adapter.
// @param config endpoint, credentials, model, and HTTP options.
// @return configured model adapter.
func NewChatCompletions(config Config) *ChatCompletions {
	return &ChatCompletions{config: normalize(config)}
}

// Generate performs one Chat Completions turn.
// @param ctx request cancellation context.
// @param req provider-neutral messages and tools.
// @return assistant response or provider error.
func (c *ChatCompletions) Generate(ctx context.Context, req model.Request) (model.Response, error) {
	if err := validate(c.config); err != nil {
		return model.Response{}, err
	}
	input := chatRequest{Model: c.config.Model, Messages: chatMessages(req.Messages), Tools: chatTools(req.Tools)}
	var output chatResponse
	if err := httpjson.Post(ctx, c.config.HTTPClient, httpjson.Endpoint(c.config.BaseURL, "chat/completions"), headers(c.config), input, &output); err != nil {
		return model.Response{}, err
	}
	if output.Error != nil {
		return model.Response{}, errors.New(output.Error.Message)
	}
	if len(output.Choices) == 0 {
		return model.Response{}, errors.New("openai: response has no choices")
	}
	message := output.Choices[0].Message
	result := model.Response{Text: message.Content}
	for _, call := range message.ToolCalls {
		result.ToolCalls = append(result.ToolCalls, model.ToolCall{ID: call.ID, Name: call.Function.Name, Arguments: json.RawMessage(call.Function.Arguments)})
	}
	return result, nil
}

// Responses implements model.Model with POST /responses.
type Responses struct{ config Config }

// NewResponses creates a Responses API adapter.
// @param config endpoint, credentials, model, and HTTP options.
// @return configured model adapter.
func NewResponses(config Config) *Responses { return &Responses{config: normalize(config)} }

// Generate performs one Responses API turn.
// @param ctx request cancellation context.
// @param req provider-neutral messages and tools.
// @return assistant response or provider error.
func (c *Responses) Generate(ctx context.Context, req model.Request) (model.Response, error) {
	if err := validate(c.config); err != nil {
		return model.Response{}, err
	}
	input := responsesRequest{Model: c.config.Model, Input: responseItems(req.Messages), Tools: responseTools(req.Tools)}
	var output responsesResponse
	if err := httpjson.Post(ctx, c.config.HTTPClient, httpjson.Endpoint(c.config.BaseURL, "responses"), headers(c.config), input, &output); err != nil {
		return model.Response{}, err
	}
	if output.Error != nil {
		return model.Response{}, errors.New(output.Error.Message)
	}
	result := model.Response{Text: output.OutputText}
	var texts []string
	for _, item := range output.Output {
		switch item.Type {
		case "message":
			for _, content := range item.Content {
				if content.Type == "output_text" && content.Text != "" {
					texts = append(texts, content.Text)
				}
			}
		case "function_call":
			result.ToolCalls = append(result.ToolCalls, model.ToolCall{ID: item.CallID, Name: item.Name, Arguments: json.RawMessage(item.Arguments)})
		}
	}
	if result.Text == "" {
		result.Text = strings.Join(texts, "")
	}
	return result, nil
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Tools    []chatTool    `json:"tools,omitempty"`
}
type chatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Name       string         `json:"name,omitempty"`
}
type chatToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function chatFunction `json:"function"`
}
type chatFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Arguments   string          `json:"arguments,omitempty"`
}
type chatTool struct {
	Type     string       `json:"type"`
	Function chatFunction `json:"function"`
}
type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *apiError `json:"error,omitempty"`
}
type responsesRequest struct {
	Model string         `json:"model"`
	Input []responseItem `json:"input"`
	Tools []responseTool `json:"tools,omitempty"`
}
type responseItem struct {
	Type      string `json:"type,omitempty"`
	Role      string `json:"role,omitempty"`
	Content   string `json:"content,omitempty"`
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	Output    string `json:"output,omitempty"`
}
type responseTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}
type responsesResponse struct {
	OutputText string `json:"output_text,omitempty"`
	Output     []struct {
		Type      string `json:"type"`
		CallID    string `json:"call_id,omitempty"`
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
		Content   []struct {
			Type string `json:"type"`
			Text string `json:"text,omitempty"`
		} `json:"content,omitempty"`
	} `json:"output"`
	Error *apiError `json:"error,omitempty"`
}
type apiError struct {
	Message string `json:"message"`
}

// normalize applies endpoint defaults and copies mutable maps.
// @param config caller-supplied configuration.
// @return normalized configuration.
func normalize(config Config) Config {
	if config.BaseURL == "" {
		config.BaseURL = defaultBaseURL
	}
	config.Headers = cloneHeaders(config.Headers)
	return config
}

// validate checks values required before an API call.
// @param config normalized configuration.
// @return configuration error or nil.
func validate(config Config) error {
	if config.Model == "" {
		return errors.New("openai: model is required")
	}
	if config.BaseURL == "" {
		return errors.New("openai: base URL is required")
	}
	return nil
}

// headers builds request headers without mutating Config.
// @param config provider configuration.
// @return request headers.
func headers(config Config) map[string]string {
	result := cloneHeaders(config.Headers)
	if config.APIKey != "" {
		result["Authorization"] = "Bearer " + config.APIKey
	}
	return result
}

// cloneHeaders copies custom headers.
// @param source headers to copy.
// @return independent header map.
func cloneHeaders(source map[string]string) map[string]string {
	result := make(map[string]string, len(source)+1)
	for name, value := range source {
		result[name] = value
	}
	return result
}

// chatMessages converts conversation history.
// @param messages provider-neutral messages.
// @return Chat Completions messages.
func chatMessages(messages []model.Message) []chatMessage {
	result := make([]chatMessage, 0, len(messages))
	for _, message := range messages {
		item := chatMessage{Role: string(message.Role), Content: message.Content, ToolCallID: message.ToolCallID, Name: message.Name}
		for _, call := range message.ToolCalls {
			item.ToolCalls = append(item.ToolCalls, chatToolCall{ID: call.ID, Type: "function", Function: chatFunction{Name: call.Name, Arguments: string(call.Arguments)}})
		}
		result = append(result, item)
	}
	return result
}

// chatTools converts tool definitions.
// @param tools provider-neutral definitions.
// @return Chat Completions tools.
func chatTools(tools []model.ToolDefinition) []chatTool {
	result := make([]chatTool, 0, len(tools))
	for _, tool := range tools {
		result = append(result, chatTool{Type: "function", Function: chatFunction{Name: tool.Name, Description: tool.Description, Parameters: tool.Parameters}})
	}
	return result
}

// responseItems converts history to stateless input items.
// @param messages provider-neutral messages.
// @return Responses API input items.
func responseItems(messages []model.Message) []responseItem {
	var result []responseItem
	for _, message := range messages {
		if message.Role == model.RoleTool {
			result = append(result, responseItem{Type: "function_call_output", CallID: message.ToolCallID, Output: message.Content})
			continue
		}
		if message.Content != "" {
			result = append(result, responseItem{Role: string(message.Role), Content: message.Content})
		}
		for _, call := range message.ToolCalls {
			result = append(result, responseItem{Type: "function_call", CallID: call.ID, Name: call.Name, Arguments: string(call.Arguments)})
		}
	}
	return result
}

// responseTools converts tool definitions.
// @param tools provider-neutral definitions.
// @return Responses API tools.
func responseTools(tools []model.ToolDefinition) []responseTool {
	result := make([]responseTool, 0, len(tools))
	for _, tool := range tools {
		result = append(result, responseTool{Type: "function", Name: tool.Name, Description: tool.Description, Parameters: tool.Parameters})
	}
	return result
}

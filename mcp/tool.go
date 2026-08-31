package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/carsonfeng/harness/model"
	"github.com/carsonfeng/harness/tool"
)

var emptyObjectSchema = json.RawMessage(`{"type":"object","properties":{}}`)

// Tools discovers every tool exposed by the MCP server.
// @param ctx request cancellation context.
// @return Harness tools or discovery error.
func (c *Client) Tools(ctx context.Context) ([]tool.Tool, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	c.discoverMu.Lock()
	defer c.discoverMu.Unlock()
	c.debugf("event=discovery_start")
	c.mu.RLock()
	mode := c.mode
	c.mu.RUnlock()
	if mode == modeUnknown {
		definitions, modernErr := c.listAll(ctx, modeModern)
		if modernErr == nil {
			c.mu.Lock()
			c.mode = modeModern
			c.protocolVersion = modernProtocolVersion
			c.mu.Unlock()
			wrapped, err := c.wrapTools(definitions)
			if err == nil {
				c.debugf("event=discovery_finish protocol=%q tools=%q", modernProtocolVersion, definitionNames(definitions))
			}
			return wrapped, err
		}
		c.debugf("event=protocol_fallback from=%q error=%q", modernProtocolVersion, c.debugError(modernErr))
		if initErr := c.initializeLegacy(ctx); initErr != nil {
			return nil, fmt.Errorf("mcp: modern discovery failed: %v; legacy initialization failed: %w", modernErr, initErr)
		}
		mode = modeLegacy
	}
	definitions, err := c.listAll(ctx, mode)
	if err != nil {
		return nil, err
	}
	wrapped, err := c.wrapTools(definitions)
	if err == nil {
		c.debugf("event=discovery_finish protocol=%q tools=%q", c.negotiatedProtocol(), definitionNames(definitions))
	}
	return wrapped, err
}

// initializeLegacy establishes an MCP 2025 session.
// @param ctx request cancellation context.
// @return initialization error or nil.
func (c *Client) initializeLegacy(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": legacyProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo": map[string]string{
			"name":    c.config.ClientName,
			"version": c.config.ClientVersion,
		},
	}
	id := c.nextID.Add(1)
	request := rpcRequest{JSONRPC: "2.0", ID: id, Method: "initialize", Params: params}
	response, headers, err := c.send(ctx, request, "initialize", "", modeUnknown)
	if err != nil {
		return err
	}
	if response.Error != nil {
		return response.Error
	}
	var result initializeResult
	if err := json.Unmarshal(response.Result, &result); err != nil {
		return fmt.Errorf("mcp: decode initialize result: %w", err)
	}
	if result.ProtocolVersion == "" {
		return errors.New("mcp: initialize response has no protocol version")
	}
	c.mu.Lock()
	c.mode = modeLegacy
	c.protocolVersion = result.ProtocolVersion
	c.sessionID = headers.Get("Mcp-Session-Id")
	c.mu.Unlock()
	if err := c.notify(ctx, "notifications/initialized", modeLegacy); err != nil {
		c.mu.Lock()
		c.mode = modeUnknown
		c.protocolVersion = ""
		c.sessionID = ""
		c.mu.Unlock()
		return fmt.Errorf("mcp: send initialized notification: %w", err)
	}
	c.debugf("event=initialized protocol=%q session=%t", result.ProtocolVersion, headers.Get("Mcp-Session-Id") != "")
	return nil
}

// listAll retrieves every page of MCP tool definitions.
// @param ctx request cancellation context.
// @param mode negotiated protocol mode.
// @return remote definitions or listing error.
func (c *Client) listAll(ctx context.Context, mode protocolMode) ([]remoteDefinition, error) {
	var definitions []remoteDefinition
	cursor := ""
	for {
		params := make(map[string]any)
		if cursor != "" {
			params["cursor"] = cursor
		}
		var page listToolsResult
		if err := c.call(ctx, "tools/list", "", params, mode, &page); err != nil {
			return nil, fmt.Errorf("mcp: list tools: %w", err)
		}
		definitions = append(definitions, page.Tools...)
		c.debugf("event=tools_list protocol=%q tools=%q next_cursor=%t", c.protocolName(mode), definitionNames(page.Tools), page.NextCursor != "")
		if page.NextCursor == "" {
			return definitions, nil
		}
		if page.NextCursor == cursor {
			return nil, errors.New("mcp: tools/list returned a repeated cursor")
		}
		cursor = page.NextCursor
	}
}

// wrapTools converts remote MCP definitions to Harness tools.
// @param definitions remote tool definitions.
// @return Harness tools or schema/name error.
func (c *Client) wrapTools(definitions []remoteDefinition) ([]tool.Tool, error) {
	result := make([]tool.Tool, 0, len(definitions))
	seen := make(map[string]string, len(definitions))
	for _, definition := range definitions {
		if definition.Name == "" {
			return nil, errors.New("mcp: tool has no name")
		}
		name := portableName(c.config.ToolPrefix + definition.Name)
		if previous, exists := seen[name]; exists {
			return nil, fmt.Errorf("mcp: tool names %q and %q normalize to %q", previous, definition.Name, name)
		}
		seen[name] = definition.Name
		schema := definition.InputSchema
		if len(schema) == 0 || string(schema) == "null" {
			schema = append(json.RawMessage(nil), emptyObjectSchema...)
		}
		if !json.Valid(schema) {
			return nil, fmt.Errorf("mcp: tool %q has invalid input schema", definition.Name)
		}
		result = append(result, &remoteTool{
			client:     c,
			remoteName: definition.Name,
			definition: model.ToolDefinition{Name: name, Description: definition.Description, Parameters: append(json.RawMessage(nil), schema...)},
		})
	}
	return result, nil
}

type remoteTool struct {
	client     *Client
	remoteName string
	definition model.ToolDefinition
}

// Definition returns the model-visible MCP tool schema.
// @param t remote MCP tool.
// @return tool definition.
func (t *remoteTool) Definition() model.ToolDefinition { return t.definition }

// Execute invokes the remote MCP tool.
// @param ctx tool cancellation context.
// @param arguments JSON arguments.
// @return normalized MCP result or execution error.
func (t *remoteTool) Execute(ctx context.Context, arguments json.RawMessage) (any, error) {
	var decoded map[string]any
	if len(arguments) == 0 {
		arguments = json.RawMessage("{}")
		decoded = make(map[string]any)
	} else if err := json.Unmarshal(arguments, &decoded); err != nil {
		return nil, fmt.Errorf("mcp: decode tool arguments: %w", err)
	}
	mode := t.client.currentMode()
	t.client.debugf("event=tool_call tool=%q arguments=%s", t.remoteName, string(arguments))
	var result callToolResult
	if err := t.client.call(ctx, "tools/call", t.remoteName, map[string]any{
		"name":      t.remoteName,
		"arguments": decoded,
	}, mode, &result); err != nil {
		t.client.debugf("event=tool_error tool=%q error=%q", t.remoteName, t.client.debugError(err))
		return nil, fmt.Errorf("mcp: call %s: %w", t.remoteName, err)
	}
	t.client.debugf("event=tool_result tool=%q is_error=%t result=%s", t.remoteName, result.IsError, debugResult(result))
	if result.IsError {
		message := contentText(result.Content)
		if message == "" {
			message = "MCP tool returned an error"
		}
		return nil, errors.New(message)
	}
	if len(result.StructuredContent) > 0 && string(result.StructuredContent) != "null" {
		return result.StructuredContent, nil
	}
	if text := contentText(result.Content); text != "" {
		return text, nil
	}
	return result.Content, nil
}

// currentMode returns the negotiated protocol mode.
// @param c MCP client.
// @return protocol mode.
func (c *Client) currentMode() protocolMode {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.mode
}

// negotiatedProtocol returns the server-selected protocol version.
// @param c MCP client.
// @return negotiated protocol version.
func (c *Client) negotiatedProtocol() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.protocolVersion
}

// definitionNames returns remote MCP Tool names.
// @param definitions remote definitions.
// @return names in server order.
func definitionNames(definitions []remoteDefinition) []string {
	names := make([]string, len(definitions))
	for index, definition := range definitions {
		names[index] = definition.Name
	}
	return names
}

// contentText joins textual MCP content blocks.
// @param content MCP content blocks.
// @return joined text.
func contentText(content []map[string]any) string {
	var values []string
	for _, block := range content {
		if block["type"] == "text" {
			if text, ok := block["text"].(string); ok {
				values = append(values, text)
			}
		}
	}
	return strings.Join(values, "\n")
}

// portableName converts an MCP name to provider-portable function syntax.
// @param name remote tool name with optional prefix.
// @return normalized model-visible name.
func portableName(name string) string {
	var result strings.Builder
	for index, value := range name {
		valid := unicode.IsLetter(value) || (index > 0 && unicode.IsDigit(value)) || (index > 0 && (value == '_' || value == '-'))
		if valid && value <= unicode.MaxASCII {
			result.WriteRune(value)
		} else {
			result.WriteByte('_')
		}
	}
	normalized := strings.Trim(result.String(), "_-")
	if normalized == "" || !((normalized[0] >= 'a' && normalized[0] <= 'z') || (normalized[0] >= 'A' && normalized[0] <= 'Z')) {
		normalized = "mcp_" + normalized
	}
	return normalized
}

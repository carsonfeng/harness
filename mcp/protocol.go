package mcp

import "encoding/json"

const (
	modernProtocolVersion = "2026-07-28"
	legacyProtocolVersion = "2025-11-25"
)

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      uint64 `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is an error returned by an MCP server.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Error formats an MCP JSON-RPC error.
// @param e MCP error.
// @return compact error text.
func (e *RPCError) Error() string {
	if len(e.Data) == 0 || string(e.Data) == "null" {
		return e.Message
	}
	return e.Message + ": " + string(e.Data)
}

type initializeResult struct {
	ProtocolVersion string `json:"protocolVersion"`
}

type listToolsResult struct {
	Tools      []remoteDefinition `json:"tools"`
	NextCursor string             `json:"nextCursor,omitempty"`
}

type remoteDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type callToolResult struct {
	Content           []map[string]any `json:"content"`
	StructuredContent json.RawMessage  `json:"structuredContent,omitempty"`
	IsError           bool             `json:"isError,omitempty"`
}

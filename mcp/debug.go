package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
)

const debugPreviewLimit = 500

// SetDebug writes MCP discovery, protocol, Tool call, and result events.
// A nil output disables MCP debug logging.
// @param output debug log destination.
// @return none.
func (c *Client) SetDebug(output io.Writer) {
	if c == nil {
		return
	}
	c.debugMu.Lock()
	defer c.debugMu.Unlock()
	if output == nil {
		c.debug = nil
		return
	}
	c.debug = log.New(output, "mcp: ", log.LstdFlags)
}

// debugf writes one MCP debug event when logging is enabled.
// @param format printf-style event format.
// @param args event values.
// @return none.
func (c *Client) debugf(format string, args ...any) {
	c.debugMu.RLock()
	logger := c.debug
	c.debugMu.RUnlock()
	if logger != nil {
		logger.Printf(format, args...)
	}
}

// debugResult formats a compact MCP Tool result.
// @param result MCP Tool result.
// @return compact JSON preview.
func debugResult(result callToolResult) string {
	encoded, err := json.Marshal(result)
	if err != nil {
		return fmt.Sprintf("%q", err.Error())
	}
	return debugPreview(string(encoded))
}

// debugPreview truncates verbose MCP data while preserving valid UTF-8.
// @param value text to summarize.
// @return original or truncated text.
func debugPreview(value string) string {
	runes := []rune(value)
	if len(runes) <= debugPreviewLimit {
		return value
	}
	return string(runes[:debugPreviewLimit]) + fmt.Sprintf("… [truncated %d chars]", len(runes)-debugPreviewLimit)
}

// debugError redacts the configured Endpoint from an error.
// @param c MCP client.
// @param err operation error.
// @return compact redacted error text.
func (c *Client) debugError(err error) string {
	if err == nil {
		return ""
	}
	value := err.Error()
	if c.config.Endpoint != "" {
		value = strings.ReplaceAll(value, c.config.Endpoint, "[endpoint]")
	}
	return debugPreview(value)
}

// protocolName returns the active MCP protocol version.
// @param c MCP client.
// @param mode protocol mode.
// @return protocol version or negotiation state.
func (c *Client) protocolName(mode protocolMode) string {
	version, _ := c.connectionState(mode)
	if version == "" {
		return "negotiating"
	}
	return version
}

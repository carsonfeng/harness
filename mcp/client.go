// Package mcp exposes Streamable HTTP MCP servers as Harness tools.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
)

const maxResponseBytes = 8 << 20

// Config configures one Streamable HTTP MCP server.
type Config struct {
	Endpoint      string
	HTTPClient    *http.Client
	Headers       map[string]string
	ToolPrefix    string
	ClientName    string
	ClientVersion string
}

// Client discovers and calls tools from one MCP server.
type Client struct {
	config     Config
	nextID     atomic.Uint64
	discoverMu sync.Mutex

	mu              sync.RWMutex
	mode            protocolMode
	protocolVersion string
	sessionID       string
}

type protocolMode uint8

const (
	modeUnknown protocolMode = iota
	modeModern
	modeLegacy
)

// New creates a Streamable HTTP MCP client.
// @param config endpoint, HTTP, headers, and naming options.
// @return configured MCP client.
func New(config Config) *Client {
	config.Endpoint = strings.TrimSpace(config.Endpoint)
	if config.ClientName == "" {
		config.ClientName = "harness-go"
	}
	if config.ClientVersion == "" {
		config.ClientVersion = "0.1.3"
	}
	headers := make(map[string]string, len(config.Headers))
	for name, value := range config.Headers {
		headers[name] = value
	}
	config.Headers = headers
	return &Client{config: config}
}

// validate checks configuration required by network operations.
// @param c MCP client.
// @return configuration error or nil.
func (c *Client) validate() error {
	if c == nil {
		return errors.New("mcp: nil client")
	}
	if c.config.Endpoint == "" {
		return errors.New("mcp: endpoint is required")
	}
	parsed, err := url.Parse(c.config.Endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("mcp: endpoint must be an absolute URL")
	}
	return nil
}

// clientInfo returns metadata required by modern stateless MCP requests.
// @param c MCP client.
// @return request metadata.
func (c *Client) clientInfo() map[string]any {
	return map[string]any{
		"io.modelcontextprotocol/clientInfo": map[string]string{
			"name":    c.config.ClientName,
			"version": c.config.ClientVersion,
		},
	}
}

// call sends one JSON-RPC request and decodes its result.
// @param ctx request cancellation context.
// @param method MCP method.
// @param name optional tool name.
// @param params method parameters.
// @param mode protocol mode.
// @param output result destination.
// @return protocol or transport error.
func (c *Client) call(ctx context.Context, method, name string, params map[string]any, mode protocolMode, output any) error {
	if params == nil {
		params = make(map[string]any)
	}
	if mode == modeModern {
		params["_meta"] = c.clientInfo()
	}
	id := c.nextID.Add(1)
	request := rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	response, _, err := c.send(ctx, request, method, name, mode)
	if err != nil {
		return err
	}
	if response.Error != nil {
		return response.Error
	}
	if len(response.Result) == 0 {
		return errors.New("mcp: response has no result")
	}
	if err := json.Unmarshal(response.Result, output); err != nil {
		return fmt.Errorf("mcp: decode %s result: %w", method, err)
	}
	return nil
}

// notify sends one JSON-RPC notification.
// @param ctx request cancellation context.
// @param method MCP notification method.
// @param mode protocol mode.
// @return transport error.
func (c *Client) notify(ctx context.Context, method string, mode protocolMode) error {
	request := rpcRequest{JSONRPC: "2.0", Method: method}
	_, _, err := c.send(ctx, request, method, "", mode)
	return err
}

// send performs one Streamable HTTP exchange.
// @param ctx request cancellation context.
// @param message JSON-RPC message.
// @param method MCP method.
// @param name optional tool name.
// @param mode protocol mode.
// @return response, HTTP headers, or transport error.
func (c *Client) send(ctx context.Context, message rpcRequest, method, name string, mode protocolMode) (rpcResponse, http.Header, error) {
	body, err := json.Marshal(message)
	if err != nil {
		return rpcResponse{}, nil, fmt.Errorf("mcp: encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.Endpoint, bytes.NewReader(body))
	if err != nil {
		return rpcResponse{}, nil, fmt.Errorf("mcp: create request: %w", err)
	}
	for header, value := range c.config.Headers {
		req.Header.Set(header, value)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Method", method)
	if name != "" {
		req.Header.Set("Mcp-Name", name)
	}
	protocolVersion, sessionID := c.connectionState(mode)
	if protocolVersion != "" {
		req.Header.Set("MCP-Protocol-Version", protocolVersion)
	}
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	client := c.config.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return rpcResponse{}, nil, fmt.Errorf("mcp: send request: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return rpcResponse{}, resp.Header, fmt.Errorf("mcp: read response: %w", err)
	}
	if len(data) > maxResponseBytes {
		return rpcResponse{}, resp.Header, errors.New("mcp: response exceeds 8 MiB")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return rpcResponse{}, resp.Header, fmt.Errorf("mcp: server returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return rpcResponse{}, resp.Header, nil
	}
	decoded, err := decodeResponse(data, resp.Header.Get("Content-Type"), message.ID)
	if err != nil {
		return rpcResponse{}, resp.Header, err
	}
	return decoded, resp.Header, nil
}

// connectionState returns headers for one protocol mode.
// @param c MCP client.
// @param mode protocol mode.
// @return protocol version and session ID.
func (c *Client) connectionState(mode protocolMode) (string, string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if mode == modeModern {
		return modernProtocolVersion, ""
	}
	if mode == modeLegacy {
		return c.protocolVersion, c.sessionID
	}
	return "", ""
}

// decodeResponse decodes JSON or an SSE data event for one request.
// @param data response bytes.
// @param contentType HTTP content type.
// @param expectedID request ID; zero disables ID matching.
// @return JSON-RPC response or decoding error.
func decodeResponse(data []byte, contentType string, expectedID uint64) (rpcResponse, error) {
	if strings.Contains(strings.ToLower(contentType), "text/event-stream") {
		for _, event := range sseDataEvents(data) {
			var response rpcResponse
			if json.Unmarshal(event, &response) == nil && (len(response.Result) > 0 || response.Error != nil) && responseMatches(response, expectedID) {
				return response, nil
			}
		}
		return rpcResponse{}, errors.New("mcp: SSE response has no JSON-RPC response")
	}
	var response rpcResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return response, fmt.Errorf("mcp: decode response: %w", err)
	}
	if !responseMatches(response, expectedID) {
		return response, fmt.Errorf("mcp: response ID %s does not match request %d", response.ID, expectedID)
	}
	return response, nil
}

// responseMatches checks a JSON-RPC response ID.
// @param response decoded response.
// @param expectedID request ID; zero accepts any response.
// @return true when IDs match.
func responseMatches(response rpcResponse, expectedID uint64) bool {
	if expectedID == 0 {
		return true
	}
	var actual uint64
	return json.Unmarshal(response.ID, &actual) == nil && actual == expectedID
}

// sseDataEvents extracts data events from an SSE response.
// @param data SSE response bytes.
// @return event data in stream order.
func sseDataEvents(data []byte) [][]byte {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64<<10), maxResponseBytes)
	var current []string
	var events [][]byte
	flush := func() {
		if len(current) > 0 {
			events = append(events, []byte(strings.Join(current, "\n")))
			current = nil
		}
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, "data:") {
			current = append(current, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	flush()
	return events
}

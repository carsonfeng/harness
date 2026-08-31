package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

// RoundTrip invokes a test HTTP transport function.
// @param request outgoing HTTP request.
// @return HTTP response or transport error.
func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

// TestModernToolsAndCall verifies stateless discovery, pagination, and calls.
// @param t test state.
// @return none.
func TestModernToolsAndCall(t *testing.T) {
	var logs bytes.Buffer
	var mu sync.Mutex
	listCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if got := r.Header.Get("MCP-Protocol-Version"); got != modernProtocolVersion {
			t.Errorf("protocol header = %q", got)
		}
		if got := r.Header.Get("Mcp-Method"); got != request.Method {
			t.Errorf("method header = %q, body = %q", got, request.Method)
		}
		if request.Params["_meta"] == nil {
			t.Error("modern request has no client metadata")
		}
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "tools/list":
			mu.Lock()
			listCalls++
			page := listCalls
			mu.Unlock()
			if page == 1 {
				fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"lookup.weather","description":"Look up weather","inputSchema":{"type":"object","properties":{"city":{"type":"string"}}}}],"nextCursor":"page-2"}}`)
				return
			}
			if request.Params["cursor"] != "page-2" {
				t.Errorf("cursor = %#v", request.Params["cursor"])
			}
			fmt.Fprint(w, `{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"ping","inputSchema":{"type":"object"}}]}}`)
		case "tools/call":
			if got := r.Header.Get("Mcp-Name"); got != "lookup.weather" {
				t.Errorf("tool name header = %q", got)
			}
			if request.Params["name"] != "lookup.weather" {
				t.Errorf("tool name = %#v", request.Params["name"])
			}
			fmt.Fprint(w, `{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"sunny"}],"structuredContent":{"temperature":28},"isError":false}}`)
		default:
			t.Errorf("unexpected method %q", request.Method)
		}
	}))
	defer server.Close()

	client := New(Config{Endpoint: server.URL, ToolPrefix: "remote_", Debug: &logs})
	tools, err := client.Tools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 2 || tools[0].Definition().Name != "remote_lookup_weather" || tools[1].Definition().Name != "remote_ping" {
		t.Fatalf("tools = %#v, %#v", tools[0].Definition(), tools[1].Definition())
	}
	result, err := tools[0].Execute(context.Background(), json.RawMessage(`{"city":"Guangzhou"}`))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"temperature":28}` {
		t.Fatalf("result = %s", encoded)
	}
	output := logs.String()
	for _, want := range []string{
		"event=discovery_start",
		`event=tools_list protocol="2026-07-28" tools=["lookup.weather"] next_cursor=true`,
		`event=discovery_finish protocol="2026-07-28" tools=["lookup.weather" "ping"]`,
		`event=tool_call tool="lookup.weather" arguments={"city":"Guangzhou"}`,
		`event=tool_result tool="lookup.weather" is_error=false`,
		`"structuredContent":{"temperature":28}`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("MCP debug output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, server.URL) {
		t.Fatalf("MCP debug output leaked endpoint:\n%s", output)
	}
}

// TestLegacyInitialization verifies fallback to initialize and session headers.
// @param t test state.
// @return none.
func TestLegacyInitialization(t *testing.T) {
	initialized := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     uint64 `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "session-123")
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":{"protocolVersion":"2025-11-25","capabilities":{"tools":{}},"serverInfo":{"name":"test","version":"1"}}}`, request.ID)
		case "notifications/initialized":
			if r.Header.Get("Mcp-Session-Id") != "session-123" {
				t.Errorf("initialized session = %q", r.Header.Get("Mcp-Session-Id"))
			}
			initialized = true
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			if r.Header.Get("MCP-Protocol-Version") == modernProtocolVersion {
				fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"error":{"code":-32002,"message":"Server not initialized"}}`, request.ID)
				return
			}
			if !initialized || r.Header.Get("Mcp-Session-Id") != "session-123" {
				t.Error("legacy list sent before initialization or without session")
			}
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":{"tools":[{"name":"legacy_tool","inputSchema":{"type":"object"}}]}}`, request.ID)
		default:
			t.Errorf("unexpected method %q", request.Method)
		}
	}))
	defer server.Close()

	tools, err := New(Config{Endpoint: server.URL}).Tools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 || tools[0].Definition().Name != "legacy_tool" {
		t.Fatalf("tools = %#v", tools)
	}
}

// TestSSEResponseAndToolError verifies SSE decoding and model-visible errors.
// @param t test state.
// @return none.
func TestSSEResponseAndToolError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     uint64 `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if request.Method == "tools/list" {
			fmt.Fprintf(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/message\"}\n\nevent: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":%d,\"result\":{\"tools\":[{\"name\":\"fail\",\"inputSchema\":{\"type\":\"object\"}}]}}\n\n", request.ID)
			return
		}
		fmt.Fprintf(w, "data: {\"jsonrpc\":\"2.0\",\"id\":%d,\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"permission denied\"}],\"isError\":true}}\n\n", request.ID)
	}))
	defer server.Close()

	tools, err := New(Config{Endpoint: server.URL}).Tools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = tools[0].Execute(context.Background(), json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("error = %v", err)
	}
}

// TestNormalizedNameCollision rejects ambiguous provider-visible names.
// @param t test state.
// @return none.
func TestNormalizedNameCollision(t *testing.T) {
	client := New(Config{Endpoint: "https://example.com"})
	_, err := client.wrapTools([]remoteDefinition{{Name: "one.two"}, {Name: "one/two"}})
	if err == nil || !strings.Contains(err.Error(), "normalize") {
		t.Fatalf("error = %v", err)
	}
}

// TestDebugRedactsEndpoint verifies URL credentials never enter MCP logs.
// @param t test state.
// @return none.
func TestDebugRedactsEndpoint(t *testing.T) {
	var logs bytes.Buffer
	endpoint := "https://example.com/private?token=endpoint-secret"
	client := New(Config{
		Endpoint: endpoint,
		Headers:  map[string]string{"Authorization": "Bearer header-secret"},
		Debug:    &logs,
		HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return nil, errors.New("cannot reach " + request.URL.String())
		})},
	})
	if _, err := client.Tools(context.Background()); err == nil {
		t.Fatal("expected discovery error")
	}
	output := logs.String()
	for _, secret := range []string{endpoint, "endpoint-secret", "header-secret"} {
		if strings.Contains(output, secret) {
			t.Fatalf("MCP debug output leaked %q:\n%s", secret, output)
		}
	}
	if !strings.Contains(output, "[endpoint]") {
		t.Fatalf("MCP debug output did not show endpoint redaction:\n%s", output)
	}
}

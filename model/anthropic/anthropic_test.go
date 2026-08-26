package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/carsonfeng/harness/model"
)

// TestMessages verifies Anthropic headers, content blocks, and tool parsing.
// @param t test state.
// @return none.
func TestMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/proxy/messages" || r.Header.Get("x-api-key") != "secret" || r.Header.Get("anthropic-version") != defaultVersion {
			t.Fatalf("request headers or path are invalid")
		}
		var body struct {
			System   string    `json:"system"`
			Messages []message `json:"messages"`
			Tools    []tool    `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.System != "Be concise" || len(body.Tools) != 1 || len(body.Messages) != 3 {
			t.Fatalf("body = %#v", body)
		}
		result := body.Messages[2].Content[0]
		if result.Type != "tool_result" || result.ToolUseID != "tool-1" {
			t.Fatalf("tool result = %#v", result)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"Checking"},{"type":"tool_use","id":"tool-2","name":"clock","input":{}}]}`))
	}))
	defer server.Close()
	client := New(Config{APIKey: "secret", Model: "claude-test", BaseURL: server.URL + "/proxy"})
	response, err := client.Generate(context.Background(), model.Request{
		Messages: []model.Message{
			{Role: model.RoleSystem, Content: "Be concise"},
			{Role: model.RoleUser, Content: "weather?"},
			{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{{ID: "tool-1", Name: "weather", Arguments: json.RawMessage(`{"city":"GZ"}`)}}},
			{Role: model.RoleTool, ToolCallID: "tool-1", Content: `"sunny"`},
		},
		Tools: []model.ToolDefinition{{Name: "weather", Parameters: json.RawMessage(`{"type":"object"}`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Text != "Checking" || len(response.ToolCalls) != 1 || response.ToolCalls[0].ID != "tool-2" {
		t.Fatalf("response = %#v", response)
	}
}

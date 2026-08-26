package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/carsonfeng/harness/model"
)

// TestChatCompletions verifies request translation and tool-call parsing.
// @param t test state.
// @return none.
func TestChatCompletions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/gateway/chat/completions" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("request = %s, auth = %s", r.URL.Path, r.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["model"] != "test-model" || len(body["tools"].([]any)) != 1 {
			t.Fatalf("body = %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call-1","type":"function","function":{"name":"weather","arguments":"{\"city\":\"GZ\"}"}}]}}]}`))
	}))
	defer server.Close()
	client := NewChatCompletions(Config{APIKey: "secret", Model: "test-model", BaseURL: server.URL + "/gateway"})
	response, err := client.Generate(context.Background(), fixtureRequest())
	if err != nil {
		t.Fatal(err)
	}
	if len(response.ToolCalls) != 1 || response.ToolCalls[0].Name != "weather" {
		t.Fatalf("response = %#v", response)
	}
}

// TestResponses verifies stateless function-call item translation.
// @param t test state.
// @return none.
func TestResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		var body struct {
			Input []map[string]any `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		last := body.Input[len(body.Input)-1]
		if last["type"] != "function_call_output" || last["call_id"] != "call-1" {
			t.Fatalf("last input = %#v", last)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":[{"type":"message","content":[{"type":"output_text","text":"Sunny"}]},{"type":"function_call","call_id":"call-2","name":"clock","arguments":"{}"}]}`))
	}))
	defer server.Close()
	client := NewResponses(Config{Model: "test-model", BaseURL: server.URL + "/v1"})
	response, err := client.Generate(context.Background(), fixtureRequest())
	if err != nil {
		t.Fatal(err)
	}
	if response.Text != "Sunny" || len(response.ToolCalls) != 1 || response.ToolCalls[0].ID != "call-2" {
		t.Fatalf("response = %#v", response)
	}
}

// fixtureRequest builds a conversation containing a complete tool round-trip.
// @param none.
// @return model request used by adapter tests.
func fixtureRequest() model.Request {
	return model.Request{
		Messages: []model.Message{
			{Role: model.RoleUser, Content: "weather?"},
			{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{{ID: "call-1", Name: "weather", Arguments: json.RawMessage(`{"city":"GZ"}`)}}},
			{Role: model.RoleTool, ToolCallID: "call-1", Name: "weather", Content: `"sunny"`},
		},
		Tools: []model.ToolDefinition{{Name: "weather", Description: "Get weather", Parameters: json.RawMessage(`{"type":"object"}`)}},
	}
}

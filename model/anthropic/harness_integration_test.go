package anthropic_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/carsonfeng/harness"
	"github.com/carsonfeng/harness/model/anthropic"
)

// TestHarnessToolLoopUsesStatelessMessages verifies the complete wire flow.
// @param t test state.
// @return none.
func TestHarnessToolLoopUsesStatelessMessages(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		turn := requests.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var request map[string]json.RawMessage
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatal(err)
		}
		if _, exists := request["previous_response_id"]; exists {
			t.Fatalf("Anthropic request contains previous_response_id: %s", body)
		}
		var messages []struct {
			Role    string `json:"role"`
			Content []struct {
				Type      string `json:"type"`
				ToolUseID string `json:"tool_use_id"`
			} `json:"content"`
		}
		if err := json.Unmarshal(request["messages"], &messages); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch turn {
		case 1:
			if len(messages) != 1 || messages[0].Role != "user" {
				t.Fatalf("first messages = %#v", messages)
			}
			_, _ = w.Write([]byte(`{"content":[{"type":"tool_use","id":"tool-1","name":"add","input":{"a":23.5,"b":18.25}}]}`))
		case 2:
			if len(messages) != 3 || messages[2].Role != "user" || len(messages[2].Content) != 1 || messages[2].Content[0].Type != "tool_result" || messages[2].Content[0].ToolUseID != "tool-1" {
				t.Fatalf("second messages = %#v", messages)
			}
			_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"41.75"}]}`))
		default:
			t.Fatalf("unexpected request %d", turn)
		}
	}))
	defer server.Close()

	type addArgs struct {
		A float64 `json:"a"`
		B float64 `json:"b"`
	}
	agent := harness.New(harness.WithModel(anthropic.New(anthropic.Config{
		APIKey:  "test-key",
		Model:   "claude-test",
		BaseURL: server.URL + "/v1",
	})))
	if err := agent.Tool(harness.Func("add", "Add two numbers", func(_ context.Context, args addArgs) (float64, error) {
		return args.A + args.B, nil
	})); err != nil {
		t.Fatal(err)
	}
	result, err := agent.Run(context.Background(), "What is 23.5 plus 18.25?")
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "41.75" || result.Steps != 2 || requests.Load() != 2 {
		t.Fatalf("result = %#v, requests = %d", result, requests.Load())
	}
}

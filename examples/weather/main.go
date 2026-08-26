package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/carsonfeng/harness"
)

type weatherArgs struct {
	City string `json:"city" description:"City to look up"`
}

// demoModel makes the example runnable without an API key. A real adapter maps
// harness.ModelRequest and harness.ModelResponse to a provider's SDK types.
type demoModel struct{}

// Generate returns a deterministic function-call conversation.
// @param req model request.
// @return demo response.
func (demoModel) Generate(_ context.Context, req harness.ModelRequest) (harness.ModelResponse, error) {
	last := req.Messages[len(req.Messages)-1]
	if last.Role == harness.RoleUser {
		return harness.ModelResponse{ToolCalls: []harness.ToolCall{{
			ID: "weather-1", Name: "get_weather", Arguments: json.RawMessage(`{"city":"Guangzhou"}`),
		}}}, nil
	}
	return harness.ModelResponse{Text: "The weather tool returned: " + last.Content}, nil
}

// main runs the no-key weather example.
// @param none.
// @return none.
func main() {
	h := harness.New(harness.WithModel(demoModel{}))
	err := h.Tool(harness.Func("get_weather", "Get the current weather", func(_ context.Context, args weatherArgs) (string, error) {
		return args.City + ": 28°C, sunny", nil
	}))
	if err != nil {
		log.Fatal(err)
	}
	result, err := h.Run(context.Background(), "What's the weather in Guangzhou?")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result.Text)
}

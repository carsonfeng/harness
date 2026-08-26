package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/carsonfeng/harness"
	"github.com/carsonfeng/harness/model/openai"
)

type weatherArgs struct {
	City string `json:"city" description:"City to look up"`
}

// main runs a real Chat Completions tool loop.
// @param none.
// @return none.
func main() {
	config := openai.Config{
		APIKey: os.Getenv("OPENAI_API_KEY"),
		Model:  required("OPENAI_MODEL"),
	}
	if baseURL := os.Getenv("OPENAI_BASE_URL"); baseURL != "" {
		config.BaseURL = baseURL
	}
	h := harness.New(
		harness.WithModel(openai.NewChatCompletions(config)),
		harness.WithDebug(os.Stderr),
	)
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

// required reads a required environment variable.
// @param name environment variable name.
// @return configured value.
func required(name string) string {
	value := os.Getenv(name)
	if value == "" {
		log.Fatalf("%s is required", name)
	}
	return value
}

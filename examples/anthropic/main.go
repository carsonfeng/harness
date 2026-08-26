// Command anthropic demonstrates Claude Messages API tool use.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/carsonfeng/harness"
	"github.com/carsonfeng/harness/model/anthropic"
)

type addArgs struct {
	A float64 `json:"a"`
	B float64 `json:"b"`
}

// main lets Claude call a typed addition function.
// @param none.
// @return none.
func main() {
	config := anthropic.Config{
		APIKey: os.Getenv("ANTHROPIC_API_KEY"),
		Model:  required("ANTHROPIC_MODEL"),
	}
	if baseURL := os.Getenv("ANTHROPIC_BASE_URL"); baseURL != "" {
		config.BaseURL = baseURL
	}
	model := anthropic.New(config)
	agent := harness.New(
		harness.WithModel(model),
		harness.WithDebug(os.Stderr),
	)
	err := agent.Tool(harness.Func("add", "Add two numbers", func(_ context.Context, args addArgs) (float64, error) { return args.A + args.B, nil }))
	if err != nil {
		log.Fatal(err)
	}
	result, err := agent.Run(context.Background(), "What is 23.5 plus 18.25?")
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

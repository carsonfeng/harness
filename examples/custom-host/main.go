// Command custom-host shows how to connect an OpenAI-compatible gateway.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/carsonfeng/harness"
	"github.com/carsonfeng/harness/model/openai"
)

// main sends a prompt through a custom Chat Completions host.
// @param none.
// @return none.
func main() {
	model := openai.NewChatCompletions(openai.Config{
		APIKey:  os.Getenv("OPENAI_API_KEY"),
		Model:   required("OPENAI_MODEL"),
		BaseURL: required("OPENAI_BASE_URL"),
		Headers: map[string]string{"X-Gateway-Client": "harness-example"},
	})
	result, err := harness.New(harness.WithModel(model)).Run(context.Background(), "Reply with: gateway works")
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

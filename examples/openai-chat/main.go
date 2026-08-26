// Command openai-chat shows the shortest Chat Completions setup.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/carsonfeng/harness"
	"github.com/carsonfeng/harness/model/openai"
)

// main sends one prompt through Chat Completions.
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
	model := openai.NewChatCompletions(config)
	agent := harness.New(
		harness.WithModel(model),
		harness.WithDebug(os.Stderr),
	)
	result, err := agent.Run(context.Background(), "Explain goroutines in one sentence.")
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

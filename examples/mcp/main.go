package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/carsonfeng/harness"
	"github.com/carsonfeng/harness/mcp"
	"github.com/carsonfeng/harness/model/openai"
)

// main runs a real model with tools discovered from an MCP server.
// @param none.
// @return none.
func main() {
	ctx := context.Background()
	config := openai.Config{
		APIKey: os.Getenv("OPENAI_API_KEY"),
		Model:  required("OPENAI_MODEL"),
	}
	if baseURL := os.Getenv("OPENAI_BASE_URL"); baseURL != "" {
		config.BaseURL = baseURL
	}
	headers := make(map[string]string)
	if token := os.Getenv("MCP_BEARER_TOKEN"); token != "" {
		headers["Authorization"] = "Bearer " + token
	}
	server := mcp.New(mcp.Config{
		Endpoint:   required("MCP_ENDPOINT"),
		Headers:    headers,
		ToolPrefix: os.Getenv("MCP_TOOL_PREFIX"),
	})
	agent := harness.New(
		harness.WithModel(openai.NewChatCompletions(config)),
		harness.WithDebug(os.Stderr),
	)
	if err := agent.MCP(ctx, server); err != nil {
		log.Fatal(err)
	}
	prompt := os.Getenv("MCP_PROMPT")
	if prompt == "" {
		prompt = "Use the available tools to answer the request."
	}
	result, err := agent.Run(ctx, prompt)
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

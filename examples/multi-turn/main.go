// Command multi-turn demonstrates user turns that repeatedly call a tool.
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

type weather struct {
	City        string `json:"city"`
	Temperature int    `json:"temperature"`
	Condition   string `json:"condition"`
}

// getWeather returns deterministic local weather data.
// @param ctx tool cancellation context.
// @param args requested city.
// @return weather result or cancellation error.
func getWeather(ctx context.Context, args weatherArgs) (weather, error) {
	if err := ctx.Err(); err != nil {
		return weather{}, err
	}
	if args.City == "Shenzhen" {
		return weather{City: args.City, Temperature: 30, Condition: "sunny"}, nil
	}
	return weather{City: args.City, Temperature: 28, Condition: "cloudy"}, nil
}

// main runs three user turns on one reusable Thread.
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
	agent := harness.New(
		harness.WithModel(openai.NewChatCompletions(config)),
		harness.WithDebug(os.Stderr),
	)
	if err := agent.Tool(harness.Func("get_weather", "Get weather for a city", getWeather)); err != nil {
		log.Fatal(err)
	}

	thread := harness.NewThread()
	prompts := []string{
		"What's the weather in Guangzhou?",
		"How about Shenzhen?",
		"Which one is warmer?",
	}
	for _, prompt := range prompts {
		result, err := agent.RunThread(context.Background(), thread, prompt)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("User: %s\nAssistant: %s\n\n", prompt, result.Text)
	}

	fmt.Printf("Thread contains %d messages.\n", len(thread.Messages()))
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

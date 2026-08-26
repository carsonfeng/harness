// Command openai-responses demonstrates a typed tool with the Responses API.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/carsonfeng/harness"
	"github.com/carsonfeng/harness/model/openai"
)

type clockArgs struct {
	Timezone string `json:"timezone" description:"IANA timezone, for example Asia/Shanghai"`
}

// main continues two user turns that may call the local clock tool.
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
	model := openai.NewResponses(config)
	agent := harness.New(
		harness.WithModel(model),
		harness.WithDebug(os.Stderr),
	)
	err := agent.Tool(harness.Func("local_time", "Get the current time", localTime))
	if err != nil {
		log.Fatal(err)
	}
	thread := harness.NewThread()
	for _, prompt := range []string{"What time is it in Shanghai?", "How about Tokyo?"} {
		result, err := agent.RunThread(context.Background(), thread, prompt)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(result.Text)
	}
}

// localTime returns the current time in an IANA timezone.
// @param ctx tool cancellation context.
// @param args requested timezone.
// @return formatted local time or timezone error.
func localTime(ctx context.Context, args clockArgs) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	location, err := time.LoadLocation(args.Timezone)
	if err != nil {
		return "", err
	}
	return time.Now().In(location).Format(time.RFC3339), nil
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

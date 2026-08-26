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

// main lets the model call a local clock function.
// @param none.
// @return none.
func main() {
	model := openai.NewResponses(openai.Config{APIKey: os.Getenv("OPENAI_API_KEY"), Model: required("OPENAI_MODEL")})
	agent := harness.New(harness.WithModel(model))
	err := agent.Tool(harness.Func("local_time", "Get the current time", localTime))
	if err != nil {
		log.Fatal(err)
	}
	result, err := agent.Run(context.Background(), "What time is it in Shanghai?")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result.Text)
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

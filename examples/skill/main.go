// Command skill demonstrates automatic SKILL.md discovery and tool use.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/carsonfeng/harness"
	"github.com/carsonfeng/harness/model/openai"
)

type diffArgs struct {
	MergeRequest string `json:"merge_request" description:"Merge request identifier"`
}

type fileArgs struct {
	Path string `json:"path" description:"Repository-relative file path"`
}

type searchArgs struct {
	Query string `json:"query" description:"Symbol or text to find"`
}

// main lets Chat Completions discover and apply a code-review skill.
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
		harness.WithSystem("Load a matching skill before handling specialized work, then follow its instructions."),
		harness.WithDebug(os.Stderr),
	)
	if err := agent.Tool(harness.Func("get_diff", "Read a merge request diff", func(_ context.Context, args diffArgs) (string, error) {
		return args.MergeRequest + ": app/cache.go changes cache invalidation from delete-before-write to write-before-delete", nil
	})); err != nil {
		log.Fatal(err)
	}
	if err := agent.Tool(harness.Func("get_file", "Read a repository file", func(_ context.Context, args fileArgs) (string, error) {
		return args.Path + ": func Replace(key string, value any) { store(key, value); removeOld(key) }", nil
	})); err != nil {
		log.Fatal(err)
	}
	if err := agent.Tool(harness.Func("search_code", "Search repository code", func(_ context.Context, args searchArgs) ([]string, error) {
		return []string{"app/cache.go:42", "app/cache_test.go:18", "cmd/server/main.go:77"}, nil
	})); err != nil {
		log.Fatal(err)
	}
	if err := agent.SkillDir("examples/skills"); err != nil {
		log.Fatal(err)
	}
	result, err := agent.Run(context.Background(), "Review MR !123")
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

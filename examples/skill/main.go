// Command skill demonstrates loading and running a SKILL.md file.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/carsonfeng/harness"
)

// main loads the example code-review skill without an API key.
// @param none.
// @return none.
func main() {
	model := harness.ModelFunc(func(_ context.Context, req harness.ModelRequest) (harness.ModelResponse, error) {
		return harness.ModelResponse{Text: "Loaded instructions:\n" + req.Messages[0].Content}, nil
	})
	agent := harness.New(harness.WithModel(model))
	for _, name := range []string{"get_diff", "get_file", "search_code"} {
		toolName := name
		err := agent.Tool(harness.Func(toolName, "Example tool", func(context.Context, struct{}) (string, error) { return "ok", nil }))
		if err != nil {
			log.Fatal(err)
		}
	}
	if err := agent.SkillDir("examples/skills"); err != nil {
		log.Fatal(err)
	}
	result, err := agent.RunSkill(context.Background(), "code-review", "Review MR !123")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result.Text)
}

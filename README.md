# Harness

**English** | [简体中文](README.zh-CN.md)

> The missing agent loop for Go.

Harness is a small, embeddable, provider-neutral Agent SDK for Go.

It connects models to ordinary Go functions and runs the complete loop:

```text
User → Model → Function Call → Go Function → Model → Final Answer
```

Harness focuses on the minimum set of primitives needed to build an agent:

- Model abstraction
- Type-safe function calling
- Agent loop
- Skills
- Thread and context

It is not a workflow engine, RAG framework, hosted platform, or multi-agent system. The core uses only the Go standard library and has no third-party dependencies.

## Features

- OpenAI Chat Completions API
- OpenAI Responses API
- Anthropic Messages API
- Custom `BaseURL`, headers, and `http.Client`
- Generic Go functions as model tools
- Automatic JSON Schema generation from Go structs
- Multiple sequential function calls
- `SKILL.md`, tool allowlists, and per-skill step limits
- Complete model, tool-call, and tool-result history
- Context cancellation
- Go 1.21+
- Zero third-party dependencies in the core

## Installation

```bash
go get github.com/carsonfeng/harness
```

Harness requires Go 1.21 or later.

## 30-second quick start

The following program sends one request through the OpenAI Responses API:

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    "github.com/carsonfeng/harness"
    "github.com/carsonfeng/harness/model/openai"
)

func main() {
    model := openai.NewResponses(openai.Config{
        APIKey: os.Getenv("OPENAI_API_KEY"),
        Model:  os.Getenv("OPENAI_MODEL"),
    })

    agent := harness.New(
        harness.WithModel(model),
    )

    result, err := agent.Run(
        context.Background(),
        "Explain goroutines in one sentence.",
    )
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(result.Text)
}
```

Run it with:

```bash
OPENAI_API_KEY=... OPENAI_MODEL=... go run .
```

## Function calling

Use `harness.Func` to expose an ordinary Go function to the model:

```go
type WeatherArgs struct {
    City string `json:"city" description:"City name, for example Guangzhou"`
    Unit string `json:"unit,omitempty" description:"Temperature unit, for example celsius"`
}

type Weather struct {
    Temperature float64 `json:"temperature"`
    Condition   string  `json:"condition"`
}

weatherTool := harness.Func(
    "get_weather",
    "Get the weather for a city",
    func(ctx context.Context, args WeatherArgs) (Weather, error) {
        return Weather{
            Temperature: 28,
            Condition:   "sunny",
        }, nil
    },
)

if err := agent.Tool(weatherTool); err != nil {
    log.Fatal(err)
}

result, err := agent.Run(ctx, "What is the weather in Guangzhou?")
```

Harness handles the complete conversion pipeline:

```text
WeatherArgs
    ↓
JSON Schema
    ↓
Model Function Call
    ↓
JSON Argument Decoding
    ↓
Go Function
    ↓
Result Serialization
    ↓
Model Continues
```

### Argument schema rules

- Use `json` tags to define argument names.
- Fields tagged with `omitempty` are optional.
- Other exported fields are added to `required`.
- Use `description` tags to explain fields to the model.
- Unexported fields and fields tagged with `json:"-"` are ignored.
- Unknown arguments are rejected during tool execution.
- Supported types include structs, pointers, strings, booleans, numbers, arrays, slices, string-keyed maps, and interfaces.

Unsupported input types and nil functions panic when `harness.Func` is called. Register tools during application initialization so configuration errors fail early.

### Tool names

A tool name must start with an ASCII letter. Remaining characters may be:

```text
A-Z  a-z  0-9  _  -
```

Duplicate names return a registration error.

## Model adapters

Harness includes three HTTP model adapters:

| API style | Constructor | Default request URL |
|---|---|---|
| OpenAI Chat Completions | `openai.NewChatCompletions` | `https://api.openai.com/v1/chat/completions` |
| OpenAI Responses | `openai.NewResponses` | `https://api.openai.com/v1/responses` |
| Anthropic Messages | `anthropic.New` | `https://api.anthropic.com/v1/messages` |

### OpenAI Chat Completions

```go
import "github.com/carsonfeng/harness/model/openai"

model := openai.NewChatCompletions(openai.Config{
    APIKey: os.Getenv("OPENAI_API_KEY"),
    Model:  os.Getenv("OPENAI_MODEL"),
})
```

This adapter uses native `tool_calls` and `role: tool` messages.

### OpenAI Responses

```go
model := openai.NewResponses(openai.Config{
    APIKey: os.Getenv("OPENAI_API_KEY"),
    Model:  os.Getenv("OPENAI_MODEL"),
})
```

This adapter uses native `function_call` and `function_call_output` items.

### Anthropic Messages

```go
import "github.com/carsonfeng/harness/model/anthropic"

model := anthropic.New(anthropic.Config{
    APIKey:    os.Getenv("ANTHROPIC_API_KEY"),
    Model:     os.Getenv("ANTHROPIC_MODEL"),
    MaxTokens: 4096,
})
```

The adapter:

- Moves system messages to the top-level `system` field.
- Converts tool calls to `tool_use` content blocks.
- Groups consecutive tool results into the immediately following `user/tool_result` message.

`Version` defaults to `2023-06-01`; `MaxTokens` defaults to `4096`.

## Custom hosts

All built-in adapters can connect to a model gateway or local server.

`BaseURL` is an API prefix, not a complete endpoint:

```go
model := openai.NewChatCompletions(openai.Config{
    Model:   "local-model",
    BaseURL: "http://localhost:8000/v1",
})
```

The final request URL is:

```text
http://localhost:8000/v1/chat/completions
```

Do not pass this as `BaseURL`:

```text
http://localhost:8000/v1/chat/completions
```

The adapter would append the endpoint path a second time.

### Custom headers and HTTP client

```go
model := openai.NewResponses(openai.Config{
    APIKey:  token,
    Model:   "gateway-model",
    BaseURL: "https://gateway.example.com/openai/v1",
    HTTPClient: &http.Client{
        Timeout: 90 * time.Second,
    },
    Headers: map[string]string{
        "X-Tenant-ID": tenantID,
    },
})
```

The SDK does not require `APIKey` to be non-empty, which supports unauthenticated local servers and gateways that handle authentication elsewhere. Official OpenAI and Anthropic endpoints normally still require valid credentials.

When `HTTPClient` is nil, the adapter uses `http.DefaultClient`. Configure an explicit timeout for production workloads.

See [Provider configuration](docs/providers.md) for additional examples.

## Skills

A tool defines what an agent can execute. A skill explains how the agent should complete a class of tasks.

Directory layout:

```text
skills/
└── code-review/
    └── SKILL.md
```

Example `SKILL.md`:

```markdown
---
name: code-review
description: Review correctness and production risk
max_steps: 20
tools:
  - get_diff
  - get_file
  - search_code
---
# Code Review

1. Read the complete diff.
2. Identify changed functions and types.
3. Search callers and read the required context.
4. Report only correctness, security, concurrency, data-loss, and compatibility issues.
```

Load and run it:

```go
if err := agent.SkillDir("./skills"); err != nil {
    log.Fatal(err)
}

result, err := agent.RunSkill(
    ctx,
    "code-review",
    "Review MR !123",
)
```

Skill behavior:

- Only immediate child directories of the configured root are scanned.
- Each skill is stored in one `SKILL.md` file.
- Supported front-matter fields are `name`, `description`, `max_steps`, and `tools`.
- Front matter uses a small Harness-specific subset; it is not a general YAML parser.
- A non-empty `tools` list acts as an allowlist.
- Omitting `tools`, or providing an empty list, exposes all registered tools.
- `max_steps` overrides the Harness default for that run.
- Without front matter, the directory name becomes the skill name.

## Thread and Result

Every `Run` and `RunSkill` creates an independent thread:

```go
result, err := agent.Run(ctx, prompt)
if err != nil {
    log.Fatal(err)
}

fmt.Println(result.Text)
fmt.Println(result.Steps)

messages := result.Thread.Messages()
```

`Result` contains:

| Field | Description |
|---|---|
| `Text` | Final text returned by the model |
| `Steps` | Number of model calls made by the run |
| `Thread` | Complete messages, tool calls, and tool results |

`Thread.Messages()` returns a safe, mutable snapshot. The Thread itself supports concurrent access.

To persist a successful run, store `result.Thread.Messages()`. Harness currently starts a new Thread for each run and does not expose an API for resuming an existing Thread.

## Agent loop behavior

- The default limit is 20 model calls.
- Use `harness.WithMaxSteps` to change the default.
- A skill's `max_steps` takes precedence for that run.
- Multiple tool calls in one model response execute sequentially in response order.
- Tool failures are returned to the model as `{"error":"..."}` so it can retry, choose another tool, or explain the failure.
- Model failures and context cancellation stop the run immediately.
- Exceeding the limit returns `harness.ErrMaxSteps`.
- Running without a model returns `harness.ErrNoModel`.
- Tool definitions are sorted by name for deterministic model requests.

Configure system instructions and a step limit:

```go
agent := harness.New(
    harness.WithModel(model),
    harness.WithSystem("You are a concise and reliable engineering assistant."),
    harness.WithMaxSteps(30),
)
```

Finish configuring `SetModel`, `Tool`, `Skill`, and `SkillDir` before running a Harness concurrently. Do not mutate Harness configuration concurrently with active runs.

## Registering tools

Handle registration errors immediately:

```go
if err := agent.Tools(toolA, toolB); err != nil {
    log.Fatal(err)
}
```

Or register tools during construction:

```go
agent := harness.New(
    harness.WithModel(model),
    harness.WithTools(toolA, toolB),
)
```

Errors from `WithTools` are reported by the first `Run` or `RunSkill`. Prefer `Tool` or `Tools` when configuration must fail immediately during startup.

## Custom model adapters

Any model provider can implement one small interface:

```go
type Model interface {
    Generate(
        context.Context,
        ModelRequest,
    ) (ModelResponse, error)
}
```

An adapter performs four translations:

1. Convert `ModelRequest.Messages` to provider messages.
2. Convert `ModelRequest.Tools` to provider tool definitions.
3. Convert assistant text and function calls to `ModelResponse`.
4. Preserve tool-call IDs so later tool results match their calls.

Tests and lightweight adapters can use `harness.ModelFunc` directly.

## Examples

| Example | Purpose | API key required |
|---|---|---|
| `examples/weather` | Complete offline function-call loop | No |
| `examples/skill` | Load and run `SKILL.md` | No |
| `examples/openai-chat` | Minimal OpenAI Chat Completions setup | Yes |
| `examples/openai-responses` | OpenAI Responses with a local-time tool | Yes |
| `examples/anthropic` | Anthropic Messages with an addition tool | Yes |
| `examples/custom-host` | OpenAI-compatible gateway or local model | Depends on the host |

Run the offline examples:

```bash
go run ./examples/weather
go run ./examples/skill
```

`examples/skill` uses a repository-relative path and must be run from the repository root.

Run the OpenAI examples:

```bash
OPENAI_API_KEY=... OPENAI_MODEL=... \
go run ./examples/openai-chat

OPENAI_API_KEY=... OPENAI_MODEL=... \
go run ./examples/openai-responses
```

Run the Anthropic example:

```bash
ANTHROPIC_API_KEY=... ANTHROPIC_MODEL=... \
go run ./examples/anthropic
```

Run the custom-host example:

```bash
OPENAI_BASE_URL=http://localhost:8000/v1 \
OPENAI_MODEL=local-model \
go run ./examples/custom-host
```

See the [examples guide](docs/examples.md) for more details.

## Project layout

```text
harness/
├── api.go                         # Root-package public facade
├── harness.go                     # Agent loop
├── registry.go                    # Tool registration and selection
├── model/
│   ├── model.go                   # Model interface and neutral messages
│   ├── openai/                    # Chat Completions and Responses
│   └── anthropic/                 # Anthropic Messages
├── tool/                          # Generic tools and JSON Schema
├── skill/                         # SKILL.md loading
├── thread/                        # Concurrent conversation state
├── internal/httpjson/             # Shared HTTP transport
├── examples/                      # Runnable examples
├── docs/                          # Extended documentation
└── go.mod
```

Most applications only need:

```go
github.com/carsonfeng/harness
```

Provider and extension authors may import the focused packages directly:

```text
github.com/carsonfeng/harness/model
github.com/carsonfeng/harness/tool
github.com/carsonfeng/harness/skill
github.com/carsonfeng/harness/thread
```

## Current scope

Harness intentionally does not yet include:

- Streaming
- Automatic retries and backoff
- Usage or token statistics
- Every provider-specific generation option
- MCP
- RAG and vector databases
- Workflow graphs
- Multi-agent orchestration
- Built-in shell and filesystem tools
- Thread compaction and persistent storage

These capabilities can be added as independent extensions without changing the core agent loop.

## Development

Run tests:

```bash
go test ./...
```

Run the race detector:

```bash
go test -race ./...
```

Run static analysis:

```bash
go vet ./...
```

Functions use a compact contract comment format:

```go
// Run starts a new agent thread.
// @param ctx run cancellation context.
// @param input user prompt.
// @return final result or run error.
```

## Design principles

- Keep the API simple for existing Go applications.
- Keep the core small, readable, and testable.
- Decouple provider protocols from the agent loop.
- Let skills describe procedures and tools provide capabilities.
- Avoid large abstractions before real use cases require them.

## License

[MIT](LICENSE)

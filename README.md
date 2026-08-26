# Harness

**The missing agent loop for Go.**

Harness is a tiny, provider-neutral SDK for embedding tool-using AI agents in
Go programs. It connects a model to typed Go functions and keeps the agent loop,
conversation state, and reusable skills out of application code.

It is not a workflow engine, RAG framework, hosted platform, or multi-agent
system. The core has no third-party dependencies and supports Go 1.21+.

## Quick start

```go
type WeatherArgs struct {
    City string `json:"city" description:"City to look up"`
}

h := harness.New(
    harness.WithModel(model),
)

err := h.Tool(harness.Func(
    "get_weather",
    "Get the current weather",
    func(ctx context.Context, args WeatherArgs) (Weather, error) {
        return weatherClient.Get(ctx, args.City)
    },
))
if err != nil {
    log.Fatal(err)
}

result, err := h.Run(ctx, "What's the weather in Guangzhou?")
if err != nil {
    log.Fatal(err)
}
fmt.Println(result.Text)
```

The model can answer immediately or call `get_weather`. Harness executes the
function, returns its JSON result to the model, and repeats until the model
produces a final answer.

Run the self-contained example without an API key:

```bash
go run ./examples/weather
```

## Project layout

The root package is the ergonomic public facade and owns only orchestration.
Focused subpackages keep provider adapters and extensions independently usable:

```text
harness/
├── harness.go          # agent loop and public Harness
├── registry.go         # tool selection and validation
├── api.go              # single-import public facade
├── model/              # provider-neutral messages and Model interface
│   ├── openai/         # Chat Completions and Responses adapters
│   └── anthropic/      # Claude Messages adapter
├── tool/               # typed functions and JSON Schema generation
├── skill/              # SKILL.md types and loader
├── thread/             # concurrency-safe conversation state
└── examples/
    ├── weather/        # runnable function-call example
    └── skills/         # example reusable skills
```

Most applications import only `github.com/carsonfeng/harness`. Provider and
extension authors can depend on the focused subpackages without pulling in the
runner API. The package graph stays one-directional:

```text
model <- tool
model <- thread
model + tool + skill + thread <- harness
```

## Built-in model adapters

Harness includes three dependency-free HTTP adapters:

| API style | Constructor | Default endpoint |
|---|---|---|
| OpenAI Chat Completions | `openai.NewChatCompletions` | `/v1/chat/completions` |
| OpenAI Responses | `openai.NewResponses` | `/v1/responses` |
| Anthropic Messages | `anthropic.New` | `/v1/messages` |

All adapters accept `BaseURL`, a custom `http.Client`, and additional headers.
API keys are optional so local models and authenticated gateways are supported.
See [Provider configuration](docs/providers.md) for request mapping and gateway
examples.

## Examples

```bash
# No API key: deterministic function call
go run ./examples/weather

# No API key: load and run SKILL.md
go run ./examples/skill

# OpenAI Chat Completions
OPENAI_API_KEY=... OPENAI_MODEL=... go run ./examples/openai-chat

# OpenAI Responses with a typed time tool
OPENAI_API_KEY=... OPENAI_MODEL=... go run ./examples/openai-responses

# Anthropic Messages with a typed calculator tool
ANTHROPIC_API_KEY=... ANTHROPIC_MODEL=... go run ./examples/anthropic

# OpenAI-compatible gateway or local server
OPENAI_BASE_URL=http://localhost:8000/v1 OPENAI_MODEL=... go run ./examples/custom-host
```

See [Examples guide](docs/examples.md) for what each example teaches.

## Core concepts

### Model

Implement one small interface for any provider:

```go
type Model interface {
    Generate(context.Context, ModelRequest) (ModelResponse, error)
}
```

An adapter translates provider messages and function calls to Harness types.
Harness deliberately does not force a provider SDK or version on applications.
`ModelFunc` is convenient for tests and lightweight adapters.

### Typed tools

`Func` derives JSON Schema from an input struct, rejects unknown arguments, calls
the typed Go function, and serializes its result. Exported fields without
`omitempty` are required. Use a `description` tag to explain fields to the model:

```go
type SearchArgs struct {
    Query string `json:"query" description:"Literal or regular expression"`
    Path  string `json:"path,omitempty" description:"Optional directory scope"`
}
```

Supported schema types are structs, pointers, strings, booleans, numbers,
arrays/slices, string-keyed maps, and interfaces.

Tool failures are returned to the model as `{"error":"..."}` so it can recover,
choose another tool, or explain the problem. Model and context errors stop a run.

### Skills

A skill tells the agent how to perform a task; a tool gives it a capability.
Skills live in immediate child directories:

```text
skills/
└── code-review/
    └── SKILL.md
```

```markdown
---
name: code-review
description: Review a change
max_steps: 20
tools:
  - get_diff
  - get_file
  - search_code
---
# Code Review

Read the complete diff, follow changed symbols, and report correctness risks.
```

Load and run it:

```go
if err := h.SkillDir("./skills"); err != nil {
    log.Fatal(err)
}

result, err := h.RunSkill(ctx, "code-review", "Review MR !123")
```

When `tools` is present, only those tools are exposed and executable. A skill's
`max_steps` overrides the harness default for that run. Skills without front
matter are also valid; their directory name becomes the skill name.

### Thread and result

Each run creates an independent `Thread`. The returned result includes the final
text, number of model turns, and a snapshot-friendly conversation:

```go
result, err := h.Run(ctx, prompt)
messages := result.Thread.Messages()
```

`Thread` is safe for concurrent access. Tool calls and tool results remain in
the history, which makes runs inspectable and easy to persist externally.

## Model adapter guide

A provider adapter performs four translations:

1. Map `ModelRequest.Messages` to provider messages.
2. Map `ModelRequest.Tools` to provider function definitions.
3. Map assistant text and tool calls into `ModelResponse`.
4. Preserve each tool call ID when translating `RoleTool` messages back.

Keep authentication, retries, rate limits, and provider-specific settings in the
adapter. This leaves the core stable and lets applications choose SDK versions.

## Run behavior

- The default limit is 20 model turns; change it with `WithMaxSteps`.
- Multiple tool calls in one response execute in their given order.
- Cancellation is checked before every model and tool invocation.
- Tool definitions are sorted by name for deterministic requests.
- Duplicate and invalid tool names are rejected during registration.
- A run that exhausts its limit returns `ErrMaxSteps`.

Configure a system instruction for every run:

```go
h := harness.New(
    harness.WithModel(model),
    harness.WithSystem("You are a concise engineering assistant."),
    harness.WithMaxSteps(30),
)
```

## Design goals

- Small, idiomatic API that embeds into an existing Go service.
- Provider-neutral core with zero framework dependencies.
- Typed functions instead of handwritten JSON plumbing.
- Skills as readable Markdown, separate from executable capabilities.
- Enough conversation state to inspect, persist, and debug every run.

Non-goals for v0.1 include MCP, RAG, vector databases, workflow graphs,
multi-agent orchestration, and a built-in shell. These can be independent
extensions without changing the agent loop.

## Development

```bash
go test ./...
go vet ./...
```

Function comments use a compact contract format:

```go
// Run starts a new agent thread.
// @param ctx run cancellation context.
// @param input user prompt.
// @return final result or run error.
```

## License

MIT

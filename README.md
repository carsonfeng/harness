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
- MCP Streamable HTTP tools
- Custom `BaseURL`, headers, and `http.Client`
- Generic Go functions as model tools
- Automatic JSON Schema generation from Go structs
- Multiple sequential function calls
- Reusable Threads for user multi-turn conversations
- Automatic model-driven `SKILL.md` discovery, post-activation tool allowlists, and per-skill step limits
- Complete model, tool-call, and tool-result history
- Opt-in incremental debug logging with detailed Tool and Skill calls
- Context cancellation
- Go 1.21+
- Zero third-party dependencies in the core

## Installation

```bash
go get github.com/carsonfeng/harness
```

Harness requires Go 1.21 or later.

## 30-second quick start

The following program sends one request through the OpenAI Chat Completions API:

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
    model := openai.NewChatCompletions(openai.Config{
        APIKey: os.Getenv("OPENAI_API_KEY"),
        Model:  os.Getenv("OPENAI_MODEL"),
    })

    agent := harness.New(
        harness.WithModel(model),
        harness.WithDebug(os.Stderr),
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

## MCP

Connect a Streamable HTTP MCP server and register all of its tools:

```go
import "github.com/carsonfeng/harness/mcp"

server := mcp.New(mcp.Config{
    Endpoint: os.Getenv("MCP_ENDPOINT"),
    Headers: map[string]string{
        "Authorization": "Bearer " + os.Getenv("MCP_TOKEN"),
    },
})

if err := agent.MCP(ctx, server); err != nil {
    log.Fatal(err)
}

result, err := agent.Run(ctx, "Use the available tools to answer my request.")
```

`agent.MCP` discovers tools before the run and registers them like ordinary Go
tools. Harness supports current stateless MCP (`2026-07-28`) and automatically
falls back to the initialize/session lifecycle used by MCP `2025-11-25` and
earlier Streamable HTTP servers. JSON and SSE responses are supported.

Use `ToolPrefix` when several MCP servers may expose the same name:

```go
server := mcp.New(mcp.Config{
    Endpoint:   os.Getenv("MCP_ENDPOINT"),
    ToolPrefix: "database_",
})
```

Remote names that are not portable across model providers are normalized to
letters, numbers, `_`, and `-`. A normalization collision returns an error.
Tool results prefer MCP `structuredContent`; otherwise textual content is
returned to the model. MCP tool errors remain visible to the model so it can
correct its next call.

See the [MCP guide](docs/mcp.md) for authentication, multiple servers, protocol
compatibility, result handling, and security guidance.

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

Choose the adapter by endpoint protocol, not model name. If a gateway serves a
Claude model through an OpenAI-compatible API, use the matching OpenAI adapter
instead of `anthropic.New`.

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

Load the directory once, then use the normal run API:

```go
if err := agent.SkillDir("./skills"); err != nil {
    log.Fatal(err)
}

result, err := agent.Run(ctx, "Review MR !123")
```

The caller does not choose a skill for each request. Harness exposes a reserved
internal `load_skill` tool containing only the sorted skill names and
descriptions. The model decides whether a skill applies and calls `load_skill`
by itself. Harness returns the complete instructions only after that call, then
narrows the available tools and applies the skill's step budget.

Skill behavior:

- Only immediate child directories of the configured root are scanned.
- Each skill is stored in one `SKILL.md` file.
- Supported front-matter fields are `name`, `description`, `max_steps`, and `tools`.
- Front matter uses a small Harness-specific subset; it is not a general YAML parser.
- The model may continue without loading a skill when none is relevant.
- `load_skill` must be the only tool call in its model turn.
- A non-empty `tools` list acts as an allowlist after activation.
- Omitting `tools`, or providing an empty list, exposes all registered tools.
- Before activation, ordinary registered tools remain visible alongside `load_skill`. A skill allowlist scopes the workflow; it is not an authorization or security boundary.
- `max_steps` limits model calls after selection. The selection call is not charged to that budget; an already-active skill applies its budget independently to every later `RunThread` call.
- One skill can be active in a Thread. Its instructions and policy are stored in Thread history and remain active across later turns and restored history.
- Start a new Thread with `Run` when the model should choose a different skill.
- `load_skill` is reserved and cannot be registered as an application tool.
- Without front matter, the directory name becomes the skill name.

Use `agent.Skill(...)` to register an in-memory skill. This also makes the skill
discoverable; it does not select it for a request.

## Execution model

Harness uses a **synchronous agent loop**. `Run` and `RunThread` block until the model returns a final answer or the run fails:

```go
result, err := agent.Run(ctx, prompt)
```

One run executes in order:

```text
Model request
    ↓ wait
Tool call 1
    ↓ wait
Tool call 2
    ↓ wait
Next model request
    ↓ wait
Final result
```

If a model returns multiple tool calls in one turn, Harness executes them sequentially in response order. It does not create goroutines or execute tools in parallel.

In this SDK, **Thread means conversation thread**. It is the ordered history of system, user, assistant, tool-call, and tool-result messages. It is not an operating-system thread, goroutine, asynchronous task, or background worker.

The Thread type uses a mutex so `Add` and `Messages` are safe to call concurrently. This protects conversation data; it does not make the agent loop asynchronous. A caller using `RunThread` already owns the Thread and may read snapshots while a run is active, but there is no event stream or stable intermediate-state contract.

Applications may start multiple independent runs concurrently:

```go
go func() {
    resultA, err := agent.Run(ctx, "task A")
    // handle resultA and err
}()

go func() {
    resultB, err := agent.Run(ctx, "task B")
    // handle resultB and err
}()
```

Each `Run` creates its own Thread, so messages are not shared between runs. Configure the Harness before starting concurrent runs, and ensure the configured Model and Tool implementations are themselves safe for concurrent use.

Wrapping `Run` in a goroutine only moves the blocking call to the caller's goroutine. It does not provide streaming, progress events, or pause/resume. Those APIs are outside the current scope.

## Debug logging

Enable compact agent-loop logs when diagnosing a run:

```go
agent := harness.New(
    harness.WithModel(model),
    harness.WithDebug(os.Stderr),
)
```

Each model request prints only Messages added since the previous request, so a
multi-turn Thread does not repeat its full history. Available Tools are shown by
name. Tool and Skill calls include their complete names and JSON arguments.
Model text and Tool Results use a 500-character preview. Successful Skill loads
show the Skill name, allowlist, and step limit without repeating its full
instructions.

When `agent.MCP` receives an `mcp.Client`, the same `WithDebug` output also
records MCP protocol negotiation, Tool discovery, remote Tool names and full
arguments, and a 500-character result preview. MCP Endpoint URLs and request
Headers are not added as transport metadata, and Endpoint values are redacted
from transport errors.

```text
harness: 2026/08/29 12:00:00 step=1/20 event=model_request skill="" added_messages=1 total_messages=9 tools=["get_weather"]
harness: 2026/08/29 12:00:00 step=1/20 event=model_request_delta
[
  {
    "role": "user",
    "content": "How about Shenzhen?"
  }
]
harness: 2026/08/29 12:00:01 step=1/20 event=tool_call tool="get_weather" call_id="call_123"
harness: 2026/08/29 12:00:01 step=1/20 event=tool_arguments
{"arguments":{"city":"Shenzhen"},"call_id":"call_123","tool":"get_weather"}
mcp: 2026/08/31 12:00:01 event=tool_call tool="get_weather" arguments={"city":"Shenzhen"}
mcp: 2026/08/31 12:00:01 event=tool_result tool="get_weather" is_error=false result={"content":[{"type":"text","text":"28°C, sunny"}]}
```

Debug logging is disabled by default. Pass nil to `WithDebug` to disable it
explicitly. All runnable examples enable it and write to standard error. Debug
logs may contain sensitive user and Tool data; protect them accordingly.

## Thread and Result

`Run` creates a new Thread. Use `RunThread` to continue a caller-owned conversation:

```go
thread := harness.NewThread()

first, err := agent.RunThread(ctx, thread, "What is the weather in Guangzhou?")
if err != nil {
    log.Fatal(err)
}

second, err := agent.RunThread(ctx, thread, "How about Shenzhen?")
if err != nil {
    log.Fatal(err)
}

fmt.Println(first.Text)
fmt.Println(second.Text)

messages := thread.Messages()
```

`Result` contains:

| Field | Description |
|---|---|
| `Text` | Final text returned by the model |
| `Steps` | Number of model calls made by the run |
| `Thread` | Complete messages, tool calls, and tool results |

Each `RunThread` appends one user message and completes that turn's full Model → Tool → Model loop. `Result.Thread` is the same pointer supplied by the caller, and `Result.Steps` counts only the current turn.

`Thread.Messages()` returns a safe, mutable snapshot. Its concurrency protection applies to message storage, not to agent execution.

To persist a successful conversation, store `thread.Messages()` and restore it with `harness.NewThread(savedMessages...)` before calling `RunThread` again. Skill activation is part of that history. Treat restored messages as trusted application state. History currently grows without automatic compaction or context-budget management.

Harness injects its system instructions only when a Thread is empty. When restoring a non-empty Thread, its existing message history is authoritative.

Only one `RunThread` may use a Thread at a time; an overlapping call returns `harness.ErrThreadBusy`. A successful `load_skill` call keeps that skill active for later `RunThread` turns. Do not call `Thread.Add` while a run is active; `Messages` may be used for read-only snapshots during a run.

If a run fails after it has started, its Thread may contain partial history. Inspect the messages before deciding whether to recover them into a new Thread; blindly retrying a side-effecting Tool can execute it twice.

## Agent loop behavior

- The default limit is 20 model calls.
- Use `harness.WithMaxSteps` to change the default.
- An active skill's `max_steps` takes precedence for each run. On the activation turn, skill selection itself does not consume that budget.
- Multiple tool calls in one model response execute sequentially in response order.
- Tool failures are returned to the model as `{"error":"..."}` so it can retry, choose another tool, or explain the failure.
- Model failures and context cancellation stop the run immediately.
- Exceeding the limit returns `harness.ErrMaxSteps`.
- Running without a model returns `harness.ErrNoModel`.
- Passing nil to `RunThread` returns `harness.ErrNilThread`.
- Overlapping runs on one Thread return `harness.ErrThreadBusy`.
- Tool definitions are sorted by name for deterministic model requests.
- Use `WithDebug` for incremental Messages and detailed Tool or Skill calls.

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

Errors from `WithTools` are reported by the first `Run` or `RunThread`. Prefer `Tool` or `Tools` when configuration must fail immediately during startup.

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
| `examples/weather` | Chat Completions function-call loop | Depends on the host |
| `examples/skill` | Automatic skill discovery and activation | Depends on the host |
| `examples/multi-turn` | User multi-turn conversation with repeated tool calls | Depends on the host |
| `examples/openai-chat` | Minimal OpenAI Chat Completions setup | Depends on the host |
| `examples/openai-responses` | OpenAI Responses with a local-time tool | Depends on the host |
| `examples/anthropic` | Anthropic Messages with an addition tool | Depends on the host |
| `examples/custom-host` | OpenAI-compatible gateway or local model | Depends on the host |
| `examples/mcp` | Discover and call tools from an MCP server | Model and MCP host dependent |

The primary examples use OpenAI Chat Completions:

```bash
OPENAI_API_KEY=... OPENAI_MODEL=... go run ./examples/weather
OPENAI_API_KEY=... OPENAI_MODEL=... go run ./examples/skill
OPENAI_API_KEY=... OPENAI_MODEL=... go run ./examples/multi-turn
OPENAI_API_KEY=... OPENAI_MODEL=... go run ./examples/openai-chat
```

`examples/skill` uses a repository-relative path and must be run from the repository root.
Set `OPENAI_BASE_URL` on any of these commands to use a compatible custom host.

Run the Responses example:

```bash
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

Run the MCP example without putting endpoint credentials in source code:

```bash
MCP_ENDPOINT=... OPENAI_API_KEY=... OPENAI_MODEL=... \
go run ./examples/mcp
```

Optional variables are `MCP_BEARER_TOKEN`, `MCP_TOOL_PREFIX`, `MCP_PROMPT`, and
`OPENAI_BASE_URL`.

See the [examples guide](docs/examples.md) for more details.

## Project layout

```text
harness/
├── api.go                         # Root-package public facade
├── harness.go                     # Agent loop
├── registry.go                    # Tool registration and selection
├── skill_discovery.go             # Model-driven skill activation
├── model/
│   ├── model.go                   # Model interface and neutral messages
│   ├── openai/                    # Chat Completions and Responses
│   └── anthropic/                 # Anthropic Messages
├── tool/                          # Generic tools and JSON Schema
├── mcp/                           # Streamable HTTP MCP client
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
github.com/carsonfeng/harness/mcp
github.com/carsonfeng/harness/skill
github.com/carsonfeng/harness/thread
```

## Current scope

Harness intentionally does not yet include:

- Streaming
- Automatic retries and backoff
- Usage or token statistics
- Every provider-specific generation option
- MCP stdio transport
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

Automatically increment the latest SemVer patch, create an annotated tag, and
push it to GitHub:

```bash
make tag m="Add skill support"
```

The first tag is `v0.1.0`; later runs increment the patch version. Tests run
before publishing, and the working tree must be committed first.

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

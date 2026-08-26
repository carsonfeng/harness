# Provider configuration

The built-in adapters use `net/http` and add no third-party dependencies. Each
implements `model.Model`, so switching APIs does not change tools, skills, or the
agent loop.

## OpenAI Chat Completions

```go
import "github.com/carsonfeng/harness/model/openai"

model := openai.NewChatCompletions(openai.Config{
    APIKey: os.Getenv("OPENAI_API_KEY"),
    Model:  "your-model-id",
})
```

The adapter sends the full thread to `POST /chat/completions`. Harness tool
definitions become `type: function` entries; assistant calls use `tool_calls`,
and results use `role: tool` with the original `tool_call_id`.

## OpenAI Responses

```go
model := openai.NewResponses(openai.Config{
    APIKey: os.Getenv("OPENAI_API_KEY"),
    Model:  "your-model-id",
})
```

The adapter sends stateless input to `POST /responses`. Assistant calls become
`function_call` items, and tool results become `function_call_output` items.
Text is collected from `output_text` content blocks.

## Anthropic Messages

```go
import "github.com/carsonfeng/harness/model/anthropic"

model := anthropic.New(anthropic.Config{
    APIKey:   os.Getenv("ANTHROPIC_API_KEY"),
    Model:    "your-claude-model-id",
    MaxTokens: 4096,
})
```

The adapter sends system instructions through the top-level `system` field.
Assistant tool calls become `tool_use` blocks. Consecutive Harness tool results
are grouped into the immediately following `user` message as `tool_result`
blocks, as required by the Anthropic Messages protocol.

## Custom host

Set `BaseURL` to the prefix before the endpoint path:

```go
model := openai.NewChatCompletions(openai.Config{
    Model:   "local-model",
    BaseURL: "http://localhost:8000/v1",
})
```

This sends requests to `http://localhost:8000/v1/chat/completions`. The adapter
does not require an API key, which supports local servers. Gateways can add
headers and a custom transport:

```go
model := openai.NewResponses(openai.Config{
    APIKey:    token,
    Model:     "gateway-model",
    BaseURL:   "https://gateway.example.com/openai/v1",
    HTTPClient: &http.Client{Timeout: 90 * time.Second},
    Headers: map[string]string{
        "X-Tenant-ID": tenantID,
    },
})
```

Anthropic supports the same fields plus `Version` and `MaxTokens`:

```go
model := anthropic.New(anthropic.Config{
    Model:     "gateway-claude",
    BaseURL:   "https://gateway.example.com/anthropic/v1",
    Version:   "2023-06-01",
    MaxTokens: 8192,
})
```

Custom headers are copied during construction. Provider authentication headers
take precedence over entries with the same name.

## Errors and timeouts

Non-2xx errors include the status code and a bounded response body. Configure
timeouts through `HTTPClient`. Request cancellation always follows the context
passed to `Harness.Run` or `Harness.RunSkill`.

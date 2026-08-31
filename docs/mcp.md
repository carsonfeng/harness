# MCP guide

Harness turns tools from a remote Streamable HTTP MCP server into ordinary
model-visible tools. Discovery happens explicitly during application setup, so
connection and naming errors fail before the agent loop starts.

## Basic setup

```go
server := mcp.New(mcp.Config{
    Endpoint: os.Getenv("MCP_ENDPOINT"),
})

agent := harness.New(harness.WithModel(model))
if err := agent.MCP(ctx, server); err != nil {
    log.Fatal(err)
}
```

`agent.MCP` performs `tools/list`, follows pagination, converts every
`inputSchema` to a Harness Tool definition, and registers the resulting tools.
The existing Harness loop then handles model calls, MCP execution, results,
errors, Threads, Skills, step limits, cancellation, and debug logging.

## Authentication and custom HTTP

Pass credentials as headers or query parameters supplied at runtime. Do not
store secrets in source code.

```go
server := mcp.New(mcp.Config{
    Endpoint: os.Getenv("MCP_ENDPOINT"),
    Headers: map[string]string{
        "Authorization": "Bearer " + os.Getenv("MCP_TOKEN"),
    },
    HTTPClient: &http.Client{
        Timeout: 30 * time.Second,
    },
})
```

The request `context.Context` is also applied to discovery and every Tool call.
Use a context deadline even when the HTTP client has its own timeout.

## Multiple servers

Register servers one at a time and give each one a stable prefix:

```go
database := mcp.New(mcp.Config{
    Endpoint:   os.Getenv("DATABASE_MCP_ENDPOINT"),
    ToolPrefix: "database_",
})
files := mcp.New(mcp.Config{
    Endpoint:   os.Getenv("FILES_MCP_ENDPOINT"),
    ToolPrefix: "files_",
})

if err := agent.MCP(ctx, database); err != nil {
    log.Fatal(err)
}
if err := agent.MCP(ctx, files); err != nil {
    log.Fatal(err)
}
```

The prefix changes only the model-visible name. Calls still use the exact name
advertised by the remote server. Unsupported name characters are normalized;
collisions after normalization are rejected.

## Protocol behavior

Harness first sends a stateless `tools/list` request using MCP `2026-07-28`,
including the required protocol, method, client metadata, and—when applicable—
tool-name fields.
If the server requires the older lifecycle, Harness automatically performs:

```text
initialize → notifications/initialized → tools/list → tools/call
```

The negotiated protocol version and optional `Mcp-Session-Id` are retained for
later calls. Both `application/json` and `text/event-stream` responses are
accepted. Response bodies are limited to 8 MiB.

## Results and errors

When an MCP call returns `structuredContent`, Harness sends that JSON object to
the model. Otherwise it joins textual content blocks. Non-text blocks are kept
as structured content blocks when no text is available.

An MCP result with `isError: true` becomes an ordinary Harness Tool error. The
error is appended to the Thread, allowing the model to fix arguments or choose
another tool. JSON-RPC and HTTP failures also include the MCP method and Tool
name in the returned error chain.

## Debug logging

`harness.WithDebug` automatically enables MCP logging when `agent.MCP` registers
an `mcp.Client`. No second logger is required:

```go
agent := harness.New(
    harness.WithModel(model),
    harness.WithDebug(os.Stderr),
)
if err := agent.MCP(ctx, server); err != nil {
    log.Fatal(err)
}
```

MCP events include protocol negotiation, each `tools/list` page, discovered
Tool names, complete `tools/call` arguments, transport errors, and Tool results.
Results are capped at 500 characters; arguments are not truncated. Endpoint
URLs and custom request Headers are not emitted as transport metadata, and
Endpoint values are redacted from transport errors. Tool arguments and results
may themselves contain sensitive data. To use MCP logging without a Harness,
set `mcp.Config.Debug` or call `client.SetDebug`.

## Security

MCP Tools may read or modify external systems. Applications should expose only
trusted servers and use Skills to restrict which discovered Tools are available
for a task. Debug logs include full Tool arguments, so protect them as sensitive
application data.

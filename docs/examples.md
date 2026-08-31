# Examples guide

Each example focuses on one concept and is a standalone Go command.

| Directory | Concept | Provider |
|---|---|---|
| `examples/weather` | Complete function-call loop | OpenAI-compatible Chat Completions |
| `examples/skill` | Automatic `SKILL.md` discovery and activation | OpenAI-compatible Chat Completions |
| `examples/multi-turn` | Reuse one Thread across user turns and tool calls | OpenAI-compatible Chat Completions |
| `examples/openai-chat` | Minimal Chat Completions call | OpenAI-compatible Chat Completions |
| `examples/openai-responses` | Responses API with a typed tool | OpenAI-compatible Responses |
| `examples/anthropic` | Claude Messages with a typed tool | Anthropic-compatible Messages |
| `examples/custom-host` | Local server or API gateway | OpenAI-compatible Chat Completions |
| `examples/mcp` | Discover and call remote MCP tools | OpenAI-compatible Chat Completions + MCP |

Start with `weather` to see a real model → tool → model lifecycle. Run
`multi-turn` to see two user turns query a tool and a third turn use the
accumulated results. Next run `skill` to see the model call the internal
`load_skill` tool, receive the selected Markdown instructions, and invoke an
allowed application tool. The caller never selects the skill directly.

Set `OPENAI_MODEL` and, for official OpenAI, `OPENAI_API_KEY`. Set the optional
`OPENAI_BASE_URL` to use a compatible gateway or local server. The Anthropic
example uses the equivalent `ANTHROPIC_*` variables.

The MCP example reads `MCP_ENDPOINT` at runtime and never stores endpoint
credentials in source code. It also accepts `MCP_BEARER_TOKEN`,
`MCP_TOOL_PREFIX`, and `MCP_PROMPT`.

See the [MCP guide](mcp.md) for protocol compatibility and security guidance.

Every example enables `harness.WithDebug(os.Stderr)`, so model-loop progress is
visible while the final answer remains on standard output. Debug output shows
incremental Messages, full Tool arguments, and compact response/result previews.

Examples intentionally read model IDs from environment variables. This avoids
silently pinning applications to a model that may not be enabled for their
account or gateway.

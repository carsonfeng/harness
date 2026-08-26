# Examples guide

Each example focuses on one concept and is a standalone Go command.

| Directory | Concept | API key |
|---|---|---|
| `examples/weather` | Deterministic tool loop | No |
| `examples/skill` | Load and run `SKILL.md` | No |
| `examples/openai-chat` | Minimal Chat Completions call | OpenAI |
| `examples/openai-responses` | Responses API with a typed tool | OpenAI |
| `examples/anthropic` | Claude Messages with a typed tool | Anthropic |
| `examples/custom-host` | Local server or API gateway | Depends on host |

Start with `weather` to see the model → tool → model lifecycle without network
access. Next run `skill` to see how Markdown instructions and tool allowlists are
applied. Then choose the provider example that matches your production API.

Examples intentionally read model IDs from environment variables. This avoids
silently pinning applications to a model that may not be enabled for their
account or gateway.

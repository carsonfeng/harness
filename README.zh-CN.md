# Harness

[English](README.md) | **简体中文**

> The missing agent loop for Go.

Harness 是一个小型、可嵌入、与模型厂商无关的 Go Agent SDK。

它负责连接模型与普通 Go 函数，自动完成：

```text
用户输入 → 模型 → Function Call → 执行 Go 函数 → 模型 → 最终回答
```

Harness 只提供构建 Agent 最必要的能力：

- 模型抽象
- 类型安全的 Function Call
- Agent Loop
- Skill
- Thread 与上下文

它不是工作流引擎、RAG 框架、托管平台或多 Agent 系统。核心代码基于 Go 标准库，不引入第三方依赖。

## 特性

- 支持 OpenAI Chat Completions API
- 支持 OpenAI Responses API
- 支持 Anthropic Messages API
- 支持自定义 `BaseURL`、Header 和 `http.Client`
- 使用泛型将 Go 函数注册为 Tool
- 自动从 Go 结构体生成 JSON Schema
- 支持多个连续 Function Call
- 支持 `SKILL.md`、工具白名单和独立步数限制
- 保存完整的模型、工具调用和工具结果记录
- 支持 Context 取消
- Go 1.21+
- 核心零第三方依赖

## 安装

```bash
go get github.com/carsonfeng/harness
```

要求 Go 1.21 或更高版本。

## 30 秒上手

下面使用 OpenAI Responses API 完成一次普通对话：

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
        "用一句话解释 goroutine",
    )
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(result.Text)
}
```

运行：

```bash
OPENAI_API_KEY=... OPENAI_MODEL=... go run .
```

## Function Call

使用 `harness.Func` 可以把一个普通 Go 函数注册给模型：

```go
type WeatherArgs struct {
    City string `json:"city" description:"城市名称，例如 Guangzhou"`
    Unit string `json:"unit,omitempty" description:"温度单位，例如 celsius"`
}

type Weather struct {
    Temperature float64 `json:"temperature"`
    Condition   string  `json:"condition"`
}

weatherTool := harness.Func(
    "get_weather",
    "查询指定城市的天气",
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

result, err := agent.Run(ctx, "广州今天天气怎么样？")
```

Harness 自动完成：

```text
WeatherArgs
    ↓
JSON Schema
    ↓
模型 Function Call
    ↓
JSON 参数解析
    ↓
Go 函数执行
    ↓
结果序列化
    ↓
返回模型继续推理
```

### 参数 Schema 规则

- 使用 `json` 标签指定参数名称。
- 带有 `omitempty` 的字段为可选参数。
- 其他导出字段会进入 `required`。
- 使用 `description` 标签向模型解释字段含义。
- 未导出字段和 `json:"-"` 字段会被忽略。
- 模型传入未知字段时，Tool 会返回参数解析错误。
- 支持结构体、指针、字符串、布尔值、数字、数组、Slice、字符串 Key 的 Map 和 Interface。

不支持的输入类型或空函数会在调用 `harness.Func` 时触发 panic，建议在程序初始化阶段完成所有 Tool 注册。

### Tool 名称规则

名称必须以英文字母开头，后续可以使用：

```text
A-Z  a-z  0-9  _  -
```

重复名称会返回注册错误。

## 模型适配器

Harness 内置三种 HTTP 模型适配器：

| API 风格 | 构造函数 | 默认请求地址 |
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

该适配器使用原生 `tool_calls` 和 `role: tool` 消息格式。

### OpenAI Responses

```go
model := openai.NewResponses(openai.Config{
    APIKey: os.Getenv("OPENAI_API_KEY"),
    Model:  os.Getenv("OPENAI_MODEL"),
})
```

该适配器使用原生 `function_call` 和 `function_call_output` Item 格式。

### Anthropic Messages

```go
import "github.com/carsonfeng/harness/model/anthropic"

model := anthropic.New(anthropic.Config{
    APIKey:   os.Getenv("ANTHROPIC_API_KEY"),
    Model:    os.Getenv("ANTHROPIC_MODEL"),
    MaxTokens: 4096,
})
```

该适配器会：

- 把 System Message 转换为顶层 `system` 字段。
- 把 Tool Call 转换为 `tool_use` 内容块。
- 把连续的 Tool Result 合并为紧随其后的 `user/tool_result` 消息。

`Version` 默认是 `2023-06-01`，`MaxTokens` 默认是 `4096`。

## 自定义 Host

三个适配器都支持自定义模型网关或本地服务。

`BaseURL` 表示 API 前缀，不是完整 Endpoint。例如：

```go
model := openai.NewChatCompletions(openai.Config{
    Model:   "local-model",
    BaseURL: "http://localhost:8000/v1",
})
```

最终请求地址是：

```text
http://localhost:8000/v1/chat/completions
```

不要写成：

```text
http://localhost:8000/v1/chat/completions
```

否则适配器会再次追加 Endpoint 路径。

### 自定义 Header 和 HTTP Client

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

SDK 不强制要求 `APIKey`，方便接入不需要认证的本地模型或已经完成认证的内部网关。但调用 OpenAI、Anthropic 官方 Host 时，通常仍然必须提供有效凭据。

如果没有配置 `HTTPClient`，适配器会使用 `http.DefaultClient`。生产环境建议显式配置 Timeout。

详细配置见 [Provider 配置说明](docs/providers.md)。

## Skill

Tool 表示 Agent 能做什么，Skill 表示 Agent 应该怎样完成一类任务。

目录结构：

```text
skills/
└── code-review/
    └── SKILL.md
```

`SKILL.md` 示例：

```markdown
---
name: code-review
description: 审查代码正确性和生产风险
max_steps: 20
tools:
  - get_diff
  - get_file
  - search_code
---
# Code Review

1. 阅读完整 Diff。
2. 找出发生变化的函数和类型。
3. 搜索调用方并阅读必要上下文。
4. 只报告正确性、安全、并发、数据损坏和兼容性问题。
```

加载并运行：

```go
if err := agent.SkillDir("./skills"); err != nil {
    log.Fatal(err)
}

result, err := agent.RunSkill(
    ctx,
    "code-review",
    "审查 MR !123",
)
```

Skill 行为：

- 只扫描根目录的直接子目录。
- 每个 Skill 使用一个 `SKILL.md`。
- 支持 `name`、`description`、`max_steps`、`tools` 四个 Front Matter 字段。
- Front Matter 是 Harness 支持的精简格式，不是完整 YAML 实现。
- 非空 `tools` 列表是 Tool 白名单。
- 省略 `tools` 或配置空列表时，该 Skill 可以使用全部已注册 Tool。
- `max_steps` 会覆盖当前 Harness 的默认步数限制。
- 没有 Front Matter 时，目录名会作为 Skill 名称。

## Thread 与 Result

每次 `Run` 或 `RunSkill` 都会创建一个独立 Thread：

```go
result, err := agent.Run(ctx, prompt)
if err != nil {
    log.Fatal(err)
}

fmt.Println(result.Text)
fmt.Println(result.Steps)

messages := result.Thread.Messages()
```

`Result` 包含：

| 字段 | 说明 |
|---|---|
| `Text` | 模型的最终文本回答 |
| `Steps` | 本次执行调用模型的次数 |
| `Thread` | 完整对话、Tool Call 和 Tool Result |

`Thread.Messages()` 返回可以安全修改的快照，并且支持并发读取。

如需持久化会话，保存 `result.Thread.Messages()` 即可。当前 Harness 每次运行都会创建新 Thread，暂不提供继续已有 Thread 的公开接口。

## Agent Loop 行为

- 默认最多调用模型 20 次。
- 使用 `harness.WithMaxSteps` 修改默认限制。
- Skill 的 `max_steps` 优先级更高。
- 同一轮返回多个 Tool Call 时，按照模型返回顺序串行执行。
- Tool 执行失败时，将 `{"error":"..."}` 返回给模型，由模型决定重试、换工具或解释错误。
- 模型错误和 Context 取消会立即终止执行。
- 达到最大步数时返回 `harness.ErrMaxSteps`。
- 未配置模型时返回 `harness.ErrNoModel`。
- Tool 定义会按名称排序，确保发送给模型的请求稳定。

配置 System Prompt 和步数：

```go
agent := harness.New(
    harness.WithModel(model),
    harness.WithSystem("你是一个简洁、可靠的工程助手。"),
    harness.WithMaxSteps(30),
)
```

建议先完成 `SetModel`、`Tool`、`Skill` 和 `SkillDir` 等配置，再并发调用 `Run`。不要在运行过程中并发修改 Harness 配置。

## 注册方式

立即处理注册错误：

```go
if err := agent.Tools(toolA, toolB); err != nil {
    log.Fatal(err)
}
```

构造时注册：

```go
agent := harness.New(
    harness.WithModel(model),
    harness.WithTools(toolA, toolB),
)
```

`WithTools` 中的错误会延迟到第一次 `Run` 或 `RunSkill` 时返回。需要启动阶段立即失败时，优先使用 `Tool` 或 `Tools`。

## 自定义 Model

也可以接入自己的模型客户端：

```go
type Model interface {
    Generate(
        context.Context,
        ModelRequest,
    ) (ModelResponse, error)
}
```

适配器需要完成四件事：

1. 把 `ModelRequest.Messages` 转换为模型消息。
2. 把 `ModelRequest.Tools` 转换为模型 Tool 定义。
3. 把模型文本和 Function Call 转换为 `ModelResponse`。
4. 保留 Tool Call ID，使后续 Tool Result 能与调用对应。

测试或轻量适配器可以直接使用 `harness.ModelFunc`。

## 示例

| 示例 | 说明 | 是否需要 API Key |
|---|---|---|
| `examples/weather` | 无网络的完整 Function Call 流程 | 否 |
| `examples/skill` | 加载并运行 `SKILL.md` | 否 |
| `examples/openai-chat` | OpenAI Chat Completions 最小接入 | 是 |
| `examples/openai-responses` | OpenAI Responses + 本地时间 Tool | 是 |
| `examples/anthropic` | Anthropic Messages + 加法 Tool | 是 |
| `examples/custom-host` | OpenAI 兼容网关或本地模型 | 取决于服务 |

运行无网络示例：

```bash
go run ./examples/weather
go run ./examples/skill
```

`examples/skill` 使用仓库内的相对路径，请从仓库根目录运行。

运行 OpenAI 示例：

```bash
OPENAI_API_KEY=... OPENAI_MODEL=... \
go run ./examples/openai-chat

OPENAI_API_KEY=... OPENAI_MODEL=... \
go run ./examples/openai-responses
```

运行 Anthropic 示例：

```bash
ANTHROPIC_API_KEY=... ANTHROPIC_MODEL=... \
go run ./examples/anthropic
```

运行自定义 Host 示例：

```bash
OPENAI_BASE_URL=http://localhost:8000/v1 \
OPENAI_MODEL=local-model \
go run ./examples/custom-host
```

更多说明见 [示例指南](docs/examples.md)。

## 目录结构

```text
harness/
├── api.go                         # 根包公共 Facade
├── harness.go                     # Agent Loop
├── registry.go                    # Tool 注册与筛选
├── model/
│   ├── model.go                   # 模型接口与中立消息格式
│   ├── openai/                    # Chat Completions / Responses
│   └── anthropic/                 # Anthropic Messages
├── tool/                          # 泛型 Tool 与 JSON Schema
├── skill/                         # SKILL.md 加载
├── thread/                        # 并发安全的会话状态
├── internal/httpjson/             # 内置适配器共用 HTTP 传输
├── examples/                      # 可运行示例
├── docs/                          # 扩展文档
└── go.mod
```

普通业务通常只需要导入：

```go
github.com/carsonfeng/harness
```

模型适配器或扩展开发者可以单独使用：

```text
github.com/carsonfeng/harness/model
github.com/carsonfeng/harness/tool
github.com/carsonfeng/harness/skill
github.com/carsonfeng/harness/thread
```

## 当前边界

当前版本有意保持精简，暂不内置：

- Streaming
- 自动重试和退避
- Usage/Token 统计
- OpenAI 全部生成参数
- MCP
- RAG 与向量数据库
- Workflow Graph
- Multi-Agent
- 内置 Shell 和文件系统工具
- Thread 压缩与持久化存储

这些能力可以作为独立扩展加入，而不需要改变核心 Agent Loop。

## 开发

运行测试：

```bash
go test ./...
```

运行 Race 检测：

```bash
go test -race ./...
```

运行静态检查：

```bash
go vet ./...
```

项目中的函数注释使用简短契约格式：

```go
// Run starts a new agent thread.
// @param ctx run cancellation context.
// @param input user prompt.
// @return final result or run error.
```

## 设计原则

- API 简单，优先服务现有 Go 项目嵌入。
- 核心保持小型、可读、可测试。
- 模型协议与 Agent Loop 解耦。
- Skill 负责说明流程，Tool 负责执行能力。
- 不为尚未出现的需求提前引入大型抽象。

## License

[MIT](LICENSE)

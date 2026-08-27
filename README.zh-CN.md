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
- 支持通过复用 Thread 完成用户多轮对话
- 支持模型自动发现 `SKILL.md`，并在激活后应用 Tool 白名单和独立步数限制
- 保存完整的模型、工具调用和工具结果记录
- 可选的完整 Debug Logging，记录每一步请求、响应和 Tool 数据
- 支持 Context 取消
- Go 1.21+
- 核心零第三方依赖

## 安装

```bash
go get github.com/carsonfeng/harness
```

要求 Go 1.21 或更高版本。

## 30 秒上手

下面使用 OpenAI Chat Completions API 完成一次普通对话：

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

Adapter 应按 Endpoint 暴露的协议选择，而不是按模型品牌选择。如果网关通过
OpenAI 兼容 API 提供 Claude 模型，应使用对应的 OpenAI Adapter，而不是
`anthropic.New`。

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

注册 Skill 目录后，像普通请求一样运行：

```go
if err := agent.SkillDir("./skills"); err != nil {
    log.Fatal(err)
}

result, err := agent.Run(ctx, "审查 MR !123")
```

调用方不需要、也不应该为每个请求指定 Skill。Harness 使用渐进披露：尚未激活 Skill 时，模型只会通过内部 Tool `load_skill` 看到按名称排序的 Skill 名称和描述，不会提前收到完整 Instructions。模型判断某个 Skill 与任务匹配后，可以调用 `load_skill`；如果没有匹配项，也可以直接继续处理请求。

`load_skill` 成功后，完整 Instructions 会作为 Tool Result 返回给模型。从下一次模型请求开始，Harness 会应用该 Skill 的 Tool 白名单和 `max_steps`。`load_skill` 是 Harness 保留名称，应用不能注册同名 Tool。

Skill 行为：

- 只扫描根目录的直接子目录。
- 每个 Skill 使用一个 `SKILL.md`。
- 支持 `name`、`description`、`max_steps`、`tools` 四个 Front Matter 字段。
- Front Matter 是 Harness 支持的精简格式，不是完整 YAML 实现。
- 没有匹配的 Skill 时，模型可以不加载 Skill，直接继续处理请求。
- `load_skill` 必须是该次模型响应中唯一的 Tool Call。
- Skill 激活后，非空 `tools` 列表是 Tool 白名单；省略或留空时可以使用全部已注册 Tool。
- 模型选择 Skill 的那次调用不计入 `max_steps`；激活后最多再调用模型 `max_steps` 次。后续每次 `RunThread` 都按该限制执行。
- Skill 激活记录保存在 Thread 历史中，后续 `RunThread` 以及保存后恢复的 Thread 会继续使用它。
- 一个 Thread 最多激活一个 Skill；需要重新选择时，请使用新的 Thread 或调用 `Run` 开始新任务。
- 没有 Front Matter 时，目录名会作为 Skill 名称。

激活前，模型仍然可以看到普通已注册 Tool 和 `load_skill`。因此 Skill 的 `tools` 只用于激活后的工作范围收敛，不是权限控制或安全边界；敏感操作仍应在 Tool 实现中完成鉴权和校验。

也可以通过 `agent.Skill(...)` 注册内存中的 Skill。它同样可以被模型发现，但不会为某个请求预先选择该 Skill。

## 执行模型

Harness 使用的是**同步 Agent Loop**。`Run` 和 `RunThread` 都会阻塞，直到模型返回最终答案或本次运行失败：

```go
result, err := agent.Run(ctx, prompt)
```

一次运行按照以下顺序执行：

```text
请求模型
    ↓ 等待返回
执行 Tool 1
    ↓ 等待完成
执行 Tool 2
    ↓ 等待完成
再次请求模型
    ↓ 等待返回
最终结果
```

如果模型在一轮中返回多个 Tool Call，Harness 会按照返回顺序串行执行。它不会自动创建 goroutine，也不会并行执行 Tool。

在本 SDK 中，**Thread 指一次对话的消息时间线（Conversation Thread）**。它保存 System、User、Assistant、Tool Call 和 Tool Result 组成的有序消息记录。它不是操作系统线程、goroutine、异步任务或后台 Worker。

Thread 内部使用互斥锁，因此 `Add` 和 `Messages` 可以安全并发调用。这只是在保护会话数据，并不代表 Agent Loop 是异步的。使用 `RunThread` 时，调用方从一开始就持有 Thread，可以在运行中读取消息快照，但当前没有事件流，也不保证中间状态稳定。

调用方可以并发启动多个互相独立的 Run：

```go
go func() {
    resultA, err := agent.Run(ctx, "task A")
    // 处理 resultA 和 err
}()

go func() {
    resultB, err := agent.Run(ctx, "task B")
    // 处理 resultB 和 err
}()
```

每次 `Run` 都会创建自己的 Thread，不会在不同 Run 之间共享消息。并发运行前应先完成 Harness 配置，并确保使用的 Model 和 Tool 实现本身支持并发调用。

把 `Run` 放进 goroutine，只是由调用方把阻塞调用移到了另一个 goroutine，并不会自动获得 Streaming、进度事件或暂停恢复能力。这些 API 当前尚未提供。

## Debug Logging

诊断运行过程时，可以开启完整的 Agent Loop 日志：

```go
agent := harness.New(
    harness.WithModel(model),
    harness.WithDebug(os.Stderr),
)
```

每一步都会打印完整的 Provider-neutral `ModelRequest`，包括全部 Messages、Prompt、
Tool 定义和 JSON Schema；同时打印完整 `ModelResponse`、Tool Call Arguments、Tool
Results、Skill 激活、错误、完成、取消及步数上限事件。

```text
harness: 2026/08/27 12:00:00 step=1/20 event=model_request_data
{
  "messages": [{"role":"user","content":"广州天气怎么样？"}],
  "tools": [{"name":"get_weather","parameters":{"type":"object"}}]
}
harness: 2026/08/27 12:00:01 step=1/20 event=model_response_data
{"tool_calls":[{"id":"call_123","name":"get_weather","arguments":{"city":"Guangzhou"}}]}
```

Debug Logging 默认关闭。向 `WithDebug` 传入 nil 可以显式关闭。所有可运行示例都会
开启 Debug Logging，并写入标准错误输出。日志可能包含敏感的用户数据和 Tool 数据，
应按敏感日志进行存储和访问控制。

## Thread 与 Result

`Run` 会创建新 Thread。使用 `RunThread` 可以继续调用方持有的用户多轮对话：

```go
thread := harness.NewThread()

first, err := agent.RunThread(ctx, thread, "广州天气怎么样？")
if err != nil {
    log.Fatal(err)
}

second, err := agent.RunThread(ctx, thread, "那深圳呢？")
if err != nil {
    log.Fatal(err)
}

fmt.Println(first.Text)
fmt.Println(second.Text)

messages := thread.Messages()
```

`Result` 包含：

| 字段 | 说明 |
|---|---|
| `Text` | 模型的最终文本回答 |
| `Steps` | 本次执行调用模型的次数 |
| `Thread` | 完整对话、Tool Call 和 Tool Result |

每次 `RunThread` 会追加一条 User Message，并完成当前这一轮完整的 Model → Tool → Model 循环。`Result.Thread` 与传入指针相同，`Result.Steps` 只统计当前用户轮次。

`Thread.Messages()` 返回可以安全修改的快照。这里的并发保护只针对消息存储，不代表 Agent 执行是异步的。

如需持久化成功的对话，可以保存 `thread.Messages()`，再通过 `harness.NewThread(savedMessages...)` 恢复并继续调用 `RunThread`。Skill 激活状态也是历史的一部分；恢复的消息应被视为可信的应用状态。当前历史会持续增长，尚未内置压缩或上下文预算管理。

Harness 只会在空 Thread 中注入 System Instructions。恢复非空 Thread 时，已有消息历史具有优先权。

同一时间只能有一个 `RunThread` 使用某个 Thread；重叠运行会返回 `harness.ErrThreadBusy`。成功的 `load_skill` Tool Call 和 Result 会成为消息历史的一部分，因此同一 Thread 的后续用户轮次会继续使用已经激活的 Skill，而不是再次选择。一个 Thread 只能激活一个 Skill。某次 `RunThread` 执行期间不要调用 `Thread.Add`；可以通过 `Messages` 获取只读用途的消息快照。

运行开始后如果发生错误，Thread 可能留下不完整历史。再次使用前应先检查消息；对于带副作用的 Tool，盲目重试可能造成重复执行。

## Agent Loop 行为

- 默认最多调用模型 20 次。
- 使用 `harness.WithMaxSteps` 修改默认限制。
- Skill 激活后，它的 `max_steps` 优先级更高；选择 Skill 的模型调用不占用该预算。
- 同一轮返回多个 Tool Call 时，按照模型返回顺序串行执行。
- Tool 执行失败时，将 `{"error":"..."}` 返回给模型，由模型决定重试、换工具或解释错误。
- 模型错误和 Context 取消会立即终止执行。
- 达到最大步数时返回 `harness.ErrMaxSteps`。
- 未配置模型时返回 `harness.ErrNoModel`。
- `RunThread` 收到 nil 时返回 `harness.ErrNilThread`。
- 同一个 Thread 重叠运行时返回 `harness.ErrThreadBusy`。
- Tool 定义会按名称排序，确保发送给模型的请求稳定。
- 使用 `WithDebug` 可以打印模型循环每一步的请求、响应和 Tool Payload。

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

`WithTools` 中的错误会延迟到第一次 `Run` 或 `RunThread` 时返回。需要启动阶段立即失败时，优先使用 `Tool` 或 `Tools`。

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
| `examples/weather` | Chat Completions Function Call 流程 | 取决于 Host |
| `examples/skill` | 模型自动发现并激活 Skill | 取决于 Host |
| `examples/multi-turn` | 用户多轮对话并重复调用 Tool | 取决于 Host |
| `examples/openai-chat` | OpenAI Chat Completions 最小接入 | 取决于 Host |
| `examples/openai-responses` | OpenAI Responses + 本地时间 Tool | 取决于 Host |
| `examples/anthropic` | Anthropic Messages + 加法 Tool | 取决于 Host |
| `examples/custom-host` | OpenAI 兼容网关或本地模型 | 取决于 Host |

主要示例使用 OpenAI Chat Completions：

```bash
OPENAI_API_KEY=... OPENAI_MODEL=... go run ./examples/weather
OPENAI_API_KEY=... OPENAI_MODEL=... go run ./examples/skill
OPENAI_API_KEY=... OPENAI_MODEL=... go run ./examples/multi-turn
OPENAI_API_KEY=... OPENAI_MODEL=... go run ./examples/openai-chat
```

`examples/skill` 使用仓库内的相对路径，请从仓库根目录运行。

`OPENAI_BASE_URL` 是可选项；不设置时使用 OpenAI 默认 Host，设置后可以连接兼容 Chat Completions 的自定义 Host。如果自定义 Host 不要求鉴权，也可以省略 `OPENAI_API_KEY`。

运行 Responses 示例：

```bash
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

自动递增最新 SemVer 的 Patch 版本，创建 Annotated Tag 并推送到 GitHub：

```bash
make tag m="支持 Skill"
```

第一次发布使用 `v0.1.0`，后续自动递增 Patch 版本。推送前会运行测试，并要求先
提交工作区中的修改。

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

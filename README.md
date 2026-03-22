# MyClaw - AI Agent

一个基于 Go 语言构建的 AI Agent 框架，支持工具调用（Function Calling）和多轮对话。

## 项目结构

```
myagent/
├── main.go              # 应用入口
├── internal/
│   ├── agent/           # Agent 核心逻辑（对话循环、工具调度）
│   │   └── agent.go
│   ├── config/          # 配置加载
│   │   └── config.go
│   ├── llm/             # LLM 客户端（OpenAI 兼容）
│   │   └── client.go
│   ├── logger/          # 日志系统（按日期轮转）
│   │   └── logger.go
│   ├── model/           # 数据模型定义
│   │   └── types.go
│   └── tool/            # 工具系统（注册、执行）
│       ├── registry.go
│       ├── calc_tool.go
│       ├── http_tool.go
│       └── time_tool.go
├── config.json.example  # 配置文件示例
├── Makefile
├── go.mod
└── README.md
```

## 快速开始

### 1. 配置

复制示例配置文件并填入你的 API Key：

```bash
cp config.json.example config.json
```

编辑 `config.json`，填入你的 LLM API Key。也可通过环境变量设置：

```bash
export MYCLAW_API_KEY="sk-your-key"
export MYCLAW_BASE_URL="https://api.openai.com/v1"   # 可选
export MYCLAW_MODEL="gpt-4o"                          # 可选
```

### 2. 构建与运行

```bash
go build -o bin/myclaw .
./bin/myclaw
```

或直接运行：

```bash
go run .
```

### 3. 交互命令

| 命令     | 说明           |
| -------- | -------------- |
| `/quit`  | 退出程序       |
| `/exit`  | 退出程序       |
| `/clear` | 清空对话历史   |

## 添加自定义工具

实现 `tool.Tool` 接口并在 `main.go` 中注册：

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() json.RawMessage
    Execute(ctx context.Context, args string) (string, error)
}
```

注册示例：

```go
registry.Register(myNewTool)
```

## 架构说明

- **Agent**: 管理对话历史，协调 LLM 与工具之间的多轮交互
- **LLM Client**: OpenAI 兼容的 HTTP 客户端，支持自定义 Base URL
- **Tool Registry**: 统一的工具注册与执行中心
- **Config**: 支持配置文件 + 环境变量的分层配置

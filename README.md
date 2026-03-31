# MyClaw - AI Agent

一个基于 Go 语言构建的多 Agent AI 框架，支持工具调用（Function Calling）、多轮对话，并提供 Web 界面在线管理和配置多个 Agent。

## 特性

- **多 Agent 管理**：通过 Web 界面动态创建、编辑、删除多个 Agent，每个 Agent 独立配置模型、System Prompt 和工具集
- **工具调用**：内置计算器、HTTP 请求、时间查询等工具，支持自定义扩展
- **多轮对话**：自动处理 LLM 与工具之间的多轮交互，支持对话历史管理
- **Web 界面**：实时 WebSocket 通信，左侧栏切换 Agent，支持在线对话
- **CLI 交互**：同时支持命令行交互式对话
- **LLM 兼容**：兼容 OpenAI API 格式，可对接 DeepSeek、通义千问等各类模型
- **配置持久化**：Agent 配置自动保存至 `agents.json`，重启后自动恢复

## 项目结构

```
myagent/
├── main.go                       # 应用入口
├── config.json                   # 全局配置（LLM、日志、Web 服务）
├── agents.json                   # Agent 配置持久化（运行时自动生成）
├── internal/
│   ├── agent/
│   │   ├── agent.go              # Agent 核心（对话循环、工具调度）
│   │   ├── event.go              # 事件类型定义（SSE 推送）
│   │   └── manager.go            # 多 Agent 管理器（CRUD + 持久化）
│   ├── config/
│   │   └── config.go             # 配置加载（文件 + 环境变量）
│   ├── llm/
│   │   └── client.go             # LLM 客户端（OpenAI 兼容，含重试）
│   ├── logger/
│   │   └── logger.go             # 日志系统（按日期轮转）
│   ├── model/
│   │   └── types.go              # 数据模型（消息、工具定义）
│   ├── tool/
│   │   ├── registry.go           # 工具注册中心
│   │   ├── calc_tool.go          # 数学计算工具
│   │   ├── http_tool.go          # HTTP 请求工具（含 SSRF 防护）
│   │   └── time_tool.go          # 时间查询工具
│   └── web/
│       ├── handler.go            # Web 服务（REST API + WebSocket）
│       └── static/
│           └── index.html        # Web 前端界面
├── Makefile
├── go.mod
└── README.md
```

## 快速开始

### 1. 配置

编辑 `config.json`，填入你的 LLM API Key：

```json
{
  "llm": {
    "api_key": "sk-your-key",
    "base_url": "https://api.deepseek.com/v1",
    "model": "deepseek-chat"
  }
}
```

也可通过环境变量覆盖：

```bash
export MYCLAW_API_KEY="sk-your-key"
export MYCLAW_BASE_URL="https://api.openai.com/v1"
export MYCLAW_MODEL="gpt-4o"
```

### 2. 构建与运行

```bash
make build
./bin/myclaw
```

或直接运行：

```bash
make run
```

启动后访问 `http://localhost:8080` 即可打开 Web 管理界面。

### 3. Web 界面使用

1. 点击左侧栏 **+** 按钮创建新 Agent
2. 配置 Agent 的名称、模型、System Prompt、启用的工具等
3. 点击列表中的 Agent 开始对话
4. 点击 ✏️ 编辑或删除已有 Agent

### 4. CLI 交互命令

| 命令     | 说明           |
| -------- | -------------- |
| `/help`  | 显示帮助信息   |
| `/clear` | 清空对话历史   |
| `/quit`  | 退出程序       |

## API 接口

| 方法   | 路径                | 说明             |
| ------ | ------------------- | ---------------- |
| GET    | `/api/agents`       | 列出所有 Agent   |
| POST   | `/api/agents`       | 创建新 Agent     |
| GET    | `/api/agents/{id}`  | 获取 Agent 配置  |
| PUT    | `/api/agents/{id}`  | 更新 Agent 配置  |
| DELETE | `/api/agents/{id}`  | 删除 Agent       |
| GET    | `/api/tools`        | 列出可用工具     |
| WS     | `/ws?agent={id}`    | WebSocket 对话   |

## 配置说明

### config.json

```json
{
  "llm": {
    "api_key": "",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-4o"
  },
  "agents": [
    {
      "id": "default",
      "name": "MyClaw",
      "model": "",
      "system_prompt": "你是一个有用的 AI 助手。",
      "max_turns": 10,
      "tools": []
    }
  ],
  "log": {
    "dir": "log",
    "level": "INFO",
    "to_console": false
  },
  "web": {
    "enable": true,
    "addr": ":8080"
  }
}
```

- `agents` 数组中可预配置多个 Agent，`tools` 为空表示启用全部工具
- `model` 为空表示使用 `llm.model` 中的默认模型

## 内置工具

| 工具名             | 说明                                         |
| ------------------ | -------------------------------------------- |
| `get_current_time` | 获取当前时间，支持指定时区                   |
| `http_get`         | HTTP GET 请求，获取网页内容（含 SSRF 防护）  |
| `calculate`        | 数学运算：加减乘除、幂运算、开方             |

## 添加自定义工具

实现 `tool.Tool` 接口并在 `main.go` 中注册：

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() json.RawMessage
    Execute(ctx context.Context, args string) (string, error)
}

// main.go 中注册
registry.Register(myNewTool)
```

## 架构说明

- **Agent Manager**：管理多个 Agent 实例的生命周期，支持动态创建/销毁，配置自动持久化
- **Agent**：管理单个对话历史，协调 LLM 与工具之间的多轮交互
- **LLM Client**：OpenAI 兼容的 HTTP 客户端，支持指数退避重试
- **Tool Registry**：统一的工具注册与执行中心，支持为不同 Agent 分配不同工具子集
- **Web Server**：REST API 管理 Agent + WebSocket 实时对话，嵌入式静态前端

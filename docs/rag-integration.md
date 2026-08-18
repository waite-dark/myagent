# MyClaw 接入 KnowledgeBase-RAG-LLM-System 技术方案

> 状态：已评审，Phase 1 已实施（见 [实施记录](#7-实施记录)）
> 日期：2026-08-17

## 1. 背景与目标

MyClaw 是基于 Go 的多 Agent AI 框架（OpenAI 兼容，支持工具调用），
`KnowledgeBase-RAG-LLM-System` 是基于 Python（Streamlit + LangChain + Chroma）的轻量级 RAG 学习项目。

**目标**：让 MyClaw 的 Agent 具备"知识库检索增强"能力——用户提问时，Agent 可自主从知识库检索相关资料，再结合检索内容由 LLM 综合回答。

**核心矛盾**：Go 无法直接 import Python 库，需要以"进程边界 + HTTP"的方式集成。

## 2. 总体架构（方案 A：RAG 服务化 + 工具调用）

```
用户提问 ──► MyClaw Agent(Go)
                │ LLM 判断需要知识库时
                ▼
           工具 rag_search ──HTTP──► RAG 服务(FastAPI, Python, 127.0.0.1:8000)
                                        │ 复用: knowledge_base.py / vector_stores.py / config_data.py
                                        │ Embedding(text-embedding-v4) + Chroma 检索
                                        ▼
                                  返回 top-k 文档片段
                ▼
           LLM(DeepSeek) 结合片段生成回答 ──► 用户
```

**关键决策**：
- 生成（LLM 回答）**继续走 MyClaw 现有 DeepSeek 配置**；
- RAG 服务只负责**检索与入库**，因此 `rag.py` 中基于 `qwen3-max` 的 LangChain 链**不使用**，依赖减半；
- MyClaw 通过**新增工具**接入，复用现有 `tool.Tool` 接口与 `EventToolCall / EventToolResult` 事件，对话循环零侵入。

## 3. 改动清单

| # | 位置 | 类型 | 说明 |
|---|------|------|------|
| 1 | `KnowledgeBase-RAG-LLM-System/api.py` | 新增 | FastAPI 服务，包装检索/入库 |
| 2 | `internal/tool/rag_tool.go` | 新增 | MyClaw 工具 `rag_search`，调用 RAG 服务 |
| 3 | `internal/config/config.go` | 修改 | 新增 `RAGConfig` 结构 |
| 4 | `config.json` | 修改 | 新增 `rag` 配置节 |
| 5 | `main.go` | 修改 | 注册 `rag_search` 工具 |

## 4. 接口契约

### 4.1 RAG 服务（Python，`api.py`）

| 端点 | 方法 | 请求体 | 响应 |
|------|------|--------|------|
| `/api/health` | GET | - | `{"status":"ok", "collection":"rag"}` |
| `/api/kb/search` | POST | `{"query":"身高180推荐尺码","top_k":4}` | `{"hits":[{"content":"...","metadata":{"source":"..."},"score":0.83}]}` |
| `/api/kb/documents` | POST | `{"text":"...","filename":"尺码推荐.txt"}` | `{"message":"[Success]..."}` |

要点：
- 复用 `VectorStoreService`（`vector_stores.py`）与 `KnowledgeBaseService`（`knowledge_base.py`），
  直接调用 `vector_store.similarity_search_with_score(query, k=top_k)`；
- `top_k` 由请求参数控制，**覆盖** `config_data.py` 中 `similarity_threshold=1` 的缺陷（只检索 1 条太少）；
- 将 `md5_path` / `persist_directory` 改为**基于文件位置的绝对路径**，避免因工作目录不同导致向量库/去重文件找不到；
- 全局单例：避免每个请求重复加载 Chroma、重复创建 Embedding 客户端。

### 4.2 MyClaw 工具（Go，`rag_tool.go`）

```go
type RAGTool struct {
    baseURL string
    topK    int
    client  *http.Client
}
// Name()        -> "rag_search"
// Description() -> "在本地知识库中检索与问题相关的文档片段作为参考资料，用于增强回答"
// Parameters()  -> { query: string (必填), top_k?: int }
// Execute()     -> POST {baseURL}/api/kb/search，拼接片段返回给 LLM
```

返回格式（供 LLM 使用）：

```
文档片段1：...
来源：尺码推荐.txt
相关度：0.83

文档片段2：...
```

无命中时返回 `"知识库中未检索到相关资料"`，LLM 会如实告知用户。

### 4.3 配置（`config.json`）

```json
"rag": {
  "enable": true,
  "base_url": "http://127.0.0.1:8000",
  "top_k": 4
}
```

- `enable=false` 时不注册工具（可灰度关闭）；
- 每个 Agent 仍可通过 Web 界面按需勾选是否启用 `rag_search`（沿用现有工具子集机制）。

## 5. 数据流（一次完整问答）

1. 用户在 Web/CLI 提问；
2. DeepSeek 判断问题可能需要知识库 → 发起 `rag_search(query)`；
3. MyClaw 工具 `Execute` → `POST http://127.0.0.1:8000/api/kb/search`；
4. RAG 服务：`text-embedding-v4` 向量化 query → Chroma 检索 top-k 片段（含分数）；
5. 片段返回 MyClaw → 作为工具结果写入对话历史（`role: tool`）；
6. DeepSeek 结合片段 + 问题生成回答；Web 端通过 `EventToolCall / EventToolResult` 实时展示检索过程。

## 6. 实施步骤

### Phase 0 — 环境准备（手动，一次性）

```bash
# 进入 RAG 项目目录
cd KnowledgeBase-RAG-LLM-System

# 安装依赖（清华源加速）
pip install -r requirements.txt -i https://pypi.tuna.tsinghua.edu.cn/simple
pip install fastapi uvicorn
```

**配置 DashScope API Key（三选一，优先级从高到低）：**

| 方式 | 操作 | 说明 |
|------|------|------|
| 1) 工程直填（推荐） | 编辑 `config_data.py`：`dashscope_api_key = "sk-xxx"` | 最省事，改完即生效 |
| 2) `.env` 文件 | `copy .env.example .env`，填入 `DASHSCOPE_API_KEY=sk-xxx` | 密钥不进代码库（`.env` 已在 `.gitignore`） |
| 3) 环境变量 | `setx DASHSCOPE_API_KEY "sk-xxx"`（需重开终端） | 传统方式 |

> 说明：若方式 1 直填了 Key，`api.py` 会自动把它写入进程环境变量，保证 `knowledge_base.py` 内部硬编码构造的 Embedding 实例也能读到（它只读环境变量）。

### Phase 1 — 打通检索链路（代码已就绪）

1. 启动 RAG 服务（绑定本机）：
   ```bash
   uvicorn api:app --host 127.0.0.1 --port 8000
   ```
2. 验证健康检查：`curl http://127.0.0.1:8000/api/health`
3. 灌入示例文档（用仓库自带素材）：
   ```bash
   curl -X POST http://127.0.0.1:8000/api/kb/documents -H "Content-Type: application/json" ^
     -d "{\"text\":\"$(Get-Content -Raw assets/尺码推荐.txt)\",\"filename\":\"尺码推荐.txt\"}"
   ```
4. 验证检索：
   ```bash
   curl -X POST http://127.0.0.1:8000/api/kb/search -H "Content-Type: application/json" ^
     -d '{"query":"身高180推荐尺码","top_k":3}'
   ```
5. 构建并运行 MyClaw：`go build -o bin/myclaw .`，CLI 提问验证 RAG 问答。

### Phase 2 — 收尾（可选）

- Web 界面 Agent 配置页勾选 `rag_search`，验证流式问答；
- 将知识库管理（上传/删除/统计）做成 MyClaw 前端页面，或直接链到 Streamlit 页面。

## 7. 实施记录

| 日期 | 内容 | 状态 |
|------|------|------|
| 2026-08-17 | 方案评审通过（架构：方案 A 服务化 + 工具调用） | ✅ |
| 2026-08-17 | Phase 1 代码：api.py / rag_tool.go / config / main.go | ✅ |

## 8. 遗留决策与注意事项

### 8.1 Embedding 选型（待定）

原项目使用阿里云 DashScope `text-embedding-v4`（需 `DASHSCOPE_API_KEY`，有免费额度）。
备选：

| 方案 | 说明 | 改动点 |
|------|------|--------|
| DashScope text-embedding-v4（默认） | 项目原配，中文效果好 | 无 |
| SiliconFlow BGE 系列（OpenAI 兼容） | 与 DeepSeek 生态一致，免阿里云账号 | 改 `api.py` 中 Embedding 构造为 `langchain_openai.OpenAIEmbeddings(base_url=...)` |

### 8.2 注意事项

- **安全**：RAG 服务只绑 `127.0.0.1`，不对外暴露；如需鉴权可在 `api.py` 加简单 token 校验；
- **上下文预算**：检索片段由现有 `trimSessionByTokens` 兜底，避免长会话超 token 上限；
- **检索质量**：`config_data.py` 的 `chunk_size=1000 / overlap=100` 可按业务文本调整；
- **一致性**：上传与检索必须使用**同一个** `persist_directory` 与 `collection_name`（本项目均为 `./chroma_db` / `rag`，已由绝对路径修正规避工作目录问题）。

## 9. 相关文件

- RAG 项目：`KnowledgeBase-RAG-LLM-System/{api.py, knowledge_base.py, vector_stores.py, config_data.py}`
- MyClaw：`internal/tool/rag_tool.go`、`internal/config/config.go`、`config.json`、`main.go`

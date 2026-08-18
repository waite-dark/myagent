package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// RAGTool 知识库检索工具：调用外部 RAG 服务（FastAPI + Chroma）
// 检索结果作为参考资料返回给 LLM，用于增强回答。
type RAGTool struct {
	baseURL string
	topK    int
	client  *http.Client
}

// NewRAGTool 创建 RAG 检索工具
func NewRAGTool(baseURL string, topK int) *RAGTool {
	if topK <= 0 {
		topK = 4
	}
	return &RAGTool{
		baseURL: strings.TrimRight(baseURL, "/"),
		topK:    topK,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (t *RAGTool) Name() string {
	return "rag_search"
}

func (t *RAGTool) Description() string {
	return "在本地知识库中检索与问题相关的文档片段，作为参考资料增强回答。当用户的问题可能涉及知识库内容时使用。"
}

func (t *RAGTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "要检索的问题或关键词"
			},
			"top_k": {
				"type": "integer",
				"description": "返回的文档片段数量，默认 4"
			}
		},
		"required": ["query"]
	}`)
}

type ragArgs struct {
	Query string `json:"query"`
	TopK  int    `json:"top_k"`
}

// 与 api.py 的请求/响应结构对齐
type ragSearchRequest struct {
	Query string `json:"query"`
	TopK  int    `json:"top_k"`
}

type ragHit struct {
	Content  string         `json:"content"`
	Metadata map[string]any `json:"metadata"`
	Score    float64        `json:"score"`
}

type ragSearchResponse struct {
	Hits []ragHit `json:"hits"`
}

func (t *RAGTool) Execute(ctx context.Context, args string) (string, error) {
	// LLM 可能返回空参数（参数生成不完整等情况），给出友好提示
	if strings.TrimSpace(args) == "" || args == "{}" {
		return "", fmt.Errorf("参数缺失: 请提供 query 参数")
	}
	var a ragArgs
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	if a.Query == "" {
		return "", fmt.Errorf("query 不能为空")
	}

	topK := a.TopK
	if topK <= 0 {
		topK = t.topK
	}

	body, err := json.Marshal(ragSearchRequest{Query: a.Query, TopK: topK})
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}

	url := t.baseURL + "/api/kb/search"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("RAG 服务请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024)) // 最大 2MB
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("RAG 服务返回错误 (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var rsp ragSearchResponse
	if err := json.Unmarshal(respBody, &rsp); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	if len(rsp.Hits) == 0 {
		return "知识库中未检索到相关资料", nil
	}

	// 拼接片段返回给 LLM
	var sb strings.Builder
	for i, hit := range rsp.Hits {
		sb.WriteString(fmt.Sprintf("文档片段%d：%s\n", i+1, hit.Content))
		if src, ok := hit.Metadata["source"]; ok {
			sb.WriteString(fmt.Sprintf("来源：%v\n", src))
		}
		sb.WriteString(fmt.Sprintf("相关度：%.4f\n\n", hit.Score))
	}
	return sb.String(), nil
}

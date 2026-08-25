package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"myagent/internal/logger"
	"myagent/internal/model"
)

// Client LLM 客户端接口
type Client interface {
	Chat(ctx context.Context, messages []model.Message, tools []model.ToolDefinition) (*model.ChatResponse, error)
}

// StreamChunk 流式输出的单个块
type StreamChunk struct {
	Content   string           // 文本增量
	ToolCalls []model.ToolCall // 工具调用增量
}

// StreamClient 支持流式输出的 LLM 客户端接口
type StreamClient interface {
	Client
	ChatStream(ctx context.Context, messages []model.Message, tools []model.ToolDefinition, onChunk func(StreamChunk)) error
}

// ClientConfig LLM 客户端配置
type ClientConfig struct {
	APIKey     string
	BaseURL    string
	Model      string
	MaxRetries int
}

// OpenAIClient OpenAI 兼容的客户端
type OpenAIClient struct {
	apiKey     string
	baseURL    string
	model      string
	maxRetries int
	http       *http.Client
}

// NewOpenAIClient 创建 OpenAI 客户端
func NewOpenAIClient(cfg ClientConfig) *OpenAIClient {
	maxRetries := cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}
	return &OpenAIClient{
		apiKey:     cfg.APIKey,
		baseURL:    cfg.BaseURL,
		model:      cfg.Model,
		maxRetries: maxRetries,
		http: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// NewOpenAIClientFromBase 从 ClientConfig 创建 OpenAI 客户端（便捷接口）
func NewOpenAIClientFromBase(cfg ClientConfig) *OpenAIClient {
	return NewOpenAIClient(cfg)
}

// Chat 发送聊天请求（含指数退避重试）
func (c *OpenAIClient) Chat(ctx context.Context, messages []model.Message, tools []model.ToolDefinition) (*model.ChatResponse, error) {
	reqBody := model.ChatRequest{
		Model:    c.model,
		Messages: messages,
	}
	if len(tools) > 0 {
		reqBody.Tools = tools
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < c.maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(math.Pow(2, float64(attempt-1))) * time.Second
			logger.Warnf("LLM 请求重试 %d/%d，等待 %v", attempt, c.maxRetries, delay)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		resp, err := c.doRequest(ctx, body)
		if err == nil {
			logger.Infof("LLM 响应成功: tokens=%d (prompt=%d, completion=%d)",
				resp.Usage.TotalTokens, resp.Usage.PromptTokens, resp.Usage.CompletionTokens)
			return resp, nil
		}
		lastErr = err

		// 仅对可重试的错误进行重试（5xx、429、网络超时）
		if !isRetryable(err) {
			logger.Errorf("LLM 不可重试错误: %v", err)
			return nil, err
		}
	}
	logger.Errorf("LLM 请求最终失败（已重试 %d 次）: %v", c.maxRetries, lastErr)
	return nil, fmt.Errorf("请求失败（已重试 %d 次）: %w", c.maxRetries, lastErr)
}

func (c *OpenAIClient) doRequest(ctx context.Context, body []byte) (*model.ChatResponse, error) {
	url := c.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, &retryableError{err: fmt.Errorf("请求失败: %w", err)}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024)) // 最大 4MB
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode >= 500 {
		return nil, &retryableError{err: fmt.Errorf("API 服务端错误 (HTTP %d): %s", resp.StatusCode, string(respBody))}
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, &retryableError{err: fmt.Errorf("API 限流 (HTTP 429): %s", string(respBody))}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API 返回错误 (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var chatResp model.ChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	return &chatResp, nil
}

// retryableError 可重试的错误
type retryableError struct {
	err error
}

func (e *retryableError) Error() string { return e.err.Error() }
func (e *retryableError) Unwrap() error { return e.err }

func isRetryable(err error) bool {
	_, ok := err.(*retryableError)
	return ok
}

// ChatStream 流式调用 LLM（SSE）
func (c *OpenAIClient) ChatStream(ctx context.Context, messages []model.Message, tools []model.ToolDefinition, onChunk func(StreamChunk)) error {
	reqBody := model.ChatRequest{
		Model:    c.model,
		Messages: messages,
		Stream:   true,
	}
	if len(tools) > 0 {
		reqBody.Tools = tools
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("序列化请求失败: %w", err)
	}

	url := c.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
		return fmt.Errorf("API 返回错误 (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk streamResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			logger.Debugf("解析 stream chunk 失败: %v", err)
			continue
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta
		sc := StreamChunk{}
		if delta.Content != "" {
			sc.Content = delta.Content
		}
		if len(delta.ToolCalls) > 0 {
			for _, tc := range delta.ToolCalls {
				sc.ToolCalls = append(sc.ToolCalls, model.ToolCall{
					Index: tc.Index,
					ID:    tc.ID,
					Type:  tc.Type,
					Function: model.FunctionCall{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				})
			}
		}
		if sc.Content != "" || len(sc.ToolCalls) > 0 {
			onChunk(sc)
		}
	}

	return scanner.Err()
}

// streamResponse SSE stream 响应结构
type streamResponse struct {
	Choices []streamChoice `json:"choices"`
}

type streamChoice struct {
	Delta        streamDelta `json:"delta"`
	FinishReason *string     `json:"finish_reason"`
}

type streamDelta struct {
	Role      string           `json:"role,omitempty"`
	Content   string           `json:"content,omitempty"`
	ToolCalls []streamToolCall `json:"tool_calls,omitempty"`
}

type streamToolCall struct {
	Index    int                `json:"index"`
	ID       string             `json:"id,omitempty"`
	Type     string             `json:"type,omitempty"`
	Function streamFunctionCall `json:"function,omitempty"`
}

type streamFunctionCall struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

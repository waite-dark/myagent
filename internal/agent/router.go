package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"myagent/internal/llm"
	"myagent/internal/model"
)

// ---------- 路由策略 ----------

// Router 路由策略：根据用户输入决定交给哪个 Agent 处理
type Router interface {
	// Route 返回目标 Agent 的 ID，空字符串表示无法决定
	Route(ctx context.Context, userMsg string, agents []*AgentConfig) (string, error)
}

// RouterType 路由策略类型
type RouterType string

const (
	RouterLLM        RouterType = "llm"         // LLM 判断路由
	RouterKeyword    RouterType = "keyword"     // 关键词匹配路由
	RouterRoundRobin RouterType = "round_robin" // 轮询路由
	RouterFirstMatch RouterType = "first_match" // 匹配 Agent name 路由
)

// RouterConfig 路由配置
type RouterConfig struct {
	Type     RouterType        `json:"type"`                // 路由策略
	Keywords map[string]string `json:"keywords,omitempty"`  // keyword 路由的关键词表: keyword → agentID
	LLMModel string            `json:"llm_model,omitempty"` // llm 路由使用的模型
}

// ---------- LLM 路由 ----------

// LLMRouter 用 LLM 判断用户意图，路由到最合适的 Agent
type LLMRouter struct {
	llm   llm.Client
	model string
}

// NewLLMRouter 创建 LLM 路由
func NewLLMRouter(lm llm.Client, model string) *LLMRouter {
	return &LLMRouter{llm: lm, model: model}
}

func (r *LLMRouter) Route(ctx context.Context, userMsg string, agents []*AgentConfig) (string, error) {
	if len(agents) == 0 {
		return "", fmt.Errorf("无可用 Agent")
	}
	if len(agents) == 1 {
		return agents[0].ID, nil
	}

	// 构建 Agent 列表描述
	var agentDesc strings.Builder
	agentDesc.WriteString("可用的 Agent 列表：\n")
	for _, a := range agents {
		agentDesc.WriteString(fmt.Sprintf("- ID: %s, 名称: %s, 描述: %s\n", a.ID, a.Name, truncate(a.SystemPrompt, 100)))
	}
	agentDesc.WriteString(fmt.Sprintf("\n用户消息: %s\n\n请根据用户消息的意图，选择最合适的 Agent ID。只返回 Agent ID，不要其他内容。", userMsg))

	msgs := []model.Message{
		{Role: "system", Content: "你是一个智能路由分发器。根据用户的消息内容和每个 Agent 的系统提示词，选择最合适的 Agent 来处理该消息。"},
		{Role: "user", Content: agentDesc.String()},
	}

	resp, err := r.llm.Chat(ctx, msgs, nil)
	if err != nil {
		return "", fmt.Errorf("LLM 路由决策失败: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("LLM 路由返回空响应")
	}

	targetID := strings.TrimSpace(resp.Choices[0].Message.Content)
	// 验证返回的 ID 是否有效
	for _, a := range agents {
		if a.ID == targetID {
			return targetID, nil
		}
	}
	// 如果 LLM 返回了无效 ID，返回第一个可用 Agent
	return agents[0].ID, nil
}

// ---------- 关键词路由 ----------

// KeywordRouter 根据关键词表匹配路由
type KeywordRouter struct {
	keywords map[string]string // keyword → agentID
}

// NewKeywordRouter 创建关键词路由
func NewKeywordRouter(keywords map[string]string) *KeywordRouter {
	return &KeywordRouter{keywords: keywords}
}

func (r *KeywordRouter) Route(_ context.Context, userMsg string, agents []*AgentConfig) (string, error) {
	if len(agents) == 0 {
		return "", fmt.Errorf("无可用 Agent")
	}

	msg := strings.ToLower(userMsg)
	// 优先匹配更长的关键词（更精确）
	var bestMatch string
	var bestLen int
	for keyword, agentID := range r.keywords {
		if strings.Contains(msg, strings.ToLower(keyword)) {
			// 验证 agentID 是否存在
			for _, a := range agents {
				if a.ID == agentID && len(keyword) > bestLen {
					bestMatch = agentID
					bestLen = len(keyword)
					break
				}
			}
		}
	}
	if bestMatch != "" {
		return bestMatch, nil
	}
	return agents[0].ID, nil // 默认第一个
}

// ---------- 轮询路由 ----------

// RoundRobinRouter 轮询路由
type RoundRobinRouter struct {
	mu    sync.Mutex
	index int
}

// NewRoundRobinRouter 创建轮询路由
func NewRoundRobinRouter() *RoundRobinRouter {
	return &RoundRobinRouter{}
}

func (r *RoundRobinRouter) Route(_ context.Context, _ string, agents []*AgentConfig) (string, error) {
	if len(agents) == 0 {
		return "", fmt.Errorf("无可用 Agent")
	}
	r.mu.Lock()
	idx := r.index % len(agents)
	r.index++
	r.mu.Unlock()
	return agents[idx].ID, nil
}

// ---------- 名称匹配路由 ----------

// FirstMatchRouter 根据 Agent name 前缀匹配路由
type FirstMatchRouter struct{}

// NewFirstMatchRouter 创建名称匹配路由
func NewFirstMatchRouter() *FirstMatchRouter {
	return &FirstMatchRouter{}
}

func (r *FirstMatchRouter) Route(_ context.Context, userMsg string, agents []*AgentConfig) (string, error) {
	if len(agents) == 0 {
		return "", fmt.Errorf("无可用 Agent")
	}
	msg := strings.ToLower(userMsg)
	for _, a := range agents {
		name := strings.ToLower(a.Name)
		if strings.Contains(msg, name) {
			return a.ID, nil
		}
	}
	return agents[0].ID, nil
}

// ---------- Router Factory ----------

// NewRouterFromConfig 根据配置创建路由策略
func NewRouterFromConfig(cfg RouterConfig, lm llm.Client) Router {
	switch cfg.Type {
	case RouterLLM:
		if lm == nil {
			// fallback 到 first_match
			return NewFirstMatchRouter()
		}
		return NewLLMRouter(lm, cfg.LLMModel)
	case RouterKeyword:
		if cfg.Keywords == nil {
			cfg.Keywords = make(map[string]string)
		}
		return NewKeywordRouter(cfg.Keywords)
	case RouterRoundRobin:
		return NewRoundRobinRouter()
	case RouterFirstMatch:
		return NewFirstMatchRouter()
	default:
		return NewFirstMatchRouter()
	}
}

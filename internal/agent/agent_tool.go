package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"myagent/internal/model"
)

// 工具名只允许 [a-zA-Z0-9_-]，对中文 ID 做转义
var safeNameRe = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

func safeToolName(s string) string {
	// 替换非 ASCII 安全字符为下划线，连续多个合并为一个
	result := safeNameRe.ReplaceAllString(s, "_")
	result = strings.Trim(result, "_")
	if result == "" {
		result = "agent"
	}
	return result
}

// AgentTool 把其他 Agent 包装成工具，让当前 Agent 能调用另一个 Agent
type AgentTool struct {
	manager  *Manager
	targetID string
}

// NewAgentTool 创建 Agent 互调工具
func NewAgentTool(manager *Manager, targetID string) *AgentTool {
	return &AgentTool{manager: manager, targetID: targetID}
}

func (t *AgentTool) Name() string {
	return fmt.Sprintf("call_agent_%s", safeToolName(t.targetID))
}

func (t *AgentTool) Description() string {
	cfg, ok := t.manager.GetConfig(t.targetID)
	if !ok {
		return fmt.Sprintf("调用 Agent %s", t.targetID)
	}
	return fmt.Sprintf("将任务委托给「%s」Agent 处理。%s", cfg.Name, cfg.SystemPrompt)
}

func (t *AgentTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "需要委托给该 Agent 处理的问题或指令"
			}
		},
		"required": ["query"]
	}`)
}

func (t *AgentTool) Execute(ctx context.Context, args string) (string, error) {
	var params struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("解析参数失败: %w", err)
	}
	if params.Query == "" {
		return "", fmt.Errorf("query 参数不能为空")
	}

	ag, ok := t.manager.GetAgent(t.targetID)
	if !ok {
		return "", fmt.Errorf("目标 Agent %q 不存在", t.targetID)
	}

	// 使用 Agent 的 Chat 方法执行委托
	result, err := ag.Chat(ctx, params.Query)
	if err != nil {
		return "", fmt.Errorf("Agent[%s] 执行失败: %w", t.targetID, err)
	}
	return result, nil
}

// ---------- 注册 Agent 互调工具到 Manager ----------

// RegisterAgentCallTools 为每个 Agent 注册其他 Agent 的调用工具
// 注意：只注册到 Agent 的子注册表，不持久化到 agents.json
func (m *Manager) RegisterAgentCallTools() error {
	m.mu.RLock()
	agentIDs := make([]string, 0, len(m.agents))
	for id := range m.agents {
		agentIDs = append(agentIDs, id)
	}
	m.mu.RUnlock()

	if len(agentIDs) <= 1 {
		return nil // 只有一个 Agent，无需互调
	}

	// 为每个 Agent 注册指向其他 Agent 的调用工具
	for _, srcID := range agentIDs {
		ag, ok := m.GetAgent(srcID)
		if !ok {
			continue
		}

		for _, dstID := range agentIDs {
			if srcID == dstID {
				continue
			}
			agentTool := NewAgentTool(m, dstID)
			ag.tools.RegisterSilent(agentTool)
		}
	}

	return nil
}

// ---------- 序列化工具 ----------

// ensure AgentTool implements model.ToolDefinition provider
var _ = model.ToolDefinition{}

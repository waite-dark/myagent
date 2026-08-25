package agent

import (
	"context"
	"fmt"
	"time"

	"myagent/internal/llm"
	"myagent/internal/logger"
	"myagent/internal/model"
)

// Orchestrator 多 Agent 编排器：路由 + 上下文共享 + 结果聚合
type Orchestrator struct {
	manager   *Manager
	router    Router
	sharedCtx *SharedContext
}

// SharedContext 多 Agent 共享上下文
type SharedContext struct {
	conversationSummary string   // 对话摘要
	keyFindings         []string // 关键发现
	lastAgentID         string   // 最后处理的 Agent ID
	updatedAt           time.Time
}

// OrchestratorConfig 编排器配置
type OrchestratorConfig struct {
	RouterConfig RouterConfig `json:"router"`
	Enabled      bool         `json:"enabled"` // 是否启用编排
}

// NewOrchestrator 创建编排器
func NewOrchestrator(manager *Manager, cfg RouterConfig) *Orchestrator {
	o := &Orchestrator{
		manager:   manager,
		sharedCtx: &SharedContext{updatedAt: time.Now()},
	}

	o.router = NewRouterFromConfig(cfg, llm.NewOpenAIClientFromBase(manager.GetLLMBase()))
	return o
}

// RouteAndChat 路由 + 对话：判断交给哪个 Agent，执行并返回结果
func (o *Orchestrator) RouteAndChat(ctx context.Context, userMsg string, session *Session) (string, error) {
	agents := o.manager.ListConfigs()
	if len(agents) == 0 {
		return "", fmt.Errorf("无可用 Agent")
	}

	targetID, err := o.router.Route(ctx, userMsg, agents)
	if err != nil {
		// 路由失败时使用第一个 Agent
		targetID = agents[0].ID
	}

	ag, ok := o.manager.GetAgent(targetID)
	if !ok {
		return "", fmt.Errorf("路由目标 Agent %q 不存在", targetID)
	}

	logger.Infof("[orchestrator] 路由: %q -> Agent[%s]", truncate(userMsg, 50), targetID)

	// 注入共享上下文到 session
	o.injectSharedContext(session)

	// 执行对话
	result, err := ag.ChatSessionStream(ctx, session, userMsg, func(e Event) {
		// 透传事件
	})
	if err != nil {
		return "", fmt.Errorf("Agent[%s] 对话失败: %w", targetID, err)
	}

	// 更新共享上下文
	o.sharedCtx.lastAgentID = targetID
	o.sharedCtx.updatedAt = time.Now()

	return result, nil
}

// RouteAndChatWithEvents 路由 + 对话（带事件回调）
func (o *Orchestrator) RouteAndChatWithEvents(ctx context.Context, userMsg string, session *Session, onEvent func(Event)) (string, error) {
	agents := o.manager.ListConfigs()
	if len(agents) == 0 {
		return "", fmt.Errorf("无可用 Agent")
	}

	emit := func(e Event) {
		if onEvent != nil {
			onEvent(e)
		}
	}

	// 步骤1: 路由决策
	emit(Event{Type: EventThinking})
	targetID, err := o.router.Route(ctx, userMsg, agents)
	if err != nil {
		targetID = agents[0].ID
	}

	ag, ok := o.manager.GetAgent(targetID)
	if !ok {
		emit(Event{Type: EventError, Content: fmt.Sprintf("路由目标 Agent %q 不存在", targetID)})
		return "", fmt.Errorf("Agent %q 不存在", targetID)
	}

	// 通知前端路由结果
	emit(Event{Type: EventRoute, Content: targetID, Name: "route"})
	logger.Infof("[orchestrator] 路由: %q -> Agent[%s]", truncate(userMsg, 50), targetID)

	// 步骤2: 注入共享上下文
	o.injectSharedContext(session)

	// 步骤3: 执行对话
	result, err := ag.ChatSessionStream(ctx, session, userMsg, onEvent)
	if err != nil {
		emit(Event{Type: EventError, Content: fmt.Sprintf("Agent[%s] 对话失败: %v", targetID, err)})
		return "", fmt.Errorf("Agent[%s] 对话失败: %w", targetID, err)
	}

	o.sharedCtx.lastAgentID = targetID
	o.sharedCtx.updatedAt = time.Now()
	return result, nil
}

// injectSharedContext 向 session 注入共享上下文信息
func (o *Orchestrator) injectSharedContext(session *Session) {
	// 如果共享上下文有内容，作为 system 补充注入
	if o.sharedCtx.conversationSummary != "" {
		summary := fmt.Sprintf("[跨 Agent 共享上下文]\n对话摘要: %s\n上次处理的 Agent: %s",
			o.sharedCtx.conversationSummary, o.sharedCtx.lastAgentID)
		session.AddMessage(model.Message{
			Role:    "system",
			Content: summary,
		})
	}
}

// GetSharedContext 获取共享上下文
func (o *Orchestrator) GetSharedContext() *SharedContext {
	return o.sharedCtx
}

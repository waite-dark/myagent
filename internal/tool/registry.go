package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"myagent/internal/logger"
	"myagent/internal/model"
)

// Tool 工具接口
type Tool interface {
	Name() string
	Description() string
	Parameters() json.RawMessage
	Execute(ctx context.Context, args string) (string, error)
}

// Registry 工具注册中心
type Registry struct {
	tools map[string]Tool
}

// NewRegistry 创建工具注册中心
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register 注册工具
func (r *Registry) Register(t Tool) {
	logger.Infof("注册工具: %s", t.Name())
	r.tools[t.Name()] = t
}

// RegisterSilent 注册工具（不输出日志，用于构建子注册表）
func (r *Registry) RegisterSilent(t Tool) {
	r.tools[t.Name()] = t
}

// Get 获取工具
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// Execute 执行工具
func (r *Registry) Execute(ctx context.Context, name string, args string) (string, error) {
	t, ok := r.tools[name]
	if !ok {
		return "", fmt.Errorf("工具 %q 未注册", name)
	}
	return t.Execute(ctx, args)
}

// Definitions 返回所有工具的定义（用于 LLM 请求）
func (r *Registry) Definitions() []model.ToolDefinition {
	defs := make([]model.ToolDefinition, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, model.ToolDefinition{
			Type: "function",
			Function: model.FunctionDefinition{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.Parameters(),
			},
		})
	}
	return defs
}

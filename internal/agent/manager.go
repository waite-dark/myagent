package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"myagent/internal/config"
	"myagent/internal/llm"
	"myagent/internal/logger"
	"myagent/internal/model"
	"myagent/internal/tool"
)

// AgentConfig 单个 Agent 的配置（用于持久化和 API）
type AgentConfig struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Model        string   `json:"model"`
	SystemPrompt string   `json:"system_prompt"`
	MaxTurns     int      `json:"max_turns"`
	Tools        []string `json:"tools"` // 启用的工具名称列表
}

// Manager 管理多个 Agent 实例
type Manager struct {
	mu       sync.RWMutex
	agents   map[string]*Agent
	configs  map[string]*AgentConfig
	llmBase  llm.ClientConfig // LLM 基础配置（APIKey、BaseURL）
	registry *tool.Registry
	savePath string
	cacheDir string // 对话历史缓存目录
}

// NewManager 创建 Agent 管理器
func NewManager(llmBase llm.ClientConfig, registry *tool.Registry, savePath string) *Manager {
	cacheDir := "cache"
	os.MkdirAll(cacheDir, 0o755)
	m := &Manager{
		agents:   make(map[string]*Agent),
		configs:  make(map[string]*AgentConfig),
		llmBase:  llmBase,
		registry: registry,
		savePath: savePath,
		cacheDir: cacheDir,
	}
	return m
}

// LoadFromFile 从文件加载已保存的 Agent 配置
func (m *Manager) LoadFromFile() error {
	data, err := os.ReadFile(m.savePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("读取 agent 配置文件失败: %w", err)
	}

	var configs []*AgentConfig
	if err := json.Unmarshal(data, &configs); err != nil {
		return fmt.Errorf("解析 agent 配置失败: %w", err)
	}

	for _, cfg := range configs {
		if err := m.buildAgent(cfg); err != nil {
			logger.Errorf("加载 agent %s 失败: %v", cfg.ID, err)
		}
	}
	return nil
}

// SaveToFile 保存所有 Agent 配置到文件（线程安全）
func (m *Manager) SaveToFile() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.saveConfigs()
}

// CreateAgent 创建并注册新 Agent
func (m *Manager) CreateAgent(cfg *AgentConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.configs[cfg.ID]; exists {
		return fmt.Errorf("agent %q 已存在", cfg.ID)
	}

	if err := m.buildAgent(cfg); err != nil {
		return err
	}

	return m.saveConfigs()
}

// UpdateAgent 更新 Agent 配置（重建实例）
func (m *Manager) UpdateAgent(cfg *AgentConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.configs[cfg.ID]; !exists {
		return fmt.Errorf("agent %q 不存在", cfg.ID)
	}

	if err := m.buildAgent(cfg); err != nil {
		return err
	}

	return m.saveConfigs()
}

// DeleteAgent 删除 Agent
func (m *Manager) DeleteAgent(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.configs[id]; !exists {
		return fmt.Errorf("agent %q 不存在", id)
	}

	delete(m.agents, id)
	delete(m.configs, id)

	// 删除历史缓存文件
	os.Remove(m.historyCachePath(id))

	return m.saveConfigs()
}

// GetAgent 获取 Agent 实例
func (m *Manager) GetAgent(id string) (*Agent, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ag, ok := m.agents[id]
	return ag, ok
}

// ListConfigs 列出所有 Agent 配置
func (m *Manager) ListConfigs() []*AgentConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	configs := make([]*AgentConfig, 0, len(m.configs))
	for _, cfg := range m.configs {
		configs = append(configs, cfg)
	}
	return configs
}

// GetConfig 获取单个 Agent 配置
func (m *Manager) GetConfig(id string) (*AgentConfig, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cfg, ok := m.configs[id]
	return cfg, ok
}

// AllToolNames 返回所有可用的工具名称
func (m *Manager) AllToolNames() []string {
	defs := m.registry.Definitions()
	names := make([]string, len(defs))
	for i, d := range defs {
		names[i] = d.Function.Name
	}
	return names
}

// buildAgent 内部方法：创建 Agent 实例（调用方须持有锁）
func (m *Manager) buildAgent(cfg *AgentConfig) error {
	if cfg.ID == "" {
		return fmt.Errorf("agent ID 不能为空")
	}
	if cfg.Name == "" {
		cfg.Name = cfg.ID
	}
	if cfg.MaxTurns <= 0 {
		cfg.MaxTurns = config.DefaultMaxTurns
	}
	if cfg.SystemPrompt == "" {
		cfg.SystemPrompt = config.DefaultSystemPrompt
	}

	// 根据配置的 model 创建 LLM 客户端
	model := cfg.Model
	if model == "" {
		model = m.llmBase.Model
	}
	llmClient := llm.NewOpenAIClient(llm.ClientConfig{
		APIKey:  m.llmBase.APIKey,
		BaseURL: m.llmBase.BaseURL,
		Model:   model,
	})

	// 创建仅包含指定工具的子注册表
	subRegistry := tool.NewRegistry()
	if len(cfg.Tools) == 0 {
		// 未指定工具时使用全部工具
		for _, def := range m.registry.Definitions() {
			if t, ok := m.registry.Get(def.Function.Name); ok {
				subRegistry.RegisterSilent(t)
			}
		}
	} else {
		for _, name := range cfg.Tools {
			if t, ok := m.registry.Get(name); ok {
				subRegistry.RegisterSilent(t)
			}
		}
	}

	ag := New(Options{
		Name:         cfg.Name,
		LLM:          llmClient,
		Tools:        subRegistry,
		MaxTurns:     cfg.MaxTurns,
		SystemPrompt: cfg.SystemPrompt,
		OnHistoryChanged: func() {
			m.saveHistory(cfg.ID)
		},
	})

	// 尝试恢复历史缓存
	if history, err := m.loadHistory(cfg.ID); err == nil && len(history) > 0 {
		ag.SetHistory(history)
		logger.Infof("Agent [%s] 已恢复 %d 条历史记录", cfg.ID, len(history))
	}

	m.agents[cfg.ID] = ag
	m.configs[cfg.ID] = cfg
	logger.Infof("Agent [%s] 已创建: model=%s, tools=%v", cfg.ID, model, cfg.Tools)
	return nil
}

// saveConfigs 写入配置文件（调用方须持有锁）
func (m *Manager) saveConfigs() error {
	configs := make([]*AgentConfig, 0, len(m.configs))
	for _, cfg := range m.configs {
		configs = append(configs, cfg)
	}

	data, err := json.MarshalIndent(configs, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 agent 配置失败: %w", err)
	}

	if err := os.WriteFile(m.savePath, data, 0o644); err != nil {
		return fmt.Errorf("写入 agent 配置文件失败: %w", err)
	}
	return nil
}

// ---------- 历史缓存 ----------

func (m *Manager) historyCachePath(agentID string) string {
	return filepath.Join(m.cacheDir, agentID+".json")
}

// saveHistory 保存指定 Agent 的对话历史
func (m *Manager) saveHistory(agentID string) {
	m.mu.RLock()
	ag, ok := m.agents[agentID]
	m.mu.RUnlock()
	if !ok {
		return
	}

	history := ag.GetHistory()
	data, err := json.Marshal(history)
	if err != nil {
		logger.Errorf("序列化 agent %s 历史失败: %v", agentID, err)
		return
	}

	if err := os.WriteFile(m.historyCachePath(agentID), data, 0o644); err != nil {
		logger.Errorf("保存 agent %s 历史缓存失败: %v", agentID, err)
	}
}

// loadHistory 加载指定 Agent 的对话历史
func (m *Manager) loadHistory(agentID string) ([]model.Message, error) {
	data, err := os.ReadFile(m.historyCachePath(agentID))
	if err != nil {
		return nil, err
	}

	var history []model.Message
	if err := json.Unmarshal(data, &history); err != nil {
		return nil, err
	}
	return history, nil
}

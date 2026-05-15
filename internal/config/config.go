package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// 默认值常量
const (
	DefaultModel        = "gpt-4o"
	DefaultBaseURL      = "https://api.openai.com/v1"
	DefaultMaxTurns     = 10
	DefaultMaxHistory   = 50
	DefaultMaxTokens    = 4096
	DefaultLogDir       = "log"
	DefaultLogLevel     = "INFO"
	DefaultWebAddr      = ":8080"
	DefaultWebBasePath  = "/myagent"
	DefaultSystemPrompt = "你是一个有用的 AI 助手。请用简洁、准确的方式回答问题。"
)

// Config 应用配置
type Config struct {
	LLM    LLMConfig           `json:"llm"`
	Agent  AgentConfig         `json:"agent"`
	Agents []PresetAgentConfig `json:"agents"`
	Log    LogConfig           `json:"log"`
	Web    WebConfig           `json:"web"`
}

// WebConfig Web 服务配置
type WebConfig struct {
	Enable   bool   `json:"enable"`
	Addr     string `json:"addr"`
	BasePath string `json:"base_path"`
}

// LogConfig 日志配置
type LogConfig struct {
	Dir       string `json:"dir"`
	Level     string `json:"level"`
	ToConsole bool   `json:"to_console"`
}

// LLMConfig LLM 相关配置
type LLMConfig struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
}

// AgentConfig Agent 相关配置（兼容旧配置）
type AgentConfig struct {
	Name         string `json:"name"`
	SystemPrompt string `json:"system_prompt"`
	MaxTurns     int    `json:"max_turns"`
}

// PresetAgentConfig 预配置的 Agent（在 config.json 的 agents 数组中定义）
type PresetAgentConfig struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Model        string   `json:"model"`
	SystemPrompt string   `json:"system_prompt"`
	MaxTurns     int      `json:"max_turns"`
	Tools        []string `json:"tools"`
}

// Load 加载配置，优先从配置文件读取，支持环境变量覆盖
func Load() (*Config, error) {
	cfg := &Config{
		LLM: LLMConfig{
			BaseURL: DefaultBaseURL,
			Model:   DefaultModel,
		},
		Agent: AgentConfig{
			Name:         "MyClaw",
			MaxTurns:     DefaultMaxTurns,
			SystemPrompt: "你是 MyClaw，" + DefaultSystemPrompt,
		},
		Log: LogConfig{
			Dir:       DefaultLogDir,
			Level:     DefaultLogLevel,
			ToConsole: false,
		},
		Web: WebConfig{
			Enable:   true,
			Addr:     DefaultWebAddr,
			BasePath: DefaultWebBasePath,
		},
	}

	// 尝试从配置文件加载
	if data, err := os.ReadFile("config.json"); err == nil {
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("解析配置文件失败: %w", err)
		}
	}

	// 环境变量覆盖
	if v := os.Getenv("MYCLAW_API_KEY"); v != "" {
		cfg.LLM.APIKey = v
	}
	if v := os.Getenv("MYCLAW_BASE_URL"); v != "" {
		cfg.LLM.BaseURL = v
	}
	if v := os.Getenv("MYCLAW_MODEL"); v != "" {
		cfg.LLM.Model = v
	}

	if cfg.LLM.APIKey == "" {
		return nil, fmt.Errorf("未设置 API Key，请设置环境变量 MYCLAW_API_KEY 或在 config.json 中配置")
	}

	return cfg, nil
}

// MaskKey 脱敏显示 API Key（仅保留前4后4字符）
func MaskKey(key string) string {
	if len(key) <= 8 {
		return strings.Repeat("*", len(key))
	}
	return key[:4] + strings.Repeat("*", len(key)-8) + key[len(key)-4:]
}

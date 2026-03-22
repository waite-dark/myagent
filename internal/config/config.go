package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config 应用配置
type Config struct {
	LLM   LLMConfig   `json:"llm"`
	Agent AgentConfig `json:"agent"`
	Log   LogConfig   `json:"log"`
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

// AgentConfig Agent 相关配置
type AgentConfig struct {
	Name         string `json:"name"`
	SystemPrompt string `json:"system_prompt"`
	MaxTurns     int    `json:"max_turns"`
}

// Load 加载配置，优先从配置文件读取，支持环境变量覆盖
func Load() (*Config, error) {
	cfg := &Config{
		LLM: LLMConfig{
			BaseURL: "https://api.openai.com/v1",
			Model:   "gpt-4o",
		},
		Agent: AgentConfig{
			Name:         "MyClaw",
			MaxTurns:     10,
			SystemPrompt: "你是 MyClaw，一个有用的 AI 助手。你可以使用工具来帮助用户完成任务。请用简洁、准确的方式回答问题。",
		},
		Log: LogConfig{
			Dir:       "log",
			Level:     "INFO",
			ToConsole: false,
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

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"myagent/internal/agent"
	"myagent/internal/config"
	"myagent/internal/llm"
	"myagent/internal/logger"
	"myagent/internal/tool"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 初始化日志系统
	if err := logger.Init(logger.Config{
		Dir:       cfg.Log.Dir,
		Level:     logger.ParseLevel(cfg.Log.Level),
		ToConsole: cfg.Log.ToConsole,
	}); err != nil {
		log.Fatalf("初始化日志失败: %v", err)
	}
	defer logger.Close()
	logger.Infof("MyClaw 启动, model=%s", cfg.LLM.Model)

	// 初始化 LLM 客户端
	llmClient := llm.NewOpenAIClient(llm.ClientConfig{
		APIKey:  cfg.LLM.APIKey,
		BaseURL: cfg.LLM.BaseURL,
		Model:   cfg.LLM.Model,
	})

	// 注册工具
	registry := tool.NewRegistry()
	registry.Register(tool.NewTimeTool())
	registry.Register(tool.NewHTTPTool())
	registry.Register(tool.NewCalcTool())

	// 创建 Agent
	ag := agent.New(agent.Options{
		Name:         cfg.Agent.Name,
		LLM:          llmClient,
		Tools:        registry,
		MaxTurns:     cfg.Agent.MaxTurns,
		SystemPrompt: cfg.Agent.SystemPrompt,
	})

	// 优雅退出
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\n正在退出...")
		cancel()
	}()

	// 启动交互式对话
	if err := ag.RunInteractive(ctx); err != nil {
		if ctx.Err() != nil {
			os.Exit(0)
		}
		log.Fatalf("Agent 运行出错: %v", err)
	}
}

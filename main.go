package main

import (
	"context"
	"flag"
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
	"myagent/internal/web"
)

func main() {
	agentFlag := flag.String("agent", "", "CLI 模式下使用的 Agent ID（为空则使用第一个）")
	flag.Parse()

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
	logger.Infof("MyClaw 启动, model=%s, api_key=%s", cfg.LLM.Model, config.MaskKey(cfg.LLM.APIKey))

	// 提醒优先使用环境变量
	if os.Getenv("MYCLAW_API_KEY") == "" {
		logger.Warnf("建议使用环境变量 MYCLAW_API_KEY 代替 config.json 中的明文 api_key")
	}

	// 注册全局工具
	registry := tool.NewRegistry()
	registry.Register(tool.NewTimeTool())
	registry.Register(tool.NewHTTPTool())
	registry.Register(tool.NewCalcTool())

	// 创建 Agent 管理器
	llmBase := llm.ClientConfig{
		APIKey:  cfg.LLM.APIKey,
		BaseURL: cfg.LLM.BaseURL,
		Model:   cfg.LLM.Model,
	}
	manager := agent.NewManager(llmBase, registry, "agents.json")

	// 从持久化文件加载已有 agents
	if err := manager.LoadFromFile(); err != nil {
		logger.Warnf("加载 agents 失败: %v", err)
	}

	// 从 config.json 中的预配置 agents 创建（如果尚不存在）
	for _, ac := range cfg.Agents {
		if _, exists := manager.GetConfig(ac.ID); !exists {
			if err := manager.CreateAgent(&agent.AgentConfig{
				ID:           ac.ID,
				Name:         ac.Name,
				Model:        ac.Model,
				SystemPrompt: ac.SystemPrompt,
				MaxTurns:     ac.MaxTurns,
				Tools:        ac.Tools,
			}); err != nil {
				logger.Warnf("从配置创建 agent %s 失败: %v", ac.ID, err)
			}
		}
	}

	// 确保至少有一个默认 agent（兼容旧配置）
	if len(manager.ListConfigs()) == 0 {
		_ = manager.CreateAgent(&agent.AgentConfig{
			ID:           "default",
			Name:         cfg.Agent.Name,
			Model:        cfg.LLM.Model,
			SystemPrompt: cfg.Agent.SystemPrompt,
			MaxTurns:     cfg.Agent.MaxTurns,
		})
	}

	// 退出
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\n正在退出...")
		cancel()
	}()

	// 启动 Web 服务
	if cfg.Web.Enable {
		webSrv := web.NewServer(manager, cfg.Web.Addr, cfg.Web.BasePath)
		go func() {
			if err := webSrv.Start(ctx); err != nil {
				logger.Errorf("Web 服务错误: %v", err)
			}
		}()
		fmt.Printf("🌐 Web 界面已启动: http://localhost%s%s\n\n", cfg.Web.Addr, cfg.Web.BasePath)
	}

	// 启动交互式对话
	configs := manager.ListConfigs()
	if len(configs) > 0 {
		targetID := configs[0].ID
		if *agentFlag != "" {
			targetID = *agentFlag
		}
		ag, ok := manager.GetAgent(targetID)
		if !ok {
			log.Fatalf("Agent %q 不存在，可用的 Agent: %v", targetID, agentIDs(configs))
		}
		if err := ag.RunInteractive(ctx); err != nil {
			if ctx.Err() != nil {
				os.Exit(0)
			}
			log.Fatalf("Agent 运行出错: %v", err)
		}
	}

	// 等待退出信号
	<-ctx.Done()
}

func agentIDs(configs []*agent.AgentConfig) []string {
	ids := make([]string, len(configs))
	for i, c := range configs {
		ids[i] = c.ID
	}
	return ids
}

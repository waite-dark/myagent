package agent

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"myagent/internal/llm"
	"myagent/internal/logger"
	"myagent/internal/model"
	"myagent/internal/tool"
)

// Options Agent 配置选项
type Options struct {
	Name             string
	LLM              llm.Client
	Tools            *tool.Registry
	MaxTurns         int
	MaxHistory       int // 最大保留的历史消息条数（0 表示不限制）
	SystemPrompt     string
	OnHistoryChanged func() // 历史变更回调（用于持久化）
}

// Agent AI Agent
type Agent struct {
	name             string
	llm              llm.Client
	tools            *tool.Registry
	maxTurns         int
	maxHistory       int
	systemPrompt     string
	history          []model.Message
	onHistoryChanged func()
}

// New 创建 Agent
func New(opts Options) *Agent {
	maxHistory := opts.MaxHistory
	if maxHistory <= 0 {
		maxHistory = 50
	}
	return &Agent{
		name:             opts.Name,
		llm:              opts.LLM,
		tools:            opts.Tools,
		maxTurns:         opts.MaxTurns,
		maxHistory:       maxHistory,
		systemPrompt:     opts.SystemPrompt,
		onHistoryChanged: opts.OnHistoryChanged,
		history: []model.Message{
			{Role: "system", Content: opts.SystemPrompt},
		},
	}
}

// RunInteractive 交互式运行
func (a *Agent) RunInteractive(ctx context.Context) error {
	scanner := bufio.NewScanner(os.Stdin)
	logger.Infof("Agent [%s] 启动", a.name)
	fmt.Printf("🤖 %s 已启动！输入消息开始对话，输入 /quit 退出，/clear 清空历史。\n\n", a.name)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		fmt.Print("You> ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		switch input {
		case "/quit", "/exit":
			logger.Infof("用户退出")
			fmt.Println("再见！")
			return nil
		case "/clear":
			a.history = []model.Message{
				{Role: "system", Content: a.systemPrompt},
			}
			fmt.Println("对话历史已清空。\n")
			continue
		case "/help":
			printHelp()
			continue
		}

		response, err := a.Chat(ctx, input)
		if err != nil {
			logger.Errorf("对话出错: %v", err)
			fmt.Printf("❌ 错误: %v\n\n", err)
			continue
		}

		fmt.Printf("\n%s> %s\n\n", a.name, response)
	}

	return scanner.Err()
}

// Chat 处理单次对话（支持多轮工具调用）
func (a *Agent) Chat(ctx context.Context, userMessage string) (string, error) {
	return a.ChatWithEvents(ctx, userMessage, nil)
}

// ChatWithEvents 处理单次对话，通过回调发送中间事件
func (a *Agent) ChatWithEvents(ctx context.Context, userMessage string, onEvent func(Event)) (string, error) {
	emit := func(e Event) {
		if onEvent != nil {
			onEvent(e)
		}
	}

	logger.Infof("用户输入: %s", userMessage)
	a.history = append(a.history, model.Message{
		Role:    "user",
		Content: userMessage,
	})

	toolDefs := a.tools.Definitions()

	for turn := 0; turn < a.maxTurns; turn++ {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		emit(Event{Type: EventThinking})

		logger.Debugf("LLM 请求: turn=%d, messages=%d", turn, len(a.history))
		resp, err := a.llm.Chat(ctx, a.history, toolDefs)
		if err != nil {
			logger.Errorf("LLM 调用失败: %v", err)
			return "", fmt.Errorf("LLM 调用失败: %w", err)
		}

		if len(resp.Choices) == 0 {
			return "", fmt.Errorf("LLM 返回空响应")
		}

		choice := resp.Choices[0]
		a.history = append(a.history, choice.Message)

		// 如果没有工具调用，裁剪历史并返回
		if len(choice.Message.ToolCalls) == 0 {
			logger.Infof("LLM 响应: %s", truncate(choice.Message.Content, 200))
			a.trimHistory()
			a.notifyHistoryChanged()
			emit(Event{Type: EventAssistant, Content: choice.Message.Content})
			return choice.Message.Content, nil
		}

		// 处理工具调用
		fmt.Printf("  🔧 正在调用工具...\n")
		for _, tc := range choice.Message.ToolCalls {
			fmt.Printf("    → %s(%s)\n", tc.Function.Name, tc.Function.Arguments)
			logger.Infof("工具调用: %s args=%s", tc.Function.Name, tc.Function.Arguments)
			emit(Event{Type: EventToolCall, Name: tc.Function.Name, Args: tc.Function.Arguments})

			result, err := a.tools.Execute(ctx, tc.Function.Name, tc.Function.Arguments)
			if err != nil {
				logger.Errorf("工具 %s 执行失败: %v", tc.Function.Name, err)
				result = fmt.Sprintf("工具执行错误: %v", err)
			} else {
				logger.Debugf("工具 %s 结果: %s", tc.Function.Name, truncate(result, 200))
			}
			emit(Event{Type: EventToolResult, Name: tc.Function.Name, Result: result})

			a.history = append(a.history, model.Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
			})
		}
	}

	msg := "达到最大工具调用轮次限制"
	emit(Event{Type: EventAssistant, Content: msg})
	return msg, nil
}

// ClearHistory 清空对话历史
func (a *Agent) ClearHistory() {
	a.history = []model.Message{
		{Role: "system", Content: a.systemPrompt},
	}
	a.notifyHistoryChanged()
}

// GetHistory 获取当前对话历史（用于持久化）
func (a *Agent) GetHistory() []model.Message {
	return a.history
}

// SetHistory 恢复对话历史（从缓存加载）
func (a *Agent) SetHistory(history []model.Message) {
	if len(history) == 0 {
		return
	}
	a.history = history
}

func (a *Agent) notifyHistoryChanged() {
	if a.onHistoryChanged != nil {
		a.onHistoryChanged()
	}
}

// trimHistory 裁剪对话历史，保留 system 消息 + 最近 maxHistory 条
func (a *Agent) trimHistory() {
	// history[0] 是 system prompt，不计入裁剪
	if len(a.history) <= a.maxHistory+1 {
		return
	}
	trimmed := make([]model.Message, 0, a.maxHistory+1)
	trimmed = append(trimmed, a.history[0]) // system prompt
	trimmed = append(trimmed, a.history[len(a.history)-a.maxHistory:]...)
	a.history = trimmed
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func printHelp() {
	fmt.Println(`
可用命令:
  /help   显示帮助信息
  /clear  清空对话历史
  /quit   退出程序
  /exit   退出程序
`)
}

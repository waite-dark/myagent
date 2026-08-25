package agent

// EventType 事件类型
type EventType string

const (
	EventThinking   EventType = "thinking"
	EventToolCall   EventType = "tool_call"
	EventToolResult EventType = "tool_result"
	EventAssistant  EventType = "assistant"
	EventStream     EventType = "stream"     // 流式输出增量
	EventStreamEnd  EventType = "stream_end" // 流式输出结束
	EventError      EventType = "error"
	EventRoute      EventType = "route" // 路由事件：通知前端当前请求被路由到哪个 Agent
)

// Event Agent 事件
type Event struct {
	Type    EventType `json:"type"`
	Content string    `json:"content,omitempty"`
	Name    string    `json:"name,omitempty"`
	Args    string    `json:"args,omitempty"`
	Result  string    `json:"result,omitempty"`
}

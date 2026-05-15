package agent

import (
	"sync"
	"time"

	"myagent/internal/model"
)

// Session 独立的对话会话，每个 WebSocket 连接一个
type Session struct {
	mu           sync.Mutex
	ID           string
	AgentID      string
	History      []model.Message
	SystemPrompt string
	CreatedAt    time.Time
	LastActiveAt time.Time
	MaxTokens    int // token 预算上限（0 表示使用默认值）
}

// NewSession 创建新会话
func NewSession(id, agentID, systemPrompt string, maxTokens int) *Session {
	now := time.Now()
	return &Session{
		ID:           id,
		AgentID:      agentID,
		SystemPrompt: systemPrompt,
		CreatedAt:    now,
		LastActiveAt: now,
		MaxTokens:    maxTokens,
		History: []model.Message{
			{Role: "system", Content: systemPrompt},
		},
	}
}

// AddMessage 添加消息到会话历史
func (s *Session) AddMessage(msg model.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.History = append(s.History, msg)
	s.LastActiveAt = time.Now()
}

// GetHistory 获取会话历史（线程安全拷贝）
func (s *Session) GetHistory() []model.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	copied := make([]model.Message, len(s.History))
	copy(copied, s.History)
	return copied
}

// SetHistory 设置会话历史
func (s *Session) SetHistory(history []model.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.History = history
	s.LastActiveAt = time.Now()
}

// Clear 清空历史，仅保留 system prompt
func (s *Session) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.History = []model.Message{
		{Role: "system", Content: s.SystemPrompt},
	}
	s.LastActiveAt = time.Now()
}

// EstimateTokens 估算当前历史的 token 数
// 简单估算：中文约 2 字符/token，英文约 4 字符/token，取平均 3 字符/token
func (s *Session) EstimateTokens() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return estimateMessagesTokens(s.History)
}

func estimateMessagesTokens(messages []model.Message) int {
	total := 0
	for _, msg := range messages {
		total += estimateStringTokens(msg.Content)
		for _, tc := range msg.ToolCalls {
			total += estimateStringTokens(tc.Function.Name)
			total += estimateStringTokens(tc.Function.Arguments)
		}
		// 每条消息有固定开销（role 等元数据）
		total += 4
	}
	return total
}

func estimateStringTokens(s string) int {
	// 混合中英文场景取平均：约 3 字符 = 1 token
	runeCount := len([]rune(s))
	if runeCount == 0 {
		return 0
	}
	return (runeCount + 2) / 3
}

// SessionManager 管理多个会话
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

// NewSessionManager 创建会话管理器
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
	}
}

// Get 获取会话
func (sm *SessionManager) Get(sessionID string) (*Session, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	s, ok := sm.sessions[sessionID]
	return s, ok
}

// Create 创建会话
func (sm *SessionManager) Create(id, agentID, systemPrompt string, maxTokens int) *Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := NewSession(id, agentID, systemPrompt, maxTokens)
	sm.sessions[id] = s
	return s
}

// Delete 删除会话
func (sm *SessionManager) Delete(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.sessions, sessionID)
}

// ListByAgent 列出指定 agent 的所有会话
func (sm *SessionManager) ListByAgent(agentID string) []*Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	var result []*Session
	for _, s := range sm.sessions {
		if s.AgentID == agentID {
			result = append(result, s)
		}
	}
	return result
}

// CleanExpired 清理过期会话（超过 maxAge 未活跃的会话）
func (sm *SessionManager) CleanExpired(maxAge time.Duration) int {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	cutoff := time.Now().Add(-maxAge)
	count := 0
	for id, s := range sm.sessions {
		if s.LastActiveAt.Before(cutoff) {
			delete(sm.sessions, id)
			count++
		}
	}
	return count
}

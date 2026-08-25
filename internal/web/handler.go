package web

import (
	"context"
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"time"

	"myagent/internal/agent"
	"myagent/internal/logger"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

//go:embed static
var staticFiles embed.FS

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// clientMessage 客户端发送的消息
type clientMessage struct {
	Type    string `json:"type"`    // "message" | "clear"
	Content string `json:"content"` // 用户输入
}

// Server Web 服务
type Server struct {
	manager        *agent.Manager
	sessionManager *agent.SessionManager
	orchestrator   *agent.Orchestrator
	addr           string
	basePath       string
}

// NewServer 创建 Web 服务
func NewServer(manager *agent.Manager, addr string, basePath string) *Server {
	basePath = strings.TrimRight(basePath, "/")
	if basePath != "" && !strings.HasPrefix(basePath, "/") {
		basePath = "/" + basePath
	}
	return &Server{
		manager:        manager,
		sessionManager: agent.NewSessionManager(),
		addr:           addr,
		basePath:       basePath,
	}
}

// SetOrchestrator 设置编排器（可选）
func (s *Server) SetOrchestrator(o *agent.Orchestrator) {
	s.orchestrator = o
}

// Start 启动 HTTP 服务
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// 静态文件
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return err
	}
	mux.Handle(s.basePath+"/", http.StripPrefix(s.basePath, http.FileServer(http.FS(sub))))

	// Agent CRUD API
	mux.HandleFunc(s.basePath+"/api/agents", s.handleAgents)
	mux.HandleFunc(s.basePath+"/api/agents/", s.handleAgent)
	mux.HandleFunc(s.basePath+"/api/tools", s.handleTools)

	// WebSocket（通过 ?agent=<id> 指定 agent）
	mux.HandleFunc(s.basePath+"/ws", s.handleWS)

	srv := &http.Server{Addr: s.addr, Handler: mux}

	// 定期清理过期会话（1小时未活跃）
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if count := s.sessionManager.CleanExpired(1 * time.Hour); count > 0 {
					logger.Infof("清理了 %d 个过期会话", count)
				}
			}
		}
	}()

	go func() {
		<-ctx.Done()
		srv.Close()
	}()

	logger.Infof("Web 服务启动: http://%s%s", s.addr, s.basePath)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// ---------- REST API ----------

func (s *Server) handleTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.manager.AllToolNames())
}

func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.manager.ListConfigs())
	case http.MethodPost:
		var cfg agent.AgentConfig
		if err := readBody(r, &cfg); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := s.manager.CreateAgent(&cfg); err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, cfg)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAgent(w http.ResponseWriter, r *http.Request) {
	// 从路径 {basePath}/api/agents/{id} 提取 id
	prefix := s.basePath + "/api/agents/"
	id := r.URL.Path[len(prefix):]
	if id == "" {
		http.Error(w, "缺少 agent ID", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		cfg, ok := s.manager.GetConfig(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent 不存在"})
			return
		}
		writeJSON(w, http.StatusOK, cfg)

	case http.MethodPut:
		var cfg agent.AgentConfig
		if err := readBody(r, &cfg); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		cfg.ID = id
		if err := s.manager.UpdateAgent(&cfg); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, cfg)

	case http.MethodDelete:
		if err := s.manager.DeleteAgent(id); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// ---------- WebSocket ----------

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("agent")
	useRoute := s.orchestrator != nil && r.URL.Query().Get("route") == "true"

	if agentID == "" && !useRoute {
		http.Error(w, "缺少 agent 参数", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Errorf("WebSocket 升级失败: %v", err)
		return
	}
	defer conn.Close()

	// 为每个 WebSocket 连接创建独立会话
	sessionID := uuid.New().String()
	var session *agent.Session

	if useRoute {
		// 路由模式：使用编排器，默认 system prompt
		session = s.sessionManager.Create(sessionID, "__orchestrator__", "编排器路由", 0)
	} else {
		ag, ok := s.manager.GetAgent(agentID)
		if !ok {
			http.Error(w, "agent 不存在", http.StatusNotFound)
			return
		}
		session = s.sessionManager.Create(sessionID, agentID, ag.GetSystemPrompt(), ag.GetMaxTokens())
	}

	defer s.sessionManager.Delete(sessionID)
	logger.Infof("WebSocket 客户端连接: %s -> agent=%s, session=%s, route=%v", r.RemoteAddr, agentID, sessionID, useRoute)

	var writeMu sync.Mutex
	wsWriteJSON := func(v interface{}) {
		writeMu.Lock()
		defer writeMu.Unlock()
		conn.WriteJSON(v)
	}

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				logger.Errorf("WebSocket 读取错误: %v", err)
			}
			return
		}

		var msg clientMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			wsWriteJSON(agent.Event{Type: agent.EventError, Content: "无效的消息格式"})
			continue
		}

		switch msg.Type {
		case "clear":
			session.Clear()
			wsWriteJSON(agent.Event{Type: agent.EventAssistant, Content: "对话历史已清空。"})

		case "message":
			if msg.Content == "" {
				continue
			}

			if useRoute && s.orchestrator != nil {
				// 编排路由模式
				_, err := s.orchestrator.RouteAndChatWithEvents(r.Context(), msg.Content, session, func(e agent.Event) {
					wsWriteJSON(e)
				})
				if err != nil {
					wsWriteJSON(agent.Event{Type: agent.EventError, Content: err.Error()})
				}
			} else {
				// 直接 Agent 模式
				ag, ok := s.manager.GetAgent(agentID)
				if !ok {
					wsWriteJSON(agent.Event{Type: agent.EventError, Content: "agent 不存在"})
					continue
				}
				_, err := ag.ChatSessionStream(r.Context(), session, msg.Content, func(e agent.Event) {
					wsWriteJSON(e)
				})
				if err != nil {
					wsWriteJSON(agent.Event{Type: agent.EventError, Content: err.Error()})
				}
			}

		default:
			wsWriteJSON(agent.Event{Type: agent.EventError, Content: "未知的消息类型"})
		}
	}
}

// ---------- Helpers ----------

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func readBody(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	data, err := io.ReadAll(io.LimitReader(r.Body, 1*1024*1024))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

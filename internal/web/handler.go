package web

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"sync"

	"myagent/internal/agent"
	"myagent/internal/logger"

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
	ag   *agent.Agent
	addr string
}

// NewServer 创建 Web 服务
func NewServer(ag *agent.Agent, addr string) *Server {
	return &Server{ag: ag, addr: addr}
}

// Start 启动 HTTP 服务
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// 静态文件
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return err
	}
	mux.Handle("/", http.FileServer(http.FS(sub)))

	// WebSocket
	mux.HandleFunc("/ws", s.handleWS)

	srv := &http.Server{Addr: s.addr, Handler: mux}

	go func() {
		<-ctx.Done()
		srv.Close()
	}()

	logger.Infof("Web 服务启动: http://%s", s.addr)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Errorf("WebSocket 升级失败: %v", err)
		return
	}
	defer conn.Close()

	logger.Infof("WebSocket 客户端连接: %s", r.RemoteAddr)

	var writeMu sync.Mutex
	writeJSON := func(v interface{}) {
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
			writeJSON(agent.Event{Type: agent.EventError, Content: "无效的消息格式"})
			continue
		}

		switch msg.Type {
		case "clear":
			s.ag.ClearHistory()
			writeJSON(agent.Event{Type: agent.EventAssistant, Content: "对话历史已清空。"})

		case "message":
			if msg.Content == "" {
				continue
			}
			_, err := s.ag.ChatWithEvents(r.Context(), msg.Content, func(e agent.Event) {
				writeJSON(e)
			})
			if err != nil {
				writeJSON(agent.Event{Type: agent.EventError, Content: err.Error()})
			}

		default:
			writeJSON(agent.Event{Type: agent.EventError, Content: "未知的消息类型"})
		}
	}
}

package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Level 日志级别
type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
)

func (l Level) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// ParseLevel 从字符串解析日志级别
func ParseLevel(s string) Level {
	switch s {
	case "DEBUG", "debug":
		return DEBUG
	case "INFO", "info":
		return INFO
	case "WARN", "warn":
		return WARN
	case "ERROR", "error":
		return ERROR
	default:
		return INFO
	}
}

// Config 日志配置
type Config struct {
	Dir       string // 日志文件目录
	Level     Level  // 最低日志级别
	ToConsole bool   // 是否同时输出到控制台
}

// Logger 日志记录器
type Logger struct {
	mu        sync.Mutex
	dir       string
	level     Level
	toConsole bool
	file      *os.File
	fileDate  string
	logger    *log.Logger
}

var global *Logger

// Init 初始化全局日志记录器
func Init(cfg Config) error {
	dir := cfg.Dir
	if dir == "" {
		dir = "log"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建日志目录失败: %w", err)
	}

	l := &Logger{
		dir:       dir,
		level:     cfg.Level,
		toConsole: cfg.ToConsole,
	}
	if err := l.rotateIfNeeded(); err != nil {
		return err
	}

	global = l
	return nil
}

// Close 关闭全局日志
func Close() {
	if global != nil && global.file != nil {
		global.file.Close()
	}
}

// rotateIfNeeded 按日期轮转日志文件
func (l *Logger) rotateIfNeeded() error {
	today := time.Now().Format("2006-01-02")
	if l.fileDate == today && l.file != nil {
		return nil
	}

	if l.file != nil {
		l.file.Close()
	}

	filename := filepath.Join(l.dir, today+".log")
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("打开日志文件失败: %w", err)
	}

	l.file = f
	l.fileDate = today

	var w io.Writer = f
	if l.toConsole {
		w = io.MultiWriter(f, os.Stderr)
	}
	l.logger = log.New(w, "", 0)

	return nil
}

func (l *Logger) log(level Level, format string, args ...any) {
	if level < l.level {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	_ = l.rotateIfNeeded()

	ts := time.Now().Format("2006-01-02 15:04:05.000")
	msg := fmt.Sprintf(format, args...)
	l.logger.Printf("%s [%s] %s", ts, level, msg)
}

// --- 全局函数 ---

func Debugf(format string, args ...any) {
	if global != nil {
		global.log(DEBUG, format, args...)
	}
}

func Infof(format string, args ...any) {
	if global != nil {
		global.log(INFO, format, args...)
	}
}

func Warnf(format string, args ...any) {
	if global != nil {
		global.log(WARN, format, args...)
	}
}

func Errorf(format string, args ...any) {
	if global != nil {
		global.log(ERROR, format, args...)
	}
}

package tool

import (
	"context"
	"encoding/json"
	"time"
)

// TimeTool 获取当前时间的工具
type TimeTool struct{}

// NewTimeTool 创建时间工具
func NewTimeTool() *TimeTool {
	return &TimeTool{}
}

func (t *TimeTool) Name() string {
	return "get_current_time"
}

func (t *TimeTool) Description() string {
	return "获取当前的日期和时间"
}

func (t *TimeTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"timezone": {
				"type": "string",
				"description": "时区名称，例如 Asia/Shanghai、America/New_York，默认使用本地时区"
			}
		}
	}`)
}

type timeArgs struct {
	Timezone string `json:"timezone"`
}

func (t *TimeTool) Execute(_ context.Context, args string) (string, error) {
	var a timeArgs
	if args != "" {
		if err := json.Unmarshal([]byte(args), &a); err != nil {
			return "", err
		}
	}

	loc := time.Local
	if a.Timezone != "" {
		var err error
		loc, err = time.LoadLocation(a.Timezone)
		if err != nil {
			return "", err
		}
	}

	now := time.Now().In(loc)
	return now.Format("2006-01-02 15:04:05 (MST)"), nil
}

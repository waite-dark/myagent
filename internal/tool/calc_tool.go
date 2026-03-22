package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// CalcTool 简易数学计算工具
type CalcTool struct{}

// NewCalcTool 创建计算工具
func NewCalcTool() *CalcTool {
	return &CalcTool{}
}

func (t *CalcTool) Name() string {
	return "calculate"
}

func (t *CalcTool) Description() string {
	return "执行数学表达式计算。支持的运算: add(加), sub(减), mul(乘), div(除), pow(幂), sqrt(开方)"
}

func (t *CalcTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"operation": {
				"type": "string",
				"description": "运算类型: add, sub, mul, div, pow, sqrt",
				"enum": ["add", "sub", "mul", "div", "pow", "sqrt"]
			},
			"a": {
				"type": "number",
				"description": "第一个操作数"
			},
			"b": {
				"type": "number",
				"description": "第二个操作数（sqrt 不需要）"
			}
		},
		"required": ["operation", "a"]
	}`)
}

type calcArgs struct {
	Operation string  `json:"operation"`
	A         float64 `json:"a"`
	B         float64 `json:"b"`
}

func (t *CalcTool) Execute(_ context.Context, args string) (string, error) {
	var a calcArgs
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}

	var result float64
	switch strings.ToLower(a.Operation) {
	case "add":
		result = a.A + a.B
	case "sub":
		result = a.A - a.B
	case "mul":
		result = a.A * a.B
	case "div":
		if a.B == 0 {
			return "", fmt.Errorf("除数不能为零")
		}
		result = a.A / a.B
	case "pow":
		result = math.Pow(a.A, a.B)
	case "sqrt":
		if a.A < 0 {
			return "", fmt.Errorf("不能对负数开方")
		}
		result = math.Sqrt(a.A)
	default:
		return "", fmt.Errorf("不支持的运算: %s", a.Operation)
	}

	return strconv.FormatFloat(result, 'f', -1, 64), nil
}

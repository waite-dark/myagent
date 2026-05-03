---
name: add-tool
description: Add a new tool to MyClaw AI agent
---

# Add Tool

This skill guides adding a new tool to the MyClaw project.

## Steps

1. Create a new file in `internal/tool/` (e.g., `xxx_tool.go`)
2. Implement the `tool.Tool` interface:

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() json.RawMessage
    Execute(ctx context.Context, args string) (string, error)
}
```

3. Define the JSON Schema for Parameters as a `json.RawMessage`
4. Define an args struct for parsing execute arguments
5. Register the tool in `main.go` with `registry.Register(tool.NewXxxTool())`

## Existing reference tools

- `internal/tool/calc_tool.go` — calculation tool with structured params
- `internal/tool/http_tool.go` — HTTP GET tool with SSRF protection
- `internal/tool/time_tool.go` — time query tool with optional timezone

## After implementation

Run `make build` (or `go build .`) and test.

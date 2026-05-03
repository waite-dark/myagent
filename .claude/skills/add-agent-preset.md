---
name: add-agent-preset
description: Add a pre-configured agent to config.json
---

# Add Agent Preset

Add a pre-configured agent entry in `config.json` so it auto-creates on startup.

## Steps

1. Edit `config.json`, add an entry to the `agents` array:

```json
{
  "id": "my-agent",
  "name": "My Agent",
  "model": "",
  "system_prompt": "You are a helpful assistant.",
  "max_turns": 10,
  "tools": []
}
```

- `id` — unique identifier
- `name` — display name
- `model` — leave empty to use default from `llm.model`
- `tools` — `[]` means all tools; specify tool names to restrict: `["get_current_time", "calculate"]`

2. Restart the app to auto-load the preset agent.

## Available tools

Check `main.go` for registered tools or call `GET /api/tools` when the server is running.

---
name: build
description: Build, test, lint, and run the MyClaw project
---

# Build

## Quick reference

| Command       | Action              |
| ------------- | ------------------- |
| `make build`  | Build binary        |
| `make run`    | Run with `go run`   |
| `make test`   | Run all tests       |
| `make lint`   | Run `go vet`        |
| `make clean`  | Remove build output |

## Environment variables

- `MYCLAW_API_KEY` — LLM API key
- `MYCLAW_BASE_URL` — LLM API base URL
- `MYCLAW_MODEL` — LLM model name

## Build output

The binary goes to `bin/myclaw` (or `myagent.exe` for local dev).

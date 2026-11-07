# AGENTS.md — signal-tui

Terminal UI client for Signal, written in Go. It drives an external `signal-cli` daemon.

## Commands
- Build: `go build -o signal-tui .`
- Vet: `go vet ./...`
- Format check: `gofmt -l .` (must print nothing)
- Tests: none yet. Add `_test.go` files for new pure logic.

## Layout
- `main.go` — app logic: config/history, signal-cli daemon + JSON-RPC, event stream, attachments, Bubble Tea model/update.
- `ui.go` — rendering and styles.
- Stack: Bubble Tea v2, lipgloss v2, bubbles v2 (`charm.land/...`), go-qrcode.

## Architecture
- Starts `signal-cli -a <account> daemon --http 127.0.0.1:<port> --receive-mode on-connection`.
- Talks to the daemon over JSON-RPC (`/api/v1/rpc`) and the events endpoint (`/api/v1/events`).
- The daemon must bind to 127.0.0.1 only.

## Security rules
- Write files 0600 and create directories 0700.
- Never log message content, phone numbers, or account identifiers.
- Never hardcode account numbers, UUIDs, or phone numbers.

## Agent prohibitions
- Do NOT run `signal-cli link` or `signal-cli register`, or start a daemon on a real account. The link feature in the app is code to edit, not to execute.
- Do NOT read or print real account data, contacts, or message history from the user's signal-cli data dir.
- Do NOT change dependencies in `go.mod` without asking.

## Repo hygiene
- Track only source and module files: `.gitignore`, `AGENTS.md`, `go.mod`, `go.sum`, `*.go`.
- Never commit `.bak` files, the `signal-tui` binary, logs, or caches.
- Do NOT commit until the user has tested the change.
- Before finishing a change, run build, vet, and the gofmt check and report the real output.

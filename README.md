# signal-tui

A terminal UI (TUI) for [Signal](https://signal.org), built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss). Connects to a local [signal-cli](https://github.com/AsamK/signal-cli) daemon over its JSON-RPC/HTTP API.

## Features

- Two-column layout: contacts sidebar + chat pane
- Send and receive text messages
- Send and receive file attachments (image/document allowlist)
- Restricted-file detection on receive (non-allowed types are removed and reported)
- Read receipts (✓ delivered, ✓✓ read) on sent messages
- Typing indicators
- Reactions
- File picker (Ctrl+A) with directory navigation
- Help overlay (Ctrl+H)
- Debug logging to `~/.cache/signal-tui/debug.log`

## Requirements

- Go 1.23+
- signal-cli daemon running on localhost (default port 8082)
- A registered Signal account (UUID in config)

## Build

```sh
cd /home/mwatters/signal-tui
go build -o signal-tui .
```

## Run

```sh
./signal-tui
```

On first run, a config is created at `~/.config/signal-tui/config.json`:

```json
{
  "account": "<your-account-uuid>",
  "port": 8082
}
```

## Keybindings

| Key | Action |
|-----|--------|
| Tab | Switch focus (contacts / input) |
| ↑/↓ | Navigate contacts |
| Enter | Send message (input) / open dir (picker) |
| Ctrl+A | Open file picker |
| Ctrl+D | Delete conversation |
| Ctrl+O | Open last received attachment |
| Ctrl+H | Toggle help |
| Esc | Close help / cancel picker |
| Q | Quit (when input empty) |

File picker: `j`/`k` or arrows navigate, `Enter`/`l` open/select, `h`/`←`/Backspace go up, `Esc` cancel.

## Attachment receive dialog

When a file is received, a dialog offers:
- `o` — open (supported types only)
- `s` — save to `~/Downloads/signal-tui/` (created if missing)
- `d`/Esc — dismiss

## Security / permissions

All config, history, cached attachments, and debug logs are written with mode `0600` inside `0700` directories. Existing files created by older versions may need a one-time `chmod 600`/`700`.

## Files

- `main.go` — application logic
- `ui.go` — theme/palette system
- `.gitignore` — excludes binary, backups, logs, cache

## License

Personal project. No license specified.

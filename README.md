# cc-tool-reviewer

A fast, daemon-based AI reviewer for [Claude Code](https://docs.anthropic.com/en/docs/claude-code) tool calls. Reduces permission prompts by using Haiku 4.5 to evaluate "ask zone" commands — those that don't match your explicit allow/deny rules but are still consistent with what you've permitted.

On macOS, shows a translucent floating HUD for approve/deny decisions instead of Claude Code's terminal prompt.

## The problem

Claude Code has three permission outcomes for tool calls:

1. **Allow** — matches an allow rule, executes immediately
2. **Deny** — matches a deny rule, blocked
3. **Ask** — matches neither, prompts the user

The "ask" zone creates friction. You get prompted for commands like `cd ~/git/x && git log` even though both `cd` and `git log` are individually allowed — the compound command doesn't match any single rule. Same for multi-line scripts that compose entirely allowed operations.

## Why not use Claude Code's built-in `type: "prompt"` hook?

Claude Code supports a built-in `prompt` hook type that sends tool calls to a model for review. It handles the API connection internally, so there's no connection overhead. But it has limitations:

| | Built-in `prompt` hook | cc-tool-reviewer |
|---|---|---|
| **Fires on** | Every matching tool call | Only "ask zone" calls |
| **`rg foo` latency** | ~700ms (API call) | ~0ms (local match) |
| **Prompt context** | Generic, user-defined | Injects your actual allow list |
| **Compound commands** | No special handling | Detects `&&`, `\|\|`, `;`, multi-line and routes to AI |

The key difference: the built-in prompt hook calls the API on every matching tool call, even ones already covered by your allow/deny rules. cc-tool-reviewer replicates your allow/deny matching locally and only calls the API for the "ask zone". Most tool calls (~90%) are resolved in under 5ms with no API call.

## Architecture

```
Claude Code ──stdin──▶ nc -U /tmp/cc-tool-reviewer.sock ──▶ Go daemon
                                                               │
                                                    ┌──────────┴──────────┐
                                                    │ 1. Auto-allow?      │
                                                    │    (WebFetch,       │
                                                    │     WebSearch)      │
                                                    │         ↓ no        │
                                                    │ 2. Local match      │
                                                    │    against allow/   │
                                                    │    deny rules       │
                                                    │         ↓ no match  │
                                                    │ 3. Call Haiku 4.5   │
                                                    │    via persistent   │
                                                    │    HTTP/2 conn      │
                                                    │         ↓ "ask"     │
                                                    │ 4. Native dialog    │
                                                    │    (macOS only)     │
                                                    └──────────┬──────────┘
Claude Code ◀──stdout── nc ◀────────────────────────────────────┘
```

## Setup

### Install from release

```bash
curl -sL https://raw.githubusercontent.com/anish749/cc-tool-reviewer/main/install.sh | bash
```

This downloads the latest release to `~/.local/bin/`. On macOS, it also compiles the native approval dialog from source (requires Xcode command line tools). Override the install directory with `INSTALL_DIR`:

```bash
curl -sL https://raw.githubusercontent.com/anish749/cc-tool-reviewer/main/install.sh | INSTALL_DIR=/usr/local/bin bash
```

### Build from source (for development)

```bash
git clone https://github.com/anish749/cc-tool-reviewer.git
cd cc-tool-reviewer
make install
```

The Makefile is for local development — it builds the Go daemon and the Swift dialog (on macOS) from source and installs both to `~/.local/bin/`.

### Configure Claude Code hook

Add to `~/.claude/settings.json`:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash|WebFetch|WebSearch",
        "hooks": [
          {
            "type": "command",
            "command": "nc -U /tmp/cc-tool-reviewer.sock"
          }
        ]
      }
    ]
  }
}
```

Do **not** use `nc -w` (timeout) — the native dialog needs time for user interaction. If the daemon isn't running, `nc` fails immediately on connect, so there's no hang risk.

### Start the daemon

```bash
cc-tool-reviewer
```

Or with a custom socket path:

```bash
cc-tool-reviewer --socket /tmp/my-reviewer.sock
```

The daemon should be started outside of Claude Code (e.g., from a shell alias or launch script) since the hook would interfere with starting it from within a Claude Code session.

### Environment

The daemon needs access to the Anthropic API. There are three ways to provide credentials:

1. **`ANTHROPIC_API_KEY`** — set your API key directly
2. **`ANTHROPIC_BASE_URL`** — inherit from a running Claude Code session (e.g., `http://localhost:1992`)
3. **Claude Code OAuth token** — run `claude setup-token` to get a token, then set it as `ANTHROPIC_API_KEY`. This lets the daemon run independently without a separate API key or a running Claude Code session.

Settings are loaded from (and hot-reloaded on change):
- `$CLAUDE_CONFIG_DIR/settings.json` (falls back to `~/.claude/settings.json`)
- `$CLAUDE_CONFIG_DIR/settings.local.json`
- `.claude/settings.json` (project-level)
- `.claude/settings.local.json` (project-level)

## How it works

1. Claude Code fires the `PreToolUse` hook, piping JSON to `nc`
2. `nc` forwards it to the daemon via Unix socket (~4ms overhead)
3. The daemon decides what to do based on the tool type:

**Auto-allowed tools** — `WebFetch` and `WebSearch` are always approved instantly with no matching or AI call. The daemon logs the URL/query for visibility.

**Bash commands** — checked against your allow/deny rules locally:
- If it matches an allow or deny rule → empty response (Claude Code handles it normally)
- If it's a compound command (`&&`, `||`, `;`, multi-line, subshells) → always sent to the AI, since simple prefix matching can't evaluate these
- Otherwise ("ask zone") → calls Haiku 4.5 with your allow list as context

**AI says "allow"** → tool call proceeds, no prompt.

**AI says "ask"** → on macOS, a translucent floating HUD appears with three options:
- **Approve** (Enter) — allows the tool call
- **Deny** — blocks the tool call
- **Later** (Esc) — defers to Claude Code's terminal prompt

On non-macOS systems, "ask" falls through to Claude Code's terminal prompt.

### Compound command detection

Simple commands like `rg foo` match locally. But compound commands containing `&&`, `||`, `;`, newlines, or subshells (`$(...)`) bypass local matching and are always sent to the AI. This is because:

- `cd ~/git/x && git log` doesn't match `Bash(cd:*)` in Claude Code's real matcher
- The AI can evaluate each part and recognize both are individually allowed
- Multi-line scripts need semantic understanding, not prefix matching

## Graceful degradation

If the daemon isn't running, `nc` fails with a non-zero exit code (but not exit code 2). Claude Code treats this as a no-op and falls through to the normal permission prompt. Nothing breaks.

## Performance

| Scenario | Latency |
|----------|---------|
| Auto-allowed (WebFetch, WebSearch) | ~4ms (`nc` overhead) |
| Local match (allow/deny rule) | ~4ms (`nc` overhead) |
| API call (cold connection) | ~1000ms |
| API call (warm connection) | ~700ms |
| Daemon not running (fallback) | ~4ms (nc fails fast) |

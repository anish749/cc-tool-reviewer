# cc-tool-reviewer

A fast, daemon-based AI reviewer for [Claude Code](https://docs.anthropic.com/en/docs/claude-code) tool calls. Reduces permission prompts by using Haiku 4.5 to evaluate "ask zone" commands — those that don't match your explicit allow/deny rules but are still consistent with what you've permitted.

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
                                                    │ 1. Local match      │
                                                    │    against allow/   │
                                                    │    deny rules       │
                                                    │                     │
                                                    │ 2. If "ask zone":  │
                                                    │    Call Haiku 4.5   │
                                                    │    via persistent   │
                                                    │    HTTP/2 conn      │
                                                    └──────────┬──────────┘
Claude Code ◀──stdout── nc ◀────────────────────────────────────┘
```

## Setup

### Build

```bash
cd ~/git/cc-tool-reviewer
go build -o cc-tool-reviewer .
```

### Configure Claude Code hook

Add to `~/.claude/settings.json`:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "nc -w 5 -U /tmp/cc-tool-reviewer.sock"
          }
        ]
      }
    ]
  }
}
```

### Start the daemon

```bash
./cc-tool-reviewer
```

Or with a custom socket path:

```bash
./cc-tool-reviewer --socket /tmp/my-reviewer.sock
```

### Environment

The daemon needs access to the Anthropic API. It uses the standard `ANTHROPIC_API_KEY` environment variable, or inherits `ANTHROPIC_BASE_URL` if set (e.g., when started from within a Claude Code session).

Settings are loaded from:
- `$CLAUDE_CONFIG_DIR/settings.json` (falls back to `~/.claude/settings.json`)
- `$CLAUDE_CONFIG_DIR/settings.local.json`
- `.claude/settings.json` (project-level)
- `.claude/settings.local.json` (project-level)

## How it works

1. Claude Code fires the `PreToolUse` hook, piping JSON to `nc`
2. `nc` forwards it to the daemon via Unix socket (~4ms overhead)
3. The daemon checks the command against your allow/deny rules locally
4. If it matches an allow or deny rule → empty response (Claude Code handles it normally)
5. If it's in the "ask zone" → calls Haiku 4.5 with your allow list as context
6. Haiku decides: **allow** (skip the prompt) or **ask** (show the prompt as usual)
7. The AI never denies — it either allows or defers to you

### Compound command detection

Simple commands like `rg foo` match locally. But compound commands containing `&&`, `||`, `;`, newlines, or subshells (`$(...)`) bypass local matching and are always sent to the AI. This is because:

- `cd ~/git/x && git log` doesn't match `Bash(cd:*)` in Claude Code's real matcher
- The AI can evaluate each part and recognize both are individually allowed
- Multi-line scripts need semantic understanding, not prefix matching

## Graceful degradation

If the daemon isn't running, `nc` fails with a non-zero exit code (but not exit code 2). Claude Code treats this as a no-op and falls through to the normal permission prompt. Nothing breaks.

## Testing

```bash
go test -v ./...
```

The integration tests start an in-process server (no daemon needed) and verify:
- `wantLocal` — command must be resolved by local matching, never hitting the API
- `wantAPI` — command must reach the AI (fails if the local matcher incorrectly short-circuits)

## Performance

| Scenario | Latency |
|----------|---------|
| Local match (allow/deny rule) | ~4ms (`nc` overhead) |
| API call (cold connection) | ~1000ms |
| API call (warm connection) | ~700ms |
| Daemon not running (fallback) | ~4ms (nc fails fast) |

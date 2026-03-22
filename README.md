# cc-tool-reviewer

<p align="center">
  <a href="#the-problem">The problem</a> &bull;
  <a href="#getting-started">Getting started</a> &bull;
  <a href="#how-it-works">How it works</a> &bull;
  <a href="#architecture">Architecture</a> &bull;
  <a href="#performance">Performance</a> &bull;
  <a href="#build-from-source">Build from source</a>
</p>

A fast, daemon-based AI reviewer for [Claude Code](https://docs.anthropic.com/en/docs/claude-code) tool calls. Reduces permission prompts by using Haiku 4.5 to evaluate commands that don't match your explicit allow/deny rules but are still consistent with what you've permitted.

On macOS, shows a translucent floating HUD for approve/deny decisions. Designed for workflows with multiple background agents running — glance at the HUD and approve (Cmd+Enter), deny, or defer (Esc) without switching to a terminal.

---

## The problem

Claude Code's permission system gives you allow and deny lists, but anything that doesn't match either becomes an "ask" that interrupts your flow. You could use `--dangerously-skip-permissions` to avoid this, but then you have no safety net at all. One bad command and your untracked files are gone, your keys are exfiltrated, or your production database is dropped.

cc-tool-reviewer sits between these two extremes. You keep your allow/deny rules, and for everything in the gray area, an AI reviewer decides if the command is consistent with what you've already permitted. If it's not sure, you get a native dialog to approve or deny with full context, not a terminal prompt you have to context-switch to.

## Getting started

### 1. Install

```bash
curl -sL https://raw.githubusercontent.com/anish749/cc-tool-reviewer/main/install.sh | bash
```

Installs to `~/.local/bin/`. On macOS, also compiles the native approval dialog (requires Xcode command line tools).

To install to a different directory:

```bash
curl -sL https://raw.githubusercontent.com/anish749/cc-tool-reviewer/main/install.sh | INSTALL_DIR=/usr/local/bin bash
```

### 2. Configure the Claude Code hook

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

Do **not** use `nc -w` (timeout). The native dialog needs time for user interaction. If the daemon isn't running, `nc` fails immediately on connect, so there's no hang risk.

### 3. Set up credentials

The daemon needs access to the Anthropic API. Set one of:

- **`ANTHROPIC_API_KEY`** — an API key from the [Anthropic Console](https://console.anthropic.com/)
- **`ANTHROPIC_AUTH_TOKEN`** — a Bearer token, if using an LLM gateway or proxy

### 4. Start the daemon

```bash
cc-tool-reviewer
```

Or with a custom socket path:

```bash
cc-tool-reviewer --socket /tmp/my-reviewer.sock
```

Start the daemon outside of Claude Code (e.g., from a shell alias or launch script), since the hook would interfere with starting it from within a Claude Code session.

Settings are hot-reloaded. No need to restart the daemon when you change your allow/deny rules.

---

## How it works

1. Claude Code fires the `PreToolUse` hook, piping JSON to `nc`
2. `nc` forwards it to the daemon via Unix socket (~4ms overhead)
3. The daemon decides what to do based on the tool type:

**Auto-allowed tools.** `WebFetch` and `WebSearch` are approved instantly with no matching or AI call. The daemon logs the URL/query for visibility.

**Bash commands.** Checked against your allow/deny rules locally:
- Matches an allow or deny rule → empty response, Claude Code handles it normally
- Compound command (`&&`, `||`, `;`, multi-line, subshells) → sent to the AI, since prefix matching can't evaluate these
- No match ("ask zone") → calls Haiku 4.5 with your allow list as context

**AI says "allow"** → tool call proceeds, no prompt.

**AI says "ask"** → on macOS, a translucent floating HUD appears with:
- Conversation title and working directory
- Recent user messages and tool call history
- The command and AI's reason for flagging it
- A feedback text field (sent back to Claude as context when denying)
- Three buttons:
  - **Approve** (Cmd+Enter) — allow the tool call
  - **Deny** — block it, with optional feedback text sent back to Claude
  - **Later** (Esc) — defer to Claude Code's terminal prompt

Keyboard shortcuts let you approve or dismiss without reaching for the mouse. On non-macOS systems, "ask" falls through to Claude Code's terminal prompt.

### Compound command detection

Simple commands like `rg foo` match locally. Compound commands containing `&&`, `||`, `;`, newlines, or subshells (`$(...)`) bypass local matching and are always sent to the AI. `cd ~/git/x && git log` doesn't match `Bash(cd:*)` in Claude Code's real matcher, but the AI can evaluate each part and recognize both are individually allowed.

### Settings

Settings are loaded from (and hot-reloaded on change):
- `$CLAUDE_CONFIG_DIR/settings.json` (falls back to `~/.claude/settings.json`)
- `$CLAUDE_CONFIG_DIR/settings.local.json`
- `.claude/settings.json` (project-level)
- `.claude/settings.local.json` (project-level)

### Graceful degradation

If the daemon isn't running, `nc` fails with a non-zero exit code (but not exit code 2). Claude Code treats this as a no-op and falls through to the normal permission prompt. Nothing breaks.

---

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

### Why not Claude Code's built-in `type: "prompt"` hook?

Claude Code has a built-in `prompt` hook type that sends tool calls to a model for review. It handles the API connection internally. But:

| | Built-in `prompt` hook | cc-tool-reviewer |
|---|---|---|
| **Fires on** | Every matching tool call | Only "ask zone" calls |
| **`rg foo` latency** | ~700ms (API call) | ~0ms (local match) |
| **Prompt context** | Generic, user-defined | Injects your actual allow list |
| **Compound commands** | No special handling | Detects `&&`, `\|\|`, `;`, multi-line and routes to AI |

cc-tool-reviewer replicates your allow/deny matching locally and only calls the API for the "ask zone". Most tool calls (~90%) are resolved in under 5ms with no API call.

---

## Performance

| Scenario | Latency |
|----------|---------|
| Auto-allowed (WebFetch, WebSearch) | ~4ms (`nc` overhead) |
| Local match (allow/deny rule) | ~4ms (`nc` overhead) |
| API call (cold connection) | ~1000ms |
| API call (warm connection) | ~700ms |
| Daemon not running (fallback) | ~4ms (nc fails fast) |

---

## Build from source

```bash
git clone https://github.com/anish749/cc-tool-reviewer.git
cd cc-tool-reviewer
make install
```

The Makefile builds the Go daemon and the Swift dialog (on macOS) from source and installs both to `~/.local/bin/`.

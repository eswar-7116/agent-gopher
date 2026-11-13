# Agent Gopher

An agentic AI coding assistant for the terminal, written in Go. It connects to any OpenAI-compatible API, runs tools on your behalf (shell commands, file I/O, web search, MCP servers), and stays out of your way with a lightweight TUI built on [Bubble Tea](https://github.com/charmbracelet/bubbletea).

[![Go CI](https://github.com/eswar-7116/agent-gopher/actions/workflows/go.yml/badge.svg)](https://github.com/eswar-7116/agent-gopher/actions/workflows/go.yml)

---

## Features

- **Terminal UI**: Full-screen TUI with streaming responses, input history (↑/↓), scroll (PgUp/PgDn), and slash-command suggestions with tab completion.
- **Streaming**: Assistant replies stream token-by-token as they arrive.
- **Tool use**: The agent can read, write and edit files (search/replace), run shell commands (foreground and background), search the web (Tavily), and kill processes — all as OpenAI function calls.
- **Permission system**: Shell commands and file writes require approval by default. Commands are shown in full (long ones scroll); writes and edits show a colored diff first. You can allow once, always-allow a command prefix (or all writes for the session), or deny with an alternative instruction the agent follows instead. Subcommand-aware whitelisting covers `git`, `go`, `docker`, `npm`, and more; allowed prefixes take effect immediately and persist to `config.json`. Whitelisted prefixes never auto-approve chained, piped, redirected or substituted commands (`;`, `&&`, `|`, `>`, `$(…)`, backticks).
- **Context management**: Tool output is capped, token usage is shown in the status line, and the conversation is summarized automatically when it nears the context window (or on demand with `/compact`). Failed commands return their output to the model, so it can see what went wrong.
- **MCP server support**: Connect any [Model Context Protocol](https://modelcontextprotocol.io/) server via stdio and its tools appear alongside the built-in ones.
- **Sub-agent delegation**: The agent can spin off a sub-agent with a fresh context and the same toolset to handle research or exploration tasks, keeping the parent's context clean.
- **Plan mode**: A read-only mode (`/plan`) where the agent can only investigate (read files, read-only shell commands like `ls`, `grep`, `rg`, `find`, `git status/log/diff`, search, delegate) and must respond with an implementation plan instead of making changes. Toggle it back with `/normal`.
- **AGENTS.md**: Drop an `AGENTS.md` file in your project root and its contents are injected as system-prompt instructions — project-specific conventions without touching config.
- **Skills**: Reusable instruction sets stored as `SKILL.md` files under `~/.agent-gopher/skills/<name>/`. The agent loads matching skills on demand via the `use_skill` tool.
- **Memory**: Global and per-project memory the agent can read, write, and forget. Managed via `/gopher memory`, `/gopher remember`, `/gopher forget`, or by the agent itself through a tool call.
- **Session persistence**: Conversations are stored as append-only JSONL files under `~/.agent-gopher/projects/<project>/`, surviving crashes and allowing future session restore.
- **Message queuing**: Type while the agent is working — messages queue and inject into the conversation at the next step. Alt+Enter interrupts the current run and force-sends queued messages immediately.
- **Cross-platform**: Builds and tests on Linux, macOS, and Windows (CI runs on all three).
- **Any OpenAI-compatible API**: Point it at OpenRouter, a local Ollama instance, or any provider that speaks the OpenAI chat completions API.

---

## Quick Start

### 1. Clone and build

```bash
git clone https://github.com/eswar-7116/agent-gopher.git
cd agent-gopher
go build -o agent-gopher .
```

### 2. Configure

On first run, Agent Gopher creates a default config at `~/.agent-gopher/config.json`. Fill in your API key and base URL (or set them through [environment variables](#environment-variables)):

```jsonc
{
  "api_key": "<your_api_key>",
  "base_url": "<your_base_url>",        // e.g. "https://openrouter.ai/api/v1"
  "model": "openrouter/free",
  "theme": "default",                    // set with /theme, or any theme under ~/.agent-gopher/themes
  "context_window": 128000,              // model context size in tokens; compaction starts at 80%
  "permissive_shell": false,             // true skips command approval prompts
  "permissive_write": false,             // true skips file write approval prompts
  "whitelisted_cmds": [],                // always-allowed command prefixes
  "tavily_api_key": "",                  // optional, enables the web_search tool
  "mcp_servers": []                      // optional MCP server configs
}
```

### 3. Run

```bash
./agent-gopher
```

---

## Configuration

> **Config path:** `~/.agent-gopher/config.json`

| Field | Default | Purpose |
|---|---|---|
| `api_key` | _(required)_ | API key for your LLM provider |
| `base_url` | _(required)_ | Base URL of an OpenAI-compatible API |
| `model` | `openrouter/free` | Model identifier to use |
| `theme` | `default` | TUI color theme; set with `/theme`, or drop your own under `~/.agent-gopher/themes/` |
| `context_window` | `128000` | Model context size in tokens; the conversation is compacted at 80% |
| `permissive_shell` | `false` | Skip shell command approval prompts |
| `permissive_write` | `false` | Skip file write/edit approval prompts |
| `whitelisted_cmds` | `[]` | Command prefixes that are always allowed |
| `tavily_api_key` | `""` | Tavily API key; `web_search` is only offered when set |
| `mcp_servers` | `[]` | MCP servers to connect on startup (see below) |

### Environment variables

These override the matching config values. They are never written back to `config.json`.

| Variable | Overrides |
|---|---|
| `GOPHER_API_KEY` | `api_key` |
| `GOPHER_BASE_URL` | `base_url` |
| `GOPHER_MODEL` | `model` |
| `TAVILY_API_KEY` | `tavily_api_key` |

### MCP Servers

Each MCP server entry specifies a stdio command:

```json
{
  "name": "example",
  "command": "npx",
  "args": ["-y", "@modelcontextprotocol/server-everything"]
}
```

All tools the server exposes are registered alongside the built-in tools as `<name>__<tool>` (characters other than letters, digits, `_` and `-` become `_`). Servers get 30 seconds to start and list their tools; if that fails, the error is shown in the chat when Agent Gopher opens.

---

## Tools

| Tool | Description |
|---|---|
| `read_file` | Read a file from disk (whole or a line range) |
| `write_file` | Write, overwrite or append to a file (asks first, with a diff) |
| `edit_file` | Replace exact text in a file (asks first, with a diff) |
| `shell_cmd` | Execute a shell command (foreground or background); read-only commands only in plan mode |
| `read_logs` | Read logs from a background process |
| `kill_process` | Kill a running background process |
| `web_search` | Search the web via Tavily (only with a Tavily key) |
| `delegate_task` | Spawn a sub-agent for a self-contained task |
| `use_skill` | Load a skill's instructions |
| `save_memory` | Persist a fact to global or project memory |
| _(MCP tools)_ | Any tools exposed by connected MCP servers |

---

## Slash Commands

| Command | Description |
|---|---|
| `/mode [plan\|normal]` | Switch mode (no argument toggles) |
| `/plan` | Switch to plan mode (read-only) |
| `/normal` | Switch to normal mode |
| `/theme [name]` | Set the TUI color theme |
| `/compact` | Summarize the conversation to free up context |
| `/gopher memory [global\|project]` | List remembered facts |
| `/gopher remember [global\|project] <fact>` | Save a memory |
| `/gopher forget [global\|project] <n\|text>` | Remove a memory |
| `/exit`, `/quit` | Quit |

---

## Keyboard Shortcuts

| Key | Action |
|---|---|
| Enter | Send message |
| Alt+Enter | Interrupt running agent and force-send queued messages |
| Esc | Cancel the running task |
| ↑ / ↓ | Navigate input history |
| PgUp / PgDn | Scroll the conversation |
| Tab | Complete a slash-command suggestion |
| Ctrl+C | Quit |

### During a permission prompt

| Key | Action |
|---|---|
| y / Enter | Allow this command or write |
| a | Always allow this command prefix, or allow all writes for this session |
| n / Esc | Deny (then type an alternative instruction, or Enter to skip) |
| ← / → | Scroll a long command |
| PgUp / PgDn | Review the diff of a pending write |

---

## Project Instructions

Create an `AGENTS.md` file in your project root to give the agent project-specific instructions. Its contents are injected into the system prompt automatically.

---

## Skills

Skills are reusable instruction sets. Create a directory under `~/.agent-gopher/skills/` with a `SKILL.md` file:

```
~/.agent-gopher/skills/
  deploy/
    SKILL.md
  review/
    SKILL.md
```

Each `SKILL.md` can have optional YAML frontmatter for `name` and `description`. The agent sees the skill list and calls `use_skill` to load the full instructions when a task matches.

---

## Architecture

```
main.go                      Entry point: config, MCP connections, TUI launch
internal/
  agent/                     Core agent loop: streaming, tool dispatch, sub-agents, plan mode, AGENTS.md
  config/                    JSON config load/save
  llm/                       OpenAI client wrapper
  mcpclient/                 MCP stdio client
  memory/                    Global and per-project memory store + tool
  paths/                     Resolves all on-disk paths at startup
  session/                   Append-only JSONL session persistence
  skills/                    Skill loader and tool
  tools/                     Built-in tool registry (file I/O and edits, diffs, permissions, shell, web search, process management)
  tui/                       Bubble Tea terminal UI
```

---

## Testing

```bash
go test ./...            # unit tests
go test -race ./...      # with race detector
```

CI runs build and test on Linux, macOS, and Windows.

---

## Performance

- **Race-free under real concurrency.** `go test -race ./...` passes clean across every package. The TUI, session writer, and agent loop all share mutable state across goroutines (streaming, input queuing, sub-agent delegation), and this is checked on every CI run, not just asserted here.
- **Sub-millisecond tool dispatch.** File I/O and diffing (including a unified diff over a 5,000-line file ahead of a write/edit approval prompt) run in single-digit-microseconds to low-single-digit-milliseconds, measured by the benchmark suite alongside the code it covers (e.g. [`internal/tools/bench_test.go`](internal/tools/bench_test.go)).
- **Found and fixed a scaling bottleneck before it mattered.** The agent re-marshals its conversation history every step to track context usage; benchmarking showed this cost grows linearly with conversation length (~4x the time at 4x the turns). That's what motivated automatic context compaction (`/compact`), which keeps this cost bounded instead of letting it grow with session length.

---

## License

Agent Gopher is released under the [MIT License](LICENSE).

---

**<p align="center">If you like this project, please consider giving this repo a star 🌟</p>**

# Agent Gopher

An agentic AI coding assistant for the terminal, written in Go. It connects to any OpenAI-compatible API, runs tools on your behalf (shell commands, file I/O, web search, MCP servers), and stays out of your way with a lightweight TUI built on [Bubble Tea](https://github.com/charmbracelet/bubbletea).

[![Go CI](https://github.com/eswar-7116/agent-gopher/actions/workflows/go.yml/badge.svg)](https://github.com/eswar-7116/agent-gopher/actions/workflows/go.yml)

---

## Features

- **Terminal UI**: Full-screen TUI with streaming responses, input history (↑/↓), scroll (PgUp/PgDn), and slash-command suggestions with tab completion.
- **Streaming**: Assistant replies stream token-by-token as they arrive.
- **Tool use**: The agent can read/write files, run shell commands (foreground and background), search the web (Tavily), and kill processes — all as OpenAI function calls.
- **Shell permission system**: Commands require approval by default. You can allow once, always-allow a command prefix, or deny with an alternative instruction the agent follows instead. Subcommand-aware whitelisting covers `git`, `go`, `docker`, `npm`, and more; allowed prefixes persist to `config.json`.
- **MCP server support**: Connect any [Model Context Protocol](https://modelcontextprotocol.io/) server via stdio and its tools appear alongside the built-in ones.
- **Sub-agent delegation**: The agent can spin off a sub-agent with a fresh context and the same toolset to handle research or exploration tasks, keeping the parent's context clean.
- **Plan mode**: A read-only mode (`/plan`) where the agent can only investigate (read files, search, delegate) and must respond with an implementation plan instead of making changes. Toggle it back with `/normal`.
- **AGENTS.md**: Drop an `AGENTS.md` file in your project root and its contents are injected as system-prompt instructions — project-specific conventions without touching config.
- **Skills**: Reusable instruction sets stored as `SKILL.md` files under `~/.agent-gopher/skills/<name>/`. The agent loads matching skills on demand via the `use_skill` tool.
- **Memory**: Global and per-project memory the agent can read, write, and forget. Managed via `/gopher memory`, `/gopher remember`, `/gopher forget`, or by the agent itself through a tool call.
- **Session persistence**: Conversations are stored as append-only JSONL files under `~/.agent-memory/`, surviving crashes and allowing future session restore.
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

On first run, Agent Gopher creates a default config at `~/.agent-gopher/config.json`. Fill in your API key and base URL:

```jsonc
{
  "api_key": "<your_api_key>",
  "base_url": "<your_base_url>",        // e.g. "https://openrouter.ai/api/v1"
  "model": "openrouter/free",
  "permissive_shell": false,             // true skips command approval prompts
  "whitelisted_cmds": [],                // always-allowed command prefixes
  "tavily_api_key": "",                  // optional, enables web_search tool
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
| `permissive_shell` | `false` | Skip shell command approval prompts |
| `whitelisted_cmds` | `[]` | Command prefixes that are always allowed |
| `tavily_api_key` | `""` | Tavily API key for the `web_search` tool |
| `mcp_servers` | `[]` | MCP servers to connect on startup (see below) |

### MCP Servers

Each MCP server entry specifies a stdio command:

```json
{
  "name": "example",
  "command": "npx",
  "args": ["-y", "@modelcontextprotocol/server-everything"]
}
```

All tools the server exposes are registered alongside the built-in tools.

---

## Tools

| Tool | Description |
|---|---|
| `read_file` | Read a file from disk |
| `write_file` | Write or overwrite a file |
| `shell_cmd` | Execute a shell command (foreground or background) |
| `read_logs` | Read logs from a background process |
| `kill_process` | Kill a running background process |
| `web_search` | Search the web via Tavily |
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
| y / Enter | Allow this command |
| a | Always allow this command prefix |
| n / Esc | Deny (then type an alternative instruction, or Enter to skip) |

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
  tools/                     Built-in tool registry (file I/O, shell, web search, process management)
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

## License

Agent Gopher is released under the [MIT License](LICENSE).

---

**<p align="center">If you like this project, please consider giving this repo a star 🌟</p>**

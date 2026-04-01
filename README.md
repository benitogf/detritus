# detritus

MCP knowledge base server + Agent Plugin. Exposes coding knowledge as MCP tools and Agent Skills for AI assistants across VS Code, Windsurf, Cursor, and Claude Code.

## Architecture

detritus delivers knowledge through two complementary layers:

1. **Agent Plugin** (`plugin.json`, `skills/`, `agents/`, `instructions/`) — Skills, custom agents, and always-on instructions. Works in VS Code (Copilot) and Claude Code natively.
2. **MCP Server** (Go binary, stdio transport) — Semantic search across all docs via `kb_list`, `kb_get`, `kb_search` tools. Works in any editor with MCP support.

## Quick Install

Paste this into your AI assistant (Copilot Chat, Windsurf Cascade, Cursor, etc.):

> Follow the setup instructions at https://raw.githubusercontent.com/benitogf/detritus/main/templates/workflows/setup-detritus.md

This handles binary install, MCP config, and editor integration.

### Manual Install

**Linux / macOS / Windows (Git Bash):**
```bash
curl -sSL https://raw.githubusercontent.com/benitogf/detritus/main/install.sh | sh
```

**Windows (PowerShell):**
```powershell
irm https://raw.githubusercontent.com/benitogf/detritus/main/install.ps1 | iex
```

The install script downloads the binary and configures:
- **Windsurf**: `~/.codeium/windsurf/mcp_config.json`
- **VS Code**: `~/.config/Code/User/mcp.json` + shared prompts/instructions
- **Cursor**: `~/.config/Cursor/User/mcp.json` (Linux), `%APPDATA%\Cursor\User\mcp.json` (Windows)
- **Continue**: `~/.continue/mcpServers/` + `~/.continue/prompts/`

Restart your editor after install.

## Agent Plugin

The repository itself is an Agent Plugin. In VS Code/Claude Code, the plugin provides:

### Skills (invokable)

| Skill | Description |
|-------|-------------|
| `/truthseeker` | Elevated rigor — evidence-based reasoning, push back on assumptions |
| `/plan` | Requirements analysis workflow |
| `/plan-export` | Export planning docs with Mermaid diagrams |
| `/diagrams` | Mermaid diagram quick reference |
| `/create` | Scaffold a new project |
| `/grow` | KB improvement from conversation corrections |
| `/optimize` | KB retrieval optimization |
| `/research-first` | Exhaust resources before asking the user |
| `/testing` | Testing decision table |
| `/line-of-sight` | Flat code style — early returns, no deep nesting |
| `/setup-detritus` | Installation workflow |

### Skills (auto-loaded)

These are loaded automatically when relevant — no manual invocation needed:

ooo-package, ooo-auth, ooo-nopog, ooo-pivot, ooo-client-js, ooo-filters-internals, coding-style, go-modern, async-events, state-management, go-backend-async, go-backend-mock, go-backend-e2e

### Custom Agent

Select the **detritus** agent for a session with truthseeker principles, research-first behavior, and MCP knowledge base access pre-configured.

### Always-On Instructions

The `instructions/detritus.instructions.md` file applies to all files (`applyTo: "**"`) with distilled guardrails: push back with evidence, research before asking, prove before acting.

## MCP Tools

| Tool | Description |
|------|-------------|
| `kb_list` | List all available documents with descriptions |
| `kb_get` | Get a full document by name |
| `kb_search` | Semantic search across all documents |

## Included Documents

### Core
- **ooo-package** — Server setup, filters, CRUD, WebSocket subscriptions, custom endpoints

### Storage
- **ooo-nopog** — PostgreSQL storage adapter

### Infrastructure
- **ooo-pivot** — AP distributed multi-instance sync
- **ooo-auth** — JWT authentication
- **ooo-client-js** — JavaScript/React WebSocket client

### Testing
- **testing** — Testing index and decision table
- **go-backend-async** — Deterministic async testing
- **go-backend-mock** — Minimal mocking at boundaries
- **go-backend-e2e** — End-to-end lifecycle tests

### Patterns
- **go-modern** — Modern Go idioms (1.22+/1.24+)
- **coding-style** — Naming, error handling, formatting, commits
- **async-events** — Channel-based pub/sub, backpressure
- **state-management** — Single source of truth, immutable updates
- **line-of-sight** — Early returns, flat code structure

### Principles
- **truthseeker** — Evidence-based reasoning, pushback, intellectual humility
- **research-first** — Exhaust available resources before asking

## How It Works

All documents are embedded in the binary at compile time (`embed.FS`). No external files or runtime dependencies.

The `kb_get` tool description contains keyword-packed summaries. When the AI sees relevant keywords in your prompt, it automatically calls `kb_get` — no manual invocation needed.

Agent Skills provide the same knowledge in a format native to VS Code/Claude Code, with YAML frontmatter controlling invocability and auto-loading behavior.

## Troubleshooting

```bash
detritus --version
```

### Windsurf
1. Config: `~/.codeium/windsurf/mcp_config.json`
2. Binary path uses **forward slashes** (even on Windows)
3. **Full restart** required (File > Exit)

### VS Code
1. Config: `~/.config/Code/User/mcp.json` (Linux), `~/Library/Application Support/Code/User/mcp.json` (macOS)
2. Config uses **`"servers"`** key (not `"mcpServers"`)
3. Run `Developer: Reload Window`

### Cursor
1. Config: `~/.config/Cursor/User/mcp.json` (Linux), `%APPDATA%\Cursor\User\mcp.json` (Windows)
2. Uses **`"mcpServers"`** key

## Development

```bash
go test -v
go build -o detritus .
```

## Release

Uses [goreleaser](https://goreleaser.com/) for cross-platform builds. Push a tag to trigger GitHub Actions:

```bash
git tag v3.0.0
git push origin v3.0.0
```

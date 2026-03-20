# detritus

MCP (Model Context Protocol) knowledge base server. Exposes coding knowledge documents as tools that AI assistants can query on-demand.

## Quick Install (Windsurf)

Paste this into Windsurf Cascade:

> Follow the setup instructions at https://raw.githubusercontent.com/benitogf/detritus/main/templates/workflows/setup.md

This handles everything: binary install, MCP config, and project workflow files. In multi-root workspaces, it will ask which project should receive the workflow files.

To update, paste the same prompt again or run `/setup` if already installed.

## Manual Install

### Linux / macOS / Windows (Git Bash)

```bash
curl -sSL https://raw.githubusercontent.com/benitogf/detritus/main/install.sh | sh
```

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/benitogf/detritus/main/install.ps1 | iex
```

The install script downloads the binary and configures `~/.codeium/windsurf/mcp_config.json` automatically. Restart Windsurf after install.

## Tools

The server exposes 3 MCP tools:

| Tool | Description |
|------|-------------|
| `kb_list` | List all available documents with descriptions |
| `kb_get` | Get a full document by name (keyword-packed description enables auto-routing) |
| `kb_search` | Search across all documents for a topic or API name |

## Included Documents

### Core
- **ooo-package** — Server setup, filters, CRUD helpers, WebSocket subscriptions, custom endpoints, remote operations

### Storage
- **ooo-ko** — LevelDB persistent storage adapter
- **ooo-nopog** — PostgreSQL storage for large-scale data

### Infrastructure
- **ooo-pivot** — AP distributed multi-instance synchronization
- **ooo-auth** — JWT authentication
- **ooo-client-js** — JavaScript/React WebSocket client

### Testing
- **testing** — Testing index and decision table
- **testing-go-backend-async** — Deterministic async testing with WaitGroup
- **testing-go-backend-mock** — Minimal mocking at boundaries
- **testing-go-backend-e2e** — End-to-end lifecycle tests
- **async-events** — General async event principles (language-agnostic)

### Patterns
- **go-modern** — Modern Go idioms (1.22+/1.24+) with gopls modernize
- **scaffold-simple-service** — Template for new ooo+ko backend services
- **plan** — Requirements analysis workflow

### Principles
- **truthseeker** — Foundational principles: evidence-based reasoning, pushback, intellectual humility

## How It Works

All documents are embedded in the binary at compile time (`embed.FS`). No external files or runtime dependencies.

The `kb_get` tool description contains keyword-packed summaries of every document. When the AI sees relevant keywords in your prompt, it automatically calls `kb_get` to fetch the full document — no manual invocation needed.

## Troubleshooting

Verify the binary:

```bash
detritus --version
```

On Windows:
```powershell
& "$env:LOCALAPPDATA\detritus\detritus.exe" --version
```

If Windsurf doesn't load the MCP server after restart, check:
1. Config path: `~/.codeium/windsurf/mcp_config.json`
2. Binary path uses **forward slashes** (even on Windows)
3. **Full restart** required (File > Exit, not just close window)
4. On Windows, antivirus may block unsigned executables

## Development

```bash
go test -v
go build -o detritus .
```

## Release

Uses [goreleaser](https://goreleaser.com/) for cross-platform builds:

```bash
goreleaser release --clean
```

Or via GitHub Actions on tag push.

# detritus

MCP (Model Context Protocol) knowledge base server. Exposes coding knowledge documents as tools that AI assistants can query on-demand.

## Install

### Linux / macOS

```bash
curl -sSL https://raw.githubusercontent.com/benitogf/detritus/main/install.sh | sh
```

Installs to `/usr/local/bin/detritus`.

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/benitogf/detritus/main/install.ps1 | iex
```

Installs to `%LOCALAPPDATA%\detritus\detritus.exe`.

## Configure Windsurf

Add to `~/.codeium/windsurf/mcp_config.json`:

### Linux / macOS

```json
{
  "mcpServers": {
    "detritus": {
      "command": "/usr/local/bin/detritus",
      "args": [],
      "disabled": false
    }
  }
}
```

### Windows

```json
{
  "mcpServers": {
    "detritus": {
      "command": "C:/Users/YOUR_USER/AppData/Local/detritus/detritus.exe",
      "args": [],
      "disabled": false
    }
  }
}
```

Restart Windsurf to activate.

## Project Files

The setup also installs project-level files (`.windsurfrules` and workflow templates) that enable Windsurf to auto-discover detritus capabilities. Run from your project root:

### Linux / macOS

```bash
mkdir -p .windsurf/workflows && [ ! -f .windsurfrules ] && curl -sSL https://raw.githubusercontent.com/benitogf/detritus/main/templates/.windsurfrules -o .windsurfrules; for f in setup.md _truthseeker.md plan.md scaffold-simple-service.md; do [ ! -f ".windsurf/workflows/$f" ] && curl -sSL "https://raw.githubusercontent.com/benitogf/detritus/main/templates/workflows/$f" -o ".windsurf/workflows/$f"; done
```

### Windows (PowerShell)

```powershell
New-Item -ItemType Directory -Path .windsurf/workflows -Force | Out-Null; if (-not (Test-Path .windsurfrules)) { irm https://raw.githubusercontent.com/benitogf/detritus/main/templates/.windsurfrules | Set-Content .windsurfrules -Encoding UTF8 }; @('setup.md','_truthseeker.md','plan.md','scaffold-simple-service.md') | ForEach-Object { if (-not (Test-Path ".windsurf/workflows/$_")) { irm "https://raw.githubusercontent.com/benitogf/detritus/main/templates/workflows/$_" | Set-Content ".windsurf/workflows/$_" -Encoding UTF8 } }
```

These files won't overwrite existing ones, so repo-specific customizations are preserved.

## Update

Re-run the install command for your platform.

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

### Patterns
- **go-modern** — Modern Go idioms (1.22+/1.24+) with gopls modernize
- **scaffold-simple-service** — Template for new ooo+ko backend services
- **plan** — Requirements analysis workflow

### Principles
- **_truthseeker** — Foundational principles: evidence-based reasoning, pushback, intellectual humility

## How It Works

All documents are embedded in the binary at compile time (`embed.FS`). No external files or runtime dependencies.

The `kb_get` tool description contains keyword-packed summaries of every document. When the AI sees relevant keywords in your prompt, it automatically calls `kb_get` to fetch the full document — no manual invocation needed.

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

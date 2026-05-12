# detritus

MCP knowledge base server. Exposes coding knowledge as MCP tools for AI assistants across VS Code, Windsurf, Cursor, Claude Code, and Verdent.

## Install

**Codex plugin:**
```bash
codex plugin marketplace add benitogf/detritus
```

The plugin manifest lives at `.codex-plugin/plugin.json`; the bundled MCP
launcher downloads the latest release binary into a local cache on first use,
then starts the server.

**Linux / macOS / Git Bash:**
```bash
curl -sSL https://raw.githubusercontent.com/benitogf/detritus/main/install.sh | sh
```

**Windows PowerShell:**
```powershell
irm https://raw.githubusercontent.com/benitogf/detritus/main/install.ps1 | iex
```

Or download from [Releases](https://github.com/benitogf/detritus/releases), place in PATH, then:

```bash
detritus --setup
```

## MCP Tools

| Tool | Description |
|------|-------------|
| `kb_list` | List all documents with descriptions |
| `kb_get` | Get document by name (optional `section` param) |
| `kb_search` | Full-text search across all documents |
| `kb_sections` | List sections in a document |

## Slash Commands

| Command | Doc |
|---------|-----|
| `/truthseeker` | Evidence-based reasoning |
| `/plan` | Requirements analysis |
| `/testing` | Testing decision table |
| `/grow` | KB improvement from corrections |
| `/optimize` | KB retrieval optimization |
| `/janitor` | Recurring safe codebase maintenance |
| `/coding-style` | Naming, error handling, commits |
| `/go-modern` | Modern Go idioms (1.22+) |
| `/line-of-sight` | Flat code, early returns |

Codex displays plugin commands with the plugin namespace, for example
`/detritus:plan`.

## Update

```bash
detritus --update
```

Or, from an AI assistant with detritus skills installed, invoke `/detritus-update`.

## Development

```bash
go generate ./...   # rebuild index
go test ./...
go build -o detritus .
```

Push a tag to release:

```bash
git tag v3.1.0
git push origin v3.1.0
```

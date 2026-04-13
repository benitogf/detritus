# detritus

MCP knowledge base server. Exposes coding knowledge as MCP tools for AI assistants across VS Code, Windsurf, Cursor, Claude Code, and Verdent.

## Install

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
| `/coding-style` | Naming, error handling, commits |
| `/go-modern` | Modern Go idioms (1.22+) |
| `/line-of-sight` | Flat code, early returns |

## Update

```bash
detritus --update
```

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

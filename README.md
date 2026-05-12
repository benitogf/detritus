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
| `/coding-style` | Naming, error handling, commits |
| `/go-modern` | Modern Go idioms (1.22+) |
| `/line-of-sight` | Flat code, early returns |

Codex displays plugin commands with the plugin namespace, for example
`/detritus:plan`.

### GitHub workflow family

The `gh-*` skills are a coordinated set dispatched by the `/gh` router. The router classifies the input (URL, ref, or free text) and hands off to one of the sub-skills.

| Command | Doc |
|---------|-----|
| `/gh` | Router for the family — picks a sub-skill from context |
| `/gh-issue-create` | Draft a GitHub issue from conversation, post with attribution footer |
| `/gh-issue-work` | Take an issue end-to-end: branch, fix, test, commit, push, self-review, open PR |
| `/gh-feedback-work` | Address open review feedback on a PR; updates PR body in place, never posts comments |
| `/gh-self-review` | Pre-flight self-audit of pending local changes — **delegated to a fresh sub-agent** so the review runs without the conversational context that produced the code |
| `/gh-pr` | Hard-review someone else's PR; posts an APPROVE or REQUEST_CHANGES review via `gh api` |

Two patterns hold the family together:

- **`meta/review-rigor`** (category: `principles`, do-not-invoke-directly) is the shared analysis checklist that both `/gh-pr` and `/gh-self-review` follow. Tightening the review bar happens in one place; both skills inherit. Treat it like `truthseeker` — loaded by other skills via `kb_get`, never a standalone slash command.
- **Fresh-agent delegation in `/gh-self-review`**: the wrapping skill collects scope + diff + intent signals, then spawns a sub-agent via the `Agent` tool to do the actual review. The sub-agent has no prior conversation context, so the audit isn't biased by the discussion that produced the code. This doesn't eliminate the shared-training blind spot, but eliminates the conversational bias — the largest source of self-review false negatives.

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

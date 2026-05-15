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
| `/smith` | Feature loop — `/plan` gate, then build until merged, then audit the changed code |
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

### Loop family — recurring work against your workspace

Two recurring-loop commands share the same scheduling, durability, and state mechanics, separated by intent:

| Command | Doc | Intent |
|---------|-----|--------|
| `/janitor` | `meta/janitor` | Preserve behavior. Recurring safe audits + small fixes against the codebase |
| `/smith` | `meta/smith` | Add behavior. `/plan` gate to settle scope, then build until merged, then transition into a janitor-style audit phase on the changed code |

Both run against the actual workspace or repo the user is in — Desktop Routines or an external scheduler (cron, launchd, systemd, Task Scheduler) by default on Claude Code, with equivalent local-checkout schedulers on other platforms. A gitignored scratchpad (`.janitor/<slug>.md` or `.smith/<slug>.md`) carries plan-state across cold ticks. Opt-in GitHub-state-only modes (Cloud Routines, Codex `worktree`, GitHub Actions) exist for maintaining a remote repo without a local checkout.

The shared mechanics live in `meta/loop-core` (do-not-invoke-directly): scratchpad layout, durability rule, audit-to-verify cadence, truthseeker pause on user critique, honest regression reporting, skip-streak guardrail (default eight ticks or two hours of idle wakes), mid-loop pivot via scratchpad orientation, `/gh` delivery routing. Each command references it instead of restating; tightening any shared rule happens in one place.

Example invocations:

```
/janitor                                                # whole-repo maintenance, default rubric
/janitor flaky tests                                    # topic-focused maintenance
/janitor get test wall time under 145s                  # maintenance with a measurable goal
/smith add typed cancellation to subscribe-list         # feature loop; first tick is /plan
/smith add SSE fallback to the live worker overnight    # feature loop with cadence hint
```

Mid-loop pivots are handled in chat — saying *"please nudge the loop to focus on perf only"* updates the scratchpad's *Current orientation* field (after a truthseeker pause) and the next tick honors the new focus without re-invocation. The same pattern works for both commands. Adapter-specific scheduling lives in `meta/janitor-platforms`.

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

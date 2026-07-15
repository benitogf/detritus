# detritus

> 🪨 **[benitogf.github.io/detritus](https://benitogf.github.io/detritus/)** — what detritus does, at a glance: the knowledge base, code intelligence, candyland, and the delivery pipeline.

Detritus is an MCP server for AI coding assistants. It exposes a curated
knowledge base of engineering doctrine (`kb_*` tools) and live codebase
exploration (`code_*`) — and layers
on slash commands that share one delivery pipeline: plan, build,
review-with-rework, deliver. Those commands run either in your session (`/plan`,
`/smith`, `/forge`, `/janitor`, `/gh-*`, `/todo`, …) or hand off to the
out-of-process **candyland sidecar** for multi-agent, dashboard-observable
delivery (`/candyland`, `/quest`). No flow merges on
its own — the human merge stays the gate.

## Install

Ask your AI coding tool to install Detritus from this repository:

```text
please install detritus mcp https://github.com/benitogf/detritus
```

Or, if your tool supports Markdown links:

```text
please install detritus mcp [benitogf/detritus](https://github.com/benitogf/detritus)
```

That is the human install path. You should not need to run Detritus as a CLI
tool yourself. Manual install details for an assistant or maintainer live in
[INSTALL.md](./INSTALL.md).

## Commands

Use Detritus by command name. This table is generated from `docs/flows/` (run
`detritus --readme`); the folder a doc lives in is its category. Internal
building blocks (`docs/core/`), agent roles (`docs/roles/`), and reference
material (`docs/ooo/`, `docs/patterns/`) are kb_get-only and intentionally
absent — they are not commands.

<!-- COMMANDS:START -->

### Plan

| Command | Use it for |
| --- | --- |
| `/plan` | Analyze requirements/feedback, create implementation plan, provide insights and questions |
| `/vibe` | Executive intake for autonomous delivery. |

### Build & maintain

| Command | Use it for |
| --- | --- |
| `/candyland` | Launch ONE candyland run in the sidecar — a tech-lead agent partitions the work, coders build it concurrently in worktrees, a reviewer loops fix→re-review until clean, then the run delivers. |
| `/forge` | Drive a settled plan to a PR with a parallel tech-lead + coders implementation loop, in-process. |
| `/janitor` | Create a recurring proactive code-maintenance worker that audits, safely fixes, verifies, self-reviews, and routes delivery through /gh. |
| `/quest` | The one flexible, persistent iterative loop in the candyland sidecar — a quest-lead ticks discover→triage→launch serial child runs against an objective + scope lens, with two delivery modes (converge-until-clean → one PR per impacted repo; per-finding → one PR per accepted finding). |
| `/smith` | Recurring loop that takes a feature from /plan all the way to an open PR — the in-session single-agent instantiation of the universal pipeline. |

### GitHub

| Command | Use it for |
| --- | --- |
| `/babysit` | Watch a single PR on an interval — fix any review feedback via /gh-feedback-work, and merge once an approval lands on the latest commit. |
| `/gh` | Router for GitHub issue/PR workflows — reads conversation context and dispatches to gh-issue-create, gh-issue-work, gh-feedback-work, gh-self-review, or gh-pr. |
| `/gh-feedback-work` | Address open review feedback on a PR, push fixes, and update the PR body in place — never posts issue/PR comments. |
| `/gh-issue-create` | Draft a GitHub issue from the current conversation, confirm with the user unless the post was already directed, post it with the Claude Code attribution footer, then offer next steps (/gh-issue-work, refine, or leave). |
| `/gh-issue-work` | Take a GitHub issue end-to-end — branch, fix, test, commit, push, self-review the diff, confirm with the user unless opening the PR was already directed, then open PR with a product-focused summary and the Claude Code attribution footer. |
| `/gh-pr` | Hard-review a GitHub PR under truthseeker rigor — verify the PR's own claims, hunt for fragility, demand evidence before flagging OR approving, and post an APPROVE or REQUEST_CHANGES review via `gh api`. |
| `/gh-pr-safe` | Hard-review a posted GitHub PR with the same truthseeker rigor as /gh-pr and auto-post an APPROVE or REQUEST_CHANGES review, but never mutate the working tree of the clone it runs from. |
| `/gh-self-review` | Deep self-audit of pending local changes (committed + uncommitted) under truthseeker rigor before commit/push/PR. |

### Project

| Command | Use it for |
| --- | --- |
| `/code` | Explicit entry point for code exploration backed by the zero-setup code_* tools (code_map, code_outline, code_graph) plus native Grep. |
| `/todo` | Cross-session todo management — router, conventions, and ALL everyday item operations (view, add, done, edit, defer, clear, file) in one doc. |

### Testing

| Command | Use it for |
| --- | --- |
| `/flaky-check` | Flaky test detection - manually-driven batches of -race runs that PRESERVE full failure logs, reproduce faithfully, and account for cross-test/ordering flakes |
| `/testing` | Testing index - entry point for all testing workflows |
| `/testing-go-backend-async` | Async testing - deterministic async event synchronization patterns |
| `/testing-go-backend-e2e` | E2E testing - consolidated tests covering full state lifecycles |
| `/testing-go-backend-mock` | Mock testing - minimal mocking at boundaries, simple state toggles |

### Principles & style

| Command | Use it for |
| --- | --- |
| `/coding-style` | Self-documenting code - naming, extraction, readability rules for AI |
| `/go-modern` | Modern Go patterns - auto-fix with gopls modernize after Go edits |
| `/line-of-sight` | Line-of-sight code style - flat code, early returns, separate error handling from business logic |
| `/truthseeker` | Foundational principles - ALWAYS ACTIVE, do not invoke |

### Maintainer

| Command | Use it for |
| --- | --- |
| `/absorb` | Close the learning loop on a shipped PR - resolve its candyland unit, fix outstanding blockers, and distill the review outcome into the KB |
| `/cleanup-extra-rules` | Remove all detritus-generated rule files, hook scripts, and the matching settings.json hook entries. |
| `/detritus-release` | Cut a release of detritus and/or candyland (bump → annotated tag → CI build), then update the local installation. |
| `/detritus-update` | Update detritus to the latest released version by running `detritus --update`. |
| `/grow` | Learn from conversation corrections - distill manual fixes into KB updates |
| `/learn` | Learn from candyland telemetry - mine failure signatures across runs/quests into audited KB updates |
| `/optimize` | Re-index and optimize KB docs for agent retrieval efficiency |
| `/setup-extra-rules` | Generate personalized Claude Code rule files and hook scripts based on the user's actual environment. |
| `/setup-superpowers` | Apply baseline Claude Code settings (deny list, status line, effort/thinking, autoMode environment). |

### Pdf

| Command | Use it for |
| --- | --- |
| `/pdf` | Generate a PDF to whatever format and style the user describes — general-purpose, multi-page, diagrams when useful. |
| `/pdf-management` | Turn the issue under discussion into a single-page decision PDF for a non-technical stakeholder — premise-only, plain real-world terms, no jargon, no component names or technical mechanisms. |
| `/pdf-tech` | Turn the issue under discussion into a single-page decision PDF at a bare-minimum, high-level technical register — name the component and mechanism conceptually, no code or deep internals. |

<!-- COMMANDS:END -->

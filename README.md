# detritus

Detritus is an MCP knowledge base for AI coding assistants. It gives the
assistant reusable guidance for planning, testing, GitHub work, project todos,
and codebase exploration.

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
building blocks (`docs/core/`) and agent roles (`docs/roles/`) are kb_get-only
and intentionally absent — they are not commands.

<!-- COMMANDS:START -->

### Plan

| Command | Use it for |
| --- | --- |
| `/plan` | Analyze requirements/feedback, create implementation plan, provide insights and questions |
| `/vibe` | Executive intake for autonomous delivery. |

### Build & maintain

| Command | Use it for |
| --- | --- |
| `/forge` | Drive a settled plan to a PR with a parallel tech-lead + coders implementation loop, in-process. |
| `/janitor` | Create a recurring proactive code-maintenance worker that audits, safely fixes, verifies, self-reviews, and routes delivery through /gh. |
| `/smith` | Recurring loop that takes a feature from /plan all the way to merged PR and then transitions into a janitor-style audit phase on the changed code. |

### GitHub

| Command | Use it for |
| --- | --- |
| `/gh` | Router for GitHub issue/PR workflows — reads conversation context and dispatches to gh-issue-create, gh-issue-work, gh-feedback-work, gh-self-review, or gh-pr. |
| `/gh-feedback-work` | Address open review feedback on a PR, push fixes, and update the PR body in place — never posts issue/PR comments. |
| `/gh-issue-create` | Draft a GitHub issue from the current conversation, confirm with the user unless the post was already directed, post it with the Claude Code attribution footer, then offer next steps (/gh-issue-work, refine, or leave). |
| `/gh-issue-work` | Take a GitHub issue end-to-end — branch, fix, test, commit, push, self-review the diff, confirm with the user unless opening the PR was already directed, then open PR with a product-focused summary and the Claude Code attribution footer. |
| `/gh-pr` | Hard-review a GitHub PR under truthseeker rigor — verify the PR's own claims, hunt for fragility, demand evidence before flagging OR approving, and post an APPROVE or REQUEST_CHANGES review via `gh api`. |
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
| `/cleanup-extra-rules` | Remove all detritus-generated rule files, hook scripts, and the matching settings.json hook entries. |
| `/detritus-update` | Update detritus to the latest released version by running `detritus --update`. |
| `/grow` | Learn from conversation corrections - distill manual fixes into KB updates |
| `/optimize` | Re-index and optimize KB docs for agent retrieval efficiency |
| `/setup-extra-rules` | Generate personalized Claude Code rule files and hook scripts based on the user's actual environment. |
| `/setup-superpowers` | Apply baseline Claude Code settings (deny list, status line, effort/thinking, autoMode environment). |

<!-- COMMANDS:END -->

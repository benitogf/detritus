---
description: Platform-specific scheduling adapters for /janitor. Keeps Codex, Claude Code, GitHub Actions, Cursor, Windsurf, and generic scheduler details out of the core janitor workflow.
category: meta
triggers:
  - janitor platform
  - janitor platforms
  - janitor scheduling
  - platform scheduling
  - codex janitor
  - claude janitor
  - claude code janitor
  - cursor janitor
  - windsurf janitor
  - github actions janitor
when: /janitor needs to create a recurring schedule on a specific platform or explain the nearest supported scheduler behavior.
related:
  - meta/janitor
  - meta/gh
  - meta/gh-self-review
---

# /janitor Platform Adapters

This document owns platform-specific scheduling behavior for `/janitor`. Do not duplicate these details in `meta/janitor`.

The core janitor workflow supplies the target, topics, requested cadence, loop contract, audit-agent contract, safety boundaries, verification rules, and `/gh` delivery rule. The adapter's job is only to schedule that loop on the host platform.

## Shared Adapter Rules

- Speak to the user in human terms: `every 5 minutes`, `hourly`, `overnight`, `weeknights`.
- Translate internally to the platform's scheduler syntax only when needed.
- Do not show cron strings, session IDs, lock files, API tokens, or low-level flags unless the user asks.
- Preserve the janitor loop exactly; platform adapters do not redefine what work is allowed.
- Start one run immediately when the platform supports it.
- If the requested cadence is below the platform's minimum, round up to the minimum and report the effective cadence; do not ask the user a clarifying question for this case.
- If the requested cadence is unsupported in a way that materially changes behavior (not just rounded up), choose the nearest safe option and say what changed.
- Every adapter must preserve target-scoped non-overlap: a wake checks existing branches, issues, PRs, or state before starting fresh audit work.

## `--platform auto`

Detect the current host when possible and choose the closest supported behavior:

1. Codex app: prefer a thread heartbeat for short intervals such as `5min`.
2. Codex app with hourly/weekly cadence: use a cron/worktree automation when the requested schedule fits detached work.
3. Claude Code: default to Cloud Routines via `/schedule` — they run on Anthropic infrastructure and survive without the user's machine, which is the typical fit for a janitor. Use Desktop Routines only when the target genuinely needs uncommitted local state or local-only MCP tools AND the user confirms the Desktop app will be open during wake windows; Desktop Routines do not fire while the Desktop app is closed.
4. GitHub Actions: create or recommend a scheduled workflow only when repository-hosted CI automation is acceptable.
5. Cursor or Windsurf: install the reusable command/workflow instructions, then require that recurrence be driven by platform UI support or an external scheduler if no native recurring scheduler is available.
6. Generic: provide a scheduler-ready prompt and leave recurrence to cron, launchd, systemd timers, Windows Task Scheduler, or the host orchestrator.

## Codex

Use the Codex app automation API.

- Codex execution modes map to *Durability* in `meta/janitor` (under *Durable Cross-Tick State*): `local` is durable; `worktree` is disposable. The specifics for each Codex automation type follow.
- For `5min` and other short intervals, create a heartbeat automation attached to the current thread. This preserves continuity and lets each wake continue pending work.
- For hourly, daily, or weekly detached maintenance, use a cron automation only when the janitor can run from a durable workspace. Prefer `local` execution against the resolved checkout when the loop needs gitignored `.janitor/` state across cold starts.
- Use `worktree` execution only for janitors that can fully reconstruct state from GitHub-side branches, issues, PRs, and the automation prompt. Do not assume uncommitted or gitignored `.janitor/` files from a previous worktree tick will exist on the next tick.
- Start one run immediately after creating the schedule so the user can confirm it works.
- The automation prompt must include the full janitor loop contract, target, topics, verification expectations, and concise reporting requirements.
- The runtime-derived scratchpad slug comes from the invocation root and topic, not from the Codex thread or automation id. Heartbeat and local cron ticks should read and update `.janitor/<slug>.md` in that resolved root; detached worktree ticks must either use a durable local checkout or report that the scratchpad cannot be persisted under the requested execution mode.
- Per-tick report files follow the same durability rule as the scratchpad: they are temporary handoff files inside `.janitor/`, and the main tick must fold them into the scratchpad before the wake ends. Worktree ticks that cannot preserve `.janitor/` between runs must not leave the only copy of audit detail in a disposable worktree.

## Claude Code

Claude Code Routines are the first-party scheduling primitive. Two variants share the same `/schedule` entry point:

- Claude Code execution modes map to *Durability* in `meta/janitor` (under *Durable Cross-Tick State*): Desktop Routines and the external scheduler fallback are durable; Cloud Routines are disposable (each tick clones the repository fresh, so the gitignored scratchpad does not persist). At setup, if the loop depends on the scratchpad (rolling metrics, multi-tick narrative, hazards queue), prefer Desktop Routines or the external scheduler. Otherwise Cloud Routines proceed under durability mode 2 (GitHub-state-only).
- **Cloud Routines** (`/schedule` → Routines → Cloud): run on Anthropic-managed infrastructure, clone the selected repository at the start of each run, and survive without the user's machine. Minimum interval is **1 hour**; cron expressions evaluating to a sub-hour cadence are rejected at routine-creation time. This is the **default** for `/janitor` — the work is GitHub-backed, runs unattended, and matches the "idle / non-office hours" intent.
- **Desktop Routines** (`/schedule` → Routines → Local): run locally and only fire while the Claude Code Desktop app is running. Minimum interval is 1 minute. Use only when the target genuinely needs uncommitted local state or local-only MCP tools AND the user confirms the Desktop app will be open during wake windows.
- **`/loop`**: session-scoped polling inside the current conversation; stops when the session ends. Not suitable for unattended or non-office-hour work.
- **External scheduler plus Claude Code CLI**: fallback when first-party Routines are unavailable on the user's plan or when the user explicitly wants local OS control.

### Cloud Routines (default path)

1. Resolve target, topics, and interval from the core janitor Inputs section.
2. Compose a self-contained janitor prompt. Routines run as fresh sessions, so the prompt must inline the target, topics or default review rubric, full janitor loop, audit-agent contract, main-agent contract, state/non-overlap rule, safety boundaries, verification expectations, and `/gh` delivery rule. Routines do not expand slash commands inside the prompt.
3. Invoke `/schedule` to create the routine. `/schedule` will prompt for repository access; pass the resolved target so the selection is pre-filled when possible.
4. If the requested cadence is below the 1-hour minimum, round up to 1 hour, create the routine, and state the effective cadence in the report. Do not fall through to Desktop Routines silently — Desktop requires the Desktop app open, which is a behavior change the user must confirm.
5. Trigger one immediate "Run now" so the user sees a first tick in the same conversation.
6. Report the effective cadence in human terms and include the routines management URL (`https://claude.ai/code/routines`) so the user can pause / edit / delete later via the UI.

Non-overlap on Cloud Routines is not automatic — the platform may fire overlapping ticks. The janitor loop handles this by inspecting open branches, draft PRs, and open issues on the target at every wake (loop step 3), since each run is a fresh session with no in-memory state from prior ticks.

### Desktop Routines (opt-in only)

Use only when both conditions hold:

- The target needs uncommitted local state or local-only MCP tools that Cloud Routines cannot reach.
- The user confirms the Claude Code Desktop app will be running during the requested wake windows.

Otherwise route to Cloud Routines. Desktop Routines that fire while the app is closed are silently skipped.

### External scheduler fallback

When Routines are unavailable, the agent — not the user — sets up the equivalent locally. Generate a `flock`-guarded entry that invokes `claude -p --maintenance --permission-mode bypassPermissions --session-id <stable-id> --output-format json "<janitor prompt>"` on the user's OS scheduler (cron, launchd, or Task Scheduler), wire it for them when permitted, and report in human terms exactly as the Cloud Routines path would. Cron syntax, lock files, and CLI flags stay internal unless the user asks how it works.

Claude Code hooks are useful for lifecycle behavior inside a tick (Setup `maintenance` matcher, Stop hooks), but hooks are not the source of recurrence.

## GitHub Actions

Use only when repository-hosted automation is acceptable and the work can run unattended in CI.

- Scheduled workflows use cron syntax and can be set as frequently as every 5 minutes, but GitHub Actions cron is best-effort and runs may be delayed or skipped under runner contention. Treat the interval as a floor, not a guarantee.
- Add `workflow_dispatch` so developers can start a tick manually.
- Use one concurrency group per target branch or workspace and set `cancel-in-progress: false` to avoid overlapping janitor runs.
- Prefer opening issues or PRs over pushing directly to protected branches.
- Treat secrets and permissions narrowly. Grant only the permissions required to create branches, issues, or PRs.

GitHub Actions is best for remote, auditable janitor ticks. It is a poor fit for interactive local context, local-only files, or IDE-specific state.

## Cursor

Use Cursor background agents for long asynchronous work when available.

- Start a background agent with the janitor prompt and target.
- Use an external scheduler or platform UI support for recurrence if available.
- Preserve the same janitor loop contract in the prompt.
- Require branch/PR review before merging any changes.

## Windsurf

Use a Windsurf workflow for the reusable janitor procedure.

- Store the workflow as `/janitor` instructions.
- Windsurf workflows are manually invoked; use an external scheduler or platform support for recurrence if available.
- Keep the workflow body platform-neutral and refer back to the core janitor loop.

## Generic

Use the host's scheduler to run the agent CLI, SDK, or automation entry point.

- Linux/macOS: cron, launchd, or systemd timers.
- Windows: Task Scheduler.
- CI/orchestrators: scheduled pipeline, queue worker, or durable job runner.
- Always configure a non-overlap lock when available.
- Always rely on the janitor loop's target-scoped state check before starting new audit work.

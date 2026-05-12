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
- If the requested cadence is unsupported, choose the nearest safe option and say what changed.
- Every adapter must preserve target-scoped non-overlap: a wake checks existing branches, issues, PRs, or state before starting fresh audit work.

## `--platform auto`

Detect the current host when possible and choose the closest supported behavior:

1. Codex app: prefer a thread heartbeat for short intervals such as `5min`.
2. Codex app with hourly/weekly cadence: use a cron/worktree automation when the requested schedule fits detached work.
3. Claude Code: use the closest native scheduling mode. Prefer Desktop scheduled tasks when local files are required and short intervals matter; use cloud routines when the work should survive without the user's machine.
4. GitHub Actions: create or recommend a scheduled workflow only when repository-hosted CI automation is acceptable.
5. Cursor or Windsurf: install the reusable command/workflow instructions, then require that recurrence be driven by platform UI support or an external scheduler if no native recurring scheduler is available.
6. Generic: provide a scheduler-ready prompt and leave recurrence to cron, launchd, systemd timers, Windows Task Scheduler, or the host orchestrator.

## Codex

Use the Codex app automation API.

- For `5min` and other short intervals, create a heartbeat automation attached to the current thread. This preserves continuity and lets each wake continue pending work.
- For hourly, daily, or weekly detached maintenance, use a cron automation, preferably in a worktree execution environment when available.
- Start one run immediately after creating the schedule so the user can confirm it works.
- The automation prompt must include the full janitor loop contract, target, topics, verification expectations, and concise reporting requirements.

## Claude Code

Claude Code has multiple scheduling modes. Choose based on the requested behavior:

- **Desktop scheduled tasks**: best when `/janitor` needs local files, local tools, MCP config files, and short intervals. Minimum interval is 1 minute.
- **Cloud routines via `/schedule`**: best when work should continue without the user's machine. Routines run on Anthropic-managed infrastructure, clone selected repositories for each run, and have a minimum interval of 1 hour.
- **`/loop`**: useful for quick polling inside a current session. It is session-scoped and not the right default for non-office-hour unattended work.
- **External scheduler plus Claude Code CLI**: fallback when first-party scheduling is unavailable or when the user explicitly wants local OS control.

When using cloud routines:

1. Resolve target, topics, and interval from the core janitor Inputs section.
2. Compose a self-contained janitor prompt. Routines run as fresh sessions, so the prompt must include the target, topics or default review rubric, janitor loop, audit-agent contract, main-agent contract, state/non-overlap rule, safety boundaries, verification expectations, and `/gh` delivery rule.
3. Use `/schedule` to create the routine. If the requested cadence is below the cloud routine minimum, use Desktop scheduled tasks or ask before relaxing the cadence.
4. Trigger a first run immediately when the platform allows it.
5. Report the cadence in human terms and include the routines management URL if available.

Non-overlap for cloud routines must be reconstructed from target state because each run is a fresh session. The janitor loop must inspect open branches, open issues, draft PRs, and any durable state before starting fresh audit work.

When using Desktop scheduled tasks or an external scheduler, keep local scheduler mechanics inside the adapter. Use a scheduler-level lock when available, but still rely on the janitor loop's target-scoped state check.

Claude Code hooks are useful for lifecycle behavior inside a tick, but hooks are not the source of recurrence.

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

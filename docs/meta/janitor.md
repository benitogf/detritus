---
description: Create a recurring proactive code-maintenance worker that audits, safely fixes, verifies, self-reviews, and routes delivery through /gh.
category: meta
triggers:
  - janitor
  - code janitor
  - maintenance worker
  - scheduled audit
  - recurring audit
  - background maintenance
  - proactive cleanup
  - idle quota
when: User invokes /janitor to schedule recurring non-feature codebase maintenance for a project or workspace.
related:
  - meta/gh
  - meta/gh-self-review
  - meta/truthseeker
  - testing/index
---

# /janitor - Recurring Codebase Maintenance

Create a recurring scheduled automation that uses otherwise idle agent quota to improve a codebase without changing product features or intended behavior.

`/janitor` is the single source of truth for the maintenance loop. Platform-specific behavior is limited to choosing the best available scheduler and launch mode.

## Inputs

Treat the user's arguments as hints, not as a required positional form. Like `/gh`, `/janitor` should inspect the conversation, current workspace, platform, and any named project before asking for more input.

Accepted inputs include:

- Nothing at all: use the current workspace, default review rubric, `5min`, and `--platform auto`.
- A project, repo, path, or workspace name.
- Free-text topic focus, such as "flaky tests", "dead code", or "auth and sessions".
- A schedule or interval, such as `5min`, `overnight`, `hourly`, or `weeknights`.
- A platform hint, such as `--platform codex`.

Structured form is supported as a convenience, not as the primary interface:

```
/janitor [project-or-workspace] [topic(s)] [interval] [--platform auto|codex|claude-code|github-actions|cursor|windsurf|generic]
```

Ask only when ambiguity would create the wrong automation, touch the wrong project, or choose a materially different scheduler.

### Target

Default: the whole current workspace.

Accept a repository path, workspace name, project name, or a short free-text target if the platform can resolve it. If the target is ambiguous, ask one concise question before scheduling.

### Topics

Default: use the `/gh-self-review` rubric as the audit lens.

That means correctness, fragility, tests, security, scope discipline, conventions, evidence quality, and no-padding triage. This keeps the review criteria centralized instead of duplicating a topic list here.

Topic arguments narrow the audit. They do not authorize feature work.

Examples of valid topic hints include flaky tests, dead code, duplicate setup, auth/session security, input validation, error handling, logging, retries, API contracts, naming, module boundaries, and maintainability.

### Interval

Default: `5min`.

The interval means "wake and keep the janitor loop alive." It does not mean "start a separate new audit every interval."

## Platform Scheduling

Keep this section as the only platform-specific part of the command.

### `--platform auto`

Detect the current host when possible and choose the closest supported behavior:

1. Codex app: prefer a thread heartbeat for short intervals such as `5min`.
2. Codex app with hourly/weekly cadence: use a cron/worktree automation when the requested schedule fits detached work.
3. Claude Code CLI: create instructions for an external scheduler to run `claude -p` or `claude -c -p` with the janitor prompt.
4. GitHub Actions: create or recommend a scheduled workflow only when repository-hosted CI automation is acceptable.
5. Cursor or Windsurf: install the reusable command/workflow instructions, then require that recurrence be driven by the platform UI or an external scheduler if no native recurring scheduler is available.
6. Generic: provide a scheduler-ready prompt and leave recurrence to cron, launchd, systemd timers, Windows Task Scheduler, or the host orchestrator.

If the requested interval is unsupported on the detected platform, choose the nearest safe scheduler and say what changed.

### Codex

Use the app automation API.

- For `5min` and other short intervals: create a heartbeat automation attached to the current thread. This preserves continuity and lets each wake continue pending work.
- For hourly, daily, or weekly detached maintenance: use a cron automation, preferably in a worktree execution environment when available.
- Start one run immediately after creating the schedule so the user can confirm it works.
- The automation prompt must include the full janitor loop contract, target, topics, verification expectations, and concise reporting requirements.

### Claude Code

Use Claude Code's headless CLI from an external scheduler.

- Use `claude -p "<janitor prompt>"` for independent ticks.
- Use `claude -c -p "<janitor prompt>"` when continuing the most recent conversation is desired.
- Use `claude -p --maintenance "<janitor prompt>"` when project Setup hooks should run in maintenance mode first.
- If session continuity matters, name or persist the session externally and resume it explicitly.
- Use OS scheduling for recurrence. Prefer a lock file or scheduler-level non-overlap control.

Claude Code hooks are useful for lifecycle checks and async follow-up after edits, but hooks are not the source of the recurring schedule.

### GitHub Actions

Use only when repository-hosted automation is acceptable and the work can run unattended in CI.

- Scheduled workflows use cron syntax and can run as frequently as every 5 minutes.
- Add `workflow_dispatch` so developers can start a tick manually.
- Use one concurrency group per target branch or workspace and set `cancel-in-progress: false` to avoid overlapping janitor runs.
- Prefer opening issues or PRs over pushing directly to protected branches.
- Treat secrets and permissions narrowly. Grant only the permissions required to create branches, issues, or PRs.

GitHub Actions is best for remote, auditable janitor ticks. It is a poor fit for interactive local context, local-only files, or IDE-specific state.

### Cursor

Use Cursor background agents for long asynchronous work when available.

- Start a background agent with the janitor prompt and target.
- Use an external scheduler or platform UI support for recurrence if available.
- Preserve the same janitor loop contract in the prompt; do not duplicate platform-specific logic outside this section.
- Require branch/PR review before merging any changes.

### Windsurf

Use a Windsurf workflow for the reusable janitor procedure.

- Store the workflow as `/janitor` instructions.
- Windsurf workflows are manually invoked; use an external scheduler or platform support for recurrence if available.
- Keep the workflow body platform-neutral and refer back to this janitor loop.

### Generic

Use the host's scheduler to run the agent CLI, SDK, or automation entry point.

- Linux/macOS: cron, launchd, or systemd timers.
- Windows: Task Scheduler.
- CI/orchestrators: scheduled pipeline, queue worker, or durable job runner.
- Always configure a non-overlap lock and a durable workspace state file.

## Janitor Loop

Each scheduled wake must follow this order:

1. Resolve the target workspace and load local project instructions.
2. Check whether previous janitor work for the same target is still running or unfinished.
3. If work is pending, continue or integrate that work.
4. If no work is pending, run a fresh audit tick with a separate audit agent using the requested topics, or the `/gh-self-review` rubric when no topics were provided.
5. Critically review the audit findings. Drop vague, risky, feature-changing, or unsupported findings.
6. Implement only small, safe, reviewable improvements that preserve intended behavior.
7. Run the canonical full test command after any code change. Do not limit verification to touched packages unless the project has no full test command.
8. Remove disposable generated test artifacts after test runs.
9. Run self-review through `/gh` before delivery.
10. Route issue, branch, commit, push, and PR work through `/gh`.
11. Report concisely: target, topics, action taken, tests run, PR/issue link if created, and any blocker.

Do not pollute the user's main thread with raw audit logs.

## Audit Agent Contract

Spawn a separate review/audit agent for every fresh audit tick.

The audit agent must return only concrete, actionable, isolated findings that can become a TODO list for the main agent.

Each finding must include:

- Title.
- Evidence with file path and line number when applicable.
- Why it is safe maintenance rather than feature work.
- Suggested smallest safe fix.
- Verification that would prove the fix.

The audit agent must not:

- Edit files.
- Stage, commit, push, or open PRs.
- Return broad themes without evidence.
- Suggest behavior changes, product changes, or speculative rewrites.
- Dump long logs.

## Main Agent Contract

The main agent owns judgment and delivery.

- Treat audit findings as untrusted input.
- Prove a finding before changing code.
- Prefer the smallest safe improvement.
- Preserve public APIs and product behavior unless the finding is clearly about a bug in validation, security, reliability, or test isolation.
- Avoid drive-by refactors.
- Avoid mixing unrelated findings in one PR.
- If the project has a canonical full test command, run it after changes.
- If tests fail because of pre-existing breakage, capture evidence, avoid masking it, and report the blocker.
- Use `/gh` for GitHub delivery so changes to the GitHub flow remain centralized.

## State And Non-Overlap

Each platform adapter must provide equivalent behavior:

- One active janitor run per target.
- A durable state record containing target, topics, active branch/PR/issue if any, last tick time, and current phase.
- A wake that finds active work continues it instead of spawning another audit.
- A stale or crashed run may be reclaimed only after inspecting the workspace and reporting why it is safe.

## Safety Boundaries

Allowed:

- Test cleanup, deduplication, isolation fixes, and flake reduction.
- Removing provably unused code or files.
- Simplifying duplicate internal logic without behavior change.
- Improving validation, error handling, logging, and retry safety.
- Small naming, structure, and maintainability fixes.
- Documentation updates that explain existing behavior.

Not allowed:

- Product feature changes.
- API behavior changes unless fixing a clearly proven bug.
- Broad rewrites.
- Dependency upgrades unless the topic explicitly asks for them and tests prove safety.
- Secret exposure in logs, reports, commits, or PR bodies.
- Pushing directly to protected or shared branches.
- Opening a PR without passing the applicable `/gh` confirmation gates.

## Initial Run

After creating the schedule, immediately start one audit tick so the user can confirm the automation works.

The first report should be short:

```
Janitor scheduled: <target> every <interval> via <platform mode>.
First tick: <continued pending work | audited topics | opened PR | no safe change | blocked>.
Verification: <test command and result, if code changed>.
Next wake: <time or schedule summary>.
```

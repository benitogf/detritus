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

Default: `5min` on heartbeat-style platforms (Codex thread automations); `30min` on platforms where each tick is a full fresh-agent run (Claude Code routines, GitHub Actions, generic cron). The interval means "wake and keep the janitor loop alive." It does not mean "start a separate new audit every interval."

Always speak to the user in human terms (`every 30 minutes`, `overnight`, `weeknights`). Translate to whatever the scheduler underneath requires — cron strings, automation cadence enums, etc. — without showing them to the user unless asked.

## Platform Scheduling

Keep this section as the only platform-specific part of the command.

### `--platform auto`

Detect the current host when possible and choose the closest supported behavior:

1. Codex app: prefer a thread heartbeat for short intervals such as `5min`.
2. Codex app with hourly/weekly cadence: use a cron/worktree automation when the requested schedule fits detached work.
3. Claude Code: use the first-party `/schedule` routine primitive. Never expose cron, the headless CLI, or session IDs to the user.
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

Claude Code ships a first-party recurring agent primitive — **`/schedule`** (cloud routines). Use it. The user must never see cron strings, lock files, the headless CLI, `--permission-mode` flags, or session IDs. Scheduling is the agent's job; the user's job is to type `/janitor`.

When `/janitor` is invoked inside Claude Code:

1. Resolve target, topics, and interval per the Inputs section.
2. Compose a **self-contained janitor prompt**. Routines accept only free text — they cannot invoke other slash commands. The prompt must inline everything the routine needs to act without further context:
   - The target (absolute repo path or workspace identifier — not "the current workspace").
   - The topic list, or "use the /gh-self-review rubric as the audit lens" if none.
   - The full Janitor Loop steps from this doc, copied verbatim.
   - The Audit Agent Contract, Main Agent Contract, and Safety Boundaries sections, copied verbatim.
   - An explicit instruction to route delivery through `/gh` (which the routine's agent will have available because it's a fresh Claude Code session, even though `/schedule` itself doesn't expand slash commands).
3. Invoke `/schedule` with that prompt and a cron expression derived internally from the human interval (`every 30 minutes` → `*/30 * * * *`, `overnight` → e.g. `0 2 * * *` in the user's stated timezone, `weeknights` → `0 22 * * 1-5`). Do not show the cron string in the user-facing report.
4. Trigger one immediate "Run now" on the routine so the user sees a first tick complete in the same conversation flow.
5. Report in human terms: target, topics, cadence as a sentence (not a cron), first-tick outcome, and the routines management URL (`https://claude.ai/code/routines`) so the user can pause / edit / delete later via UI.

Non-overlap on Claude Code routines is **not** automatic — the platform may fire overlapping ticks if a previous tick is still running. This is handled by the Janitor Loop itself, not by the scheduler: step 2 of the loop ("Check whether previous janitor work for the same target is still running or unfinished") inspects open branches, draft PRs, and open issues on the target before doing anything else. That is why the loop's State And Non-Overlap contract is target-scoped (branches/PRs/issues) rather than session-scoped — it survives across fresh routine runs that share no in-memory state.

Cross-tick continuation works the same way: each routine fire is a fresh agent with no memory of prior ticks. Continuation comes from reading the GitHub state for the target, not from resuming a session.

Claude Code hooks (Setup `maintenance` matcher, Stop hooks) remain useful for in-tick lifecycle but are not the source of recurrence.

#### Fallback when `/schedule` is unavailable

If the user's plan or build does not expose `/schedule`, the agent — not the user — should set up the equivalent locally. Generate a `flock`-guarded entry that invokes `claude -p --maintenance --permission-mode bypassPermissions --session-id <stable-id> --output-format json "<janitor prompt>"` on the user's OS scheduler (cron, launchd, or Task Scheduler), wire it for them when permitted, and then report in human terms exactly as the `/schedule` path would. Cron syntax, lock files, and CLI flags stay internal unless the user asks how it works.

### GitHub Actions

Use only when repository-hosted automation is acceptable and the work can run unattended in CI.

- Scheduled workflows use cron syntax and can be set as frequently as every 5 minutes, but GitHub Actions cron is best-effort and runs may be delayed or skipped under runner contention. Treat the interval as a floor, not a guarantee.
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
7. Run the project's canonical verification command after any code change. If the full suite is slower than the wake interval, the change is still delivered only on a tick where verification completes green — partial-tick verification does not count.
8. Run `/gh-self-review` on the resulting diff before delivery.
9. Route issue, branch, commit, push, and PR work through `/gh`.
10. Report concisely: target, topics, action taken, tests run, PR/issue link if created, and any blocker.

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
- Run the project's canonical verification command after changes; don't ship on a tick that didn't finish it green.
- If tests fail because of pre-existing breakage, capture evidence, avoid masking it, and report the blocker.
- Use `/gh` for GitHub delivery so changes to the GitHub flow remain centralized.

## State And Non-Overlap

Each platform adapter must provide equivalent behavior:

- One active janitor run per target.
- Target-scoped state: open branches, draft PRs, and open issues on the target are the source of truth for "what work is in flight." Loop step 2 reads them at every wake. Adapters that have a scheduler-managed lock (Codex thread, GitHub Actions concurrency group, generic `flock`) should still rely on this target-scoped check, not on the lock alone — fresh-agent platforms like Claude Code routines share no in-memory state across ticks and must reconstruct it from GitHub each time.
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

The first report speaks in human terms. No cron strings, no flag names, no session IDs, no platform jargon:

```
Janitor scheduled on <target>, <human cadence — e.g. "every 30 minutes" or "weeknights at 10pm">.
First tick: <continued pending work | audited topics | opened PR | no safe change | blocked>.
Verification: <test command and result, if code changed>.
Manage anytime at <management URL if the platform exposes one>.
```

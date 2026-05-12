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
  - meta/janitor-platforms
  - meta/truthseeker
  - testing/index
---

# /janitor - Recurring Codebase Maintenance

Create a recurring scheduled automation that uses otherwise idle agent quota to improve a codebase without changing product features or intended behavior.

`/janitor` is the single source of truth for the maintenance loop. Platform-specific behavior lives in `meta/janitor-platforms`; do not duplicate scheduler instructions in this core workflow.

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

Always speak to the user in human terms (`every 30 minutes`, `overnight`, `weeknights`). Translate to whatever the scheduler underneath requires — cron strings, automation cadence enums, etc. — without showing them to the user unless asked.

## Platform Adapter

After resolving target, topics, and interval, load `meta/janitor-platforms` and choose the adapter for the detected or requested platform. The adapter decides how to schedule the loop and how to translate the user's human cadence into the platform's scheduler.

The adapter must return:

- The scheduler mode being used.
- The effective human cadence.
- Whether the first tick can run immediately.
- Any platform limitation that materially changes the requested behavior.
- The management URL or command, if the platform exposes one.

Keep all platform-specific APIs, cron syntax, CLI flags, and management URLs in `meta/janitor-platforms`.

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

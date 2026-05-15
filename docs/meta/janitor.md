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

If the chosen platform adapter has a minimum interval higher than the requested cadence, the adapter rounds up to that minimum without asking and reports the effective cadence in the initial run report. Do not block the user with a clarifying question for the common case of a small-default-vs-large-minimum mismatch.

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
2. Read the scratchpad (`.janitor/<slug>.md` — see *Durable Cross-Tick State* below). Honor the current orientation. Verify the State block's next-tick plan still matches reality before proceeding.
3. Check whether previous janitor work for the same target is still running or unfinished.
4. If work is pending, continue or integrate that work.
5. If no work is pending, run a fresh audit tick with a separate audit agent using the requested topics, or the `/gh-self-review` rubric when no topics were provided.
6. Critically review the audit findings. Drop vague, risky, feature-changing, or unsupported findings.
7. Implement only small, safe, reviewable improvements that preserve intended behavior.
8. Run the project's canonical verification command after any code change. If the full suite is slower than the wake interval, the change is still delivered only on a tick where verification completes green — partial-tick verification does not count.
9. Run `/gh-self-review` on the resulting diff before delivery.
10. Route issue, branch, commit, push, and PR work through `/gh`.
11. Report concisely AND append a dated entry to the scratchpad's tick log; overwrite the State block at the bottom with the new live truth (in-flight work, metric value if any, hazards added, next-tick plan).

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

The audit agent must:

- Return findings as a per-tick report file at `.janitor/<slug>-tick-N.md` in the target root, not as inline output. The main agent reads this file, folds the summary into the scratchpad, and deletes the per-tick file once integrated.
- Describe conditions, not root-cause hypotheses. Stating a suspected cause up front biases what the audit verifies; let the audit reproduce the condition and report what it actually found.

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
- **Audit-to-verify cadence.** When the canonical verification command is slower than roughly twice the wake interval, run two audit-and-implement ticks per one verification tick. Prevents piling up unverified changes when verify is expensive.
- **Truthseeker pause on user critique.** When the user critiques the loop mid-flow, do not redirect on first read. Re-read the critique against the current orientation and the loop's evidence before changing course. Misreading "justify X" as "delete X" is the canonical failure mode.
- **Honest regression reporting.** If a tick's change regresses a metric the loop has been tracking, surface the regression in that same tick's output. Do not bury it in cumulative averages or wait for the next audit to notice.

## Primary Metric

If the `/janitor` invocation names a metric the loop should move — test wall time, flake count, dead-code line count, coverage percentage, lint warnings, anything measurable — the loop tracks it and reports its value at every tick.

- If the invocation also states a goal value (`wall ≤ 145s`, `flake count to 0`, `no lint warnings`), the loop reports progress toward the goal each tick. Reaching the goal can be a loop-end criterion if the user opted into those.
- If no goal is stated, the loop reports the per-tick delta only. No fixed end.
- If no metric is stated at all, the metric-tracking flow is inactive. The loop still does safe maintenance.

The honest-regression rule in the Main Agent Contract applies regardless: any movement of a tracked metric in the wrong direction must surface in that tick's output.

A metric is read from the invocation in plain language. No mode selection, no setup-time dropdown. "Drive flake count to 0" sets a goal; "keep wall time under 200s while improving coverage" sets two metrics with a floor; "track lint warnings" sets a metric with no goal. The agent reads what the user wrote.

## State And Non-Overlap

Each platform adapter must provide equivalent behavior:

- One active janitor run per target.
- Target-scoped state: open branches, draft PRs, and open issues on the target are the source of truth for "what work is in flight." Loop step 3 reads them at every wake. Adapters that have a scheduler-managed lock (Codex thread, GitHub Actions concurrency group, generic `flock`) should still rely on this target-scoped check, not on the lock alone — fresh-agent platforms like Claude Code routines share no in-memory state across ticks and must reconstruct it from GitHub each time.
- A wake that finds active work continues it instead of spawning another audit.
- A stale or crashed run may be reclaimed only after inspecting the workspace and reporting why it is safe.

## Durable Cross-Tick State

GitHub-side state (open branches, draft PRs, open issues) answers "what work is in flight." It does not answer "what is the rolling plan, what have we learned, what's been ranked but deferred." Fresh-session schedulers — Cloud Routines especially — restart cold every tick and lose any in-memory context from prior runs.

To carry plan-state across ticks, every janitor maintains a gitignored scratchpad file at `.janitor/<slug>.md` relative to the invocation root.

### Slug rule

The slug is derived from where `/janitor` was invoked and what it was asked to do:

| Invocation | Slug |
|---|---|
| Single repo, no topic | `whole-repo` (or `<repo-name>` if disambiguation is needed) |
| Multi-folder workspace root, no topic | `whole-workspace` |
| Topic-focused (any scope) | `<topic-slug>` — short kebab-case derived from the topic, e.g. `flaky-tests`, `dead-code-sweep`, `perf-walltime` |

Multiple parallel janitors in one workspace use distinct slugs sharing the same `.janitor/` directory. There is no cross-instance coordination beyond the GitHub-side non-overlap check.

If `.janitor/` is not already covered by the target's gitignore, the agent adds it at setup.

### Scratchpad sections

Required, in order, top-to-bottom:

- **North-star goal** — one paragraph: what the loop is trying to achieve.
- **User-stated rules** — verbatim where the user provided them. Quoted, not paraphrased.
- **Current orientation** — one or two lines describing the active focus. Updated when the user redirects mid-loop (the truthseeker-pause rule applies before this section is rewritten).
- **Primary metric** (if any) — name, baseline, goal if stated.
- **Loop-end criteria** (if any) — named exit conditions.
- **Tick log** — append-only, dated entries. One section per tick.
- **State block** (last section, overwritten each tick) — see below.

### State block

The State block is the only mutable section. Every tick overwrites it. Required fields:

- In-flight sub-agents, branches, or PRs the next tick must continue.
- Current metric value (if tracked) and delta since baseline.
- Loop-end criteria progress (if criteria exist).
- Skip-streak counter — see *Loop-End Criteria* below.
- **Hazards / Deferred** — ranked list of issues the loop surfaced but couldn't safely fix in isolation. Each entry: title; evidence (file:line if applicable); why it's outside safe-maintenance scope; suggested grouping for a separate user-led fix.
- Next-tick plan — concrete first move for the next wake.
- Last user directive — most recent pivot or scope change, dated.

### Per-tick report files

Sub-agents return findings as a per-tick file at `.janitor/<slug>-tick-N.md` in the target root, not inline. This keeps the main thread free of raw audit output while preserving the detail until fold-in. The main agent reads the file, folds the summary into the scratchpad's tick log and State block, then deletes the per-tick file. Keep the file only if the user explicitly asks.

### Durability

The scratchpad pattern assumes `.janitor/<slug>.md` persists on disk between ticks. On runners that meet that assumption — Desktop Routines, an external scheduler running CLI tools, Codex `local` execution, GitHub Actions with workspace caching — the scratchpad carries plan-state across cold starts as designed.

On **disposable runners** that discard the workspace between ticks — Codex `worktree` execution, Claude Code Cloud Routines (each tick clones the repository fresh), stateless GitHub Actions runners — the gitignored scratchpad does not survive. Each platform adapter must declare how it handles this in one of three modes:

1. **Use a durable execution mode** for that platform when one exists (e.g. Codex `local` instead of `worktree`; Claude Code Desktop Routines or the external scheduler fallback instead of Cloud).
2. **Operate GitHub-state-only.** The loop reconstructs everything it needs from open branches, draft PRs, and open issues every tick. No cross-tick scratchpad; the State block lives in the issue or PR bodies the loop maintains, hazards become open issues, the tick log becomes commit history. Acceptable for narrow janitors that don't need rolling metric history or per-tick narrative.
3. **Report the incompatibility at setup.** If neither option fits, the adapter reports that the requested scheduling mode cannot persist the scratchpad and asks the user to choose between a durable mode, stateless mode, or a different platform.

Per-tick report files inherit the same rule. On disposable runners they must be folded into the scratchpad — or into GitHub-side state under mode 2 — synchronously within the same tick, before the wake ends.

### Pruning

The tick log is append-only across the session, but older entries should be summarized into a single "cumulative since tick N" block when the user signals a phase or cluster is closed, OR the log exceeds roughly thirty entries. The thirty is soft guidance — the goal is to keep the scratchpad readable across context resets, not to grow unbounded.

## Loop-End Criteria

Loop-end criteria are encouraged but not required. Some loops are meant to run perpetually as living maintenance (self-audit, ongoing flake suppression, drift detection). For those, set no end criteria; the loop continues until the user stops it.

When end criteria are set at setup, the State block reports progress against them every tick. When all are met, the loop reports completion and asks: stop / raise the bar / pivot.

### Skip-streak guardrail

Whether or not end criteria are set, the loop protects against silent drift via a skip-streak counter:

- A tick counts as a *skip* when the audit returned no actionable findings AND no in-flight work was continued AND the next-tick plan is empty.
- A tick that does real work — commit, PR opened, hazard added, metric moved, in-flight work continued — resets the counter to zero.
- When the streak reaches the threshold — default **eight ticks OR two hours of skip-only wakes, whichever comes first** — the loop pauses, summarizes what was reviewed during the streak, and asks the user: keep going / pivot orientation / stop.

Threshold is configurable per janitor at setup. The "or two hours" floor protects against slow-cadence janitors (daily/weekly cron) where eight ticks would be too long to wait for a drift signal.

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

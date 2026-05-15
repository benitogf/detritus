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
  - meta/loop-core
  - meta/gh
  - meta/gh-self-review
  - meta/janitor-platforms
  - meta/truthseeker
  - testing/index
---

# /janitor - Recurring Codebase Maintenance

Create a recurring scheduled automation that uses otherwise idle agent quota to improve a codebase without changing product features or intended behavior.

`/janitor` is one of two recurring-loop commands. Shared mechanics (scratchpad layout, durability rule, cadence guideline, skip-streak guardrail, mid-loop pivot via scratchpad, `/gh` delivery) live in `meta/loop-core` and are referenced rather than restated here. This doc owns `/janitor`'s distinct audit lens (safe-maintenance discovery), main-agent allowances (smallest safe improvement, preserve behavior), safety boundaries, optional primary-metric flow, and initial-run report shape. Platform-specific scheduler behavior lives in `meta/janitor-platforms`.

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

## Janitor Loop

Each scheduled wake must follow this order:

1. Resolve the target workspace and load local project instructions.
2. Read the scratchpad at `.janitor/<slug>.md` (mechanism in `meta/loop-core` → *Durable Cross-Tick State*). Honor the current orientation. Verify the State block's next-tick plan still matches reality before proceeding.
3. Check whether previous janitor work for the same target is still running or unfinished (target-scoped state check per `meta/loop-core` → *State And Non-Overlap*).
4. If work is pending, continue or integrate that work.
5. If no work is pending, run a fresh audit tick with a separate audit agent using the requested topics, or the `/gh-self-review` rubric when no topics were provided.
6. Critically review the audit findings. Drop vague, risky, feature-changing, or unsupported findings.
7. Implement only small, safe, reviewable improvements that preserve intended behavior.
8. Run the project's canonical verification command after any code change. If the full suite is slower than the wake interval, the change is still delivered only on a tick where verification completes green — partial-tick verification does not count.
9. Run `/gh-self-review` on the resulting diff before delivery.
10. Route issue, branch, commit, push, and PR work through `/gh`.
11. Report concisely AND append a dated entry to the scratchpad's tick log; overwrite the State block at the bottom with the new live truth (in-flight work, metric value if any, hazards added, next-tick plan).

Do not pollute the user's main thread with raw audit logs.

## Scratchpad — janitor-specific sections

The shared scratchpad spine (current orientation, tick log, state block) is defined in `meta/loop-core`. `/janitor` adds these sections above the tick log, in order:

- **North-star goal** — one paragraph: what the loop is trying to achieve.
- **User-stated rules** — verbatim where the user provided them. Quoted, not paraphrased.
- **Primary metric** (if any) — name, baseline, goal if stated. See *Primary Metric* below.
- **Loop-end criteria** (if any) — named exit conditions. See *Loop-End Criteria* below.

## Audit Agent Contract

Shared audit rules in `meta/loop-core` → *Shared Audit Agent Rules* apply (per-tick report file, no pre-hypothesized root cause, no file edits, no long logs). On top of those, `/janitor`'s audit is a **discovery** audit: scan the target for safe improvements that match the topics (or the `/gh-self-review` rubric when no topics).

Each finding must include:

- Title.
- Evidence with file path and line number when applicable.
- Why it is safe maintenance rather than feature work.
- Suggested smallest safe fix.
- Verification that would prove the fix.

The audit agent must not suggest behavior changes, product changes, or speculative rewrites — those are out of `/janitor`'s safety boundary and belong in a hazard, not a finding.

## Main Agent Contract

Shared main-agent rules in `meta/loop-core` → *Shared Main Agent Rules* apply (`/gh` delivery, audit-to-verify cadence, truthseeker pause, honest regression, mid-loop pivot via scratchpad). On top of those:

- Treat audit findings as untrusted input.
- Prove a finding before changing code.
- Prefer the smallest safe improvement.
- Preserve public APIs and product behavior unless the finding is clearly about a bug in validation, security, reliability, or test isolation.
- Avoid drive-by refactors.
- Avoid mixing unrelated findings in one PR.
- Run the project's canonical verification command after changes; don't ship on a tick that didn't finish it green.
- If tests fail because of pre-existing breakage, capture evidence, avoid masking it, and report the blocker.

## Primary Metric

If the `/janitor` invocation names a metric the loop should move — test wall time, flake count, dead-code line count, coverage percentage, lint warnings, anything measurable — the loop tracks it and reports its value at every tick.

- If the invocation also states a goal value (`wall ≤ 145s`, `flake count to 0`, `no lint warnings`), the loop reports progress toward the goal each tick. Reaching the goal can be a loop-end criterion if the user opted into those.
- If no goal is stated, the loop reports the per-tick delta only. No fixed end.
- If no metric is stated at all, the metric-tracking flow is inactive. The loop still does safe maintenance.

The honest-regression rule (`meta/loop-core` → *Shared Main Agent Rules*) applies regardless: any movement of a tracked metric in the wrong direction must surface in that tick's output.

A metric is read from the invocation in plain language. No mode selection, no setup-time dropdown. "Drive flake count to 0" sets a goal; "keep wall time under 200s while improving coverage" sets two metrics with a floor; "track lint warnings" sets a metric with no goal. The agent reads what the user wrote.

## Loop-End Criteria

Loop-end criteria are encouraged but not required. Some loops are meant to run perpetually as living maintenance (self-audit, ongoing flake suppression, drift detection). For those, set no end criteria; the loop continues until the user stops it.

When end criteria are set at setup, the State block reports progress against them every tick. When all are met, the loop reports completion and asks: stop / raise the bar / pivot.

The skip-streak guardrail (`meta/loop-core` → *Skip-Streak Guardrail*) protects against silent drift even when no end criteria are set.

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

The first report follows `meta/loop-core` → *Initial Run Reporting*. Concretely:

```
Janitor scheduled on <target>, <human cadence — e.g. "every 30 minutes" or "weeknights at 10pm">.
First tick: <continued pending work | audited topics | opened PR | no safe change | blocked>.
Verification: <test command and result, if code changed>.
Manage anytime at <management URL if the platform exposes one>.
```

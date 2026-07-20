---
description: Shared fundamentals for recurring loop commands like /janitor and /smith — scratchpad, durability, cadence, skip-streak, mid-loop pivot, /gh delivery. Do not invoke directly.
triggers:
  - loop-core
when: Loaded by /janitor and /smith (and any future recurring-loop command) to define the shared scratchpad, durability rule, cadence guidelines, skip-streak guardrail, and delivery routing. Not a standalone workflow — invoke one of the wrapping commands instead.
related:
  - core/completion
  - flows/build/janitor
  - flows/build/smith
  - core/janitor-platforms
  - flows/github/gh
  - flows/github/gh-self-review
  - flows/principles/truthseeker
---

# Loop Core — Shared Fundamentals for Recurring Loops

This file is the single source for the mechanics shared by every recurring-loop command in the detritus knowledge base. `/janitor` and `/smith` each reference it for the shared parts and own only their differentiating sections (audit/main contracts, safety boundaries, command-specific scratchpad content, command-specific loop steps).

Throughout, `<loop>` is a placeholder for the consuming command's directory name (`.janitor/` for /janitor, `.smith/` for /smith). `<slug>` is the runtime-derived scratchpad slug (see *Slug rule*).

## Platform Adapter

After resolving target, topics or spec, and interval, load `core/janitor-platforms` and choose the adapter for the detected or requested platform. The adapter decides how to schedule the loop and how to translate the user's human cadence into the platform's scheduler.

The adapter must return:

- The scheduler mode being used.
- The effective human cadence.
- Whether the first tick can run immediately.
- Any platform limitation that materially changes the requested behavior.
- The management URL or command, if the platform exposes one.

Keep all platform-specific APIs, cron syntax, CLI flags, and management URLs in `core/janitor-platforms`. Both `/janitor` and `/smith` use the same adapter doc; the adapter doesn't need to know which loop is consuming it.

## State And Non-Overlap

Each platform adapter must provide equivalent behavior:

- One active loop run per target.
- Target-scoped state: open branches, draft PRs, and open issues on the target are the source of truth for "what work is in flight." The loop reads them at every wake. Adapters that have a scheduler-managed lock (Codex thread, GitHub Actions concurrency group, generic `flock`) should still rely on this target-scoped check, not on the lock alone — fresh-agent platforms like Claude Code routines share no in-memory state across ticks and must reconstruct it from GitHub each time.
- A wake that finds active work continues it instead of spawning another audit.
- A stale or crashed run may be reclaimed only after inspecting the workspace and reporting why it is safe.

## Durable Cross-Tick State

GitHub-side state (open branches, draft PRs, open issues) answers "what work is in flight." It does not answer "what is the rolling plan, what have we learned, what's been ranked but deferred." Fresh-session schedulers — Cloud Routines especially — restart cold every tick and lose any in-memory context from prior runs.

To carry plan-state across ticks, every loop maintains a gitignored scratchpad file at `<loop>/<slug>.md` relative to the invocation root. Different loop commands use different top-level directories (`.janitor/`, `.smith/`) so each instance's working files stay separable.

### Slug rule

The slug is derived from where the loop was invoked and what it was asked to do:

| Invocation | Slug |
|---|---|
| Single repo, no topic / spec narrowing | `whole-repo` (or `<repo-name>` if disambiguation is needed) |
| Multi-folder workspace root, no topic / spec narrowing | `whole-workspace` |
| Topic-focused or spec-focused | `<topic-slug>` — short kebab-case derived from the topic or feature name, e.g. `flaky-tests`, `dead-code-sweep`, `perf-walltime`, `typed-cancellation` |

Multiple parallel loops in one workspace use distinct slugs sharing the same `<loop>/` directory. There is no cross-instance coordination beyond the GitHub-side non-overlap check.

If `<loop>/` is not already covered by the target's gitignore, the agent adds it at setup.

### Scratchpad sections

Every scratchpad shares a common spine. Each consuming command (`/janitor`, `/smith`) adds command-specific sections above the tick log; the spine is:

- **Current orientation** — one or two lines describing the active focus. Updated when the user redirects mid-loop (the truthseeker-pause rule applies before this section is rewritten).
- **Tick log** — append-only, dated entries. One section per tick.
- **State block** (last section, overwritten each tick) — see below.

Each command lists its specific sections above the tick log in its own doc (e.g. /janitor adds *North-star goal*, *User-stated rules*, *Primary metric*, *Loop-end criteria*; /smith adds *Feature spec*, *User-stated rules*, *Acceptance criteria*).

### State block

The State block is the only mutable section. Every tick overwrites it. Required fields:

- In-flight sub-agents, branches, or PRs the next tick must continue.
- Current metric value (if tracked) and delta since baseline.
- Loop-end criteria progress (if criteria exist).
- Skip-streak counter — see *Skip-streak guardrail* below.
- **Blockers & feature-splits** — only disposition-2 (genuinely separate features) and disposition-3 (hard blockers) per `core/completion`. **Nothing handle-able in-scope is recorded here** — in-scope work is disposition 1 (do it now), never parked. Each entry: title; evidence (file:line if applicable); why it is a genuinely separate feature or a blocker beyond the loop's authority; suggested grouping for a separate fix.
- Next-tick plan — concrete first move for the next wake.
- Last user directive — most recent pivot or scope change, dated.

Commands may add command-specific State block fields (e.g. /smith adds *Acceptance items checked*).

### Per-tick report files

Sub-agents return findings as a per-tick file at `<loop>/<slug>-tick-N.md` in the target root, not inline. This keeps the main thread free of raw audit output while preserving the detail until fold-in. The main agent reads the file, folds the summary into the scratchpad's tick log and State block, then deletes the per-tick file. Keep the file only if the user explicitly asks.

The report is compact and evidence-backed (*Shared Audit Agent Rules*), not a log dump. Default shape — one line each:

- **Finding / status**
- **Evidence** — per the cite-or-it's-noise rule in *Shared Audit Agent Rules*.
- **Affected files**
- **Smallest next delta** — the minimal change to consider next.
- **Verification** — the command to run, and its result if already run.

Consuming commands may sharpen these fields for their phase. Only this durable summary is folded into the scratchpad; the raw file is deleted unless the user asked to keep it.

### Durability

The scratchpad pattern assumes `<loop>/<slug>.md` persists on disk between ticks. On runners that meet that assumption — Desktop Routines, an external scheduler running CLI tools, Codex `local` execution, GitHub Actions with workspace caching — the scratchpad carries plan-state across cold starts as designed.

On **disposable runners** that discard the workspace between ticks — Codex `worktree` execution, Claude Code Cloud Routines (each tick clones the repository fresh), stateless GitHub Actions runners — the gitignored scratchpad does not survive. Each platform adapter must declare how it handles this in one of three modes:

1. **Use a durable execution mode** for that platform when one exists (e.g. Codex `local` instead of `worktree`; Claude Code Desktop Routines or the external scheduler fallback instead of Cloud).
2. **Operate GitHub-state-only.** The loop reconstructs everything it needs from open branches, draft PRs, and open issues every tick. No cross-tick scratchpad; the State block lives in the issue or PR bodies the loop maintains, feature-splits and blockers become open issues — icebox issues (`core/icebox`), filed via the chokepoint — the tick log becomes commit history. Acceptable for narrow loops that don't need rolling metric history or per-tick narrative.
3. **Report the incompatibility at setup.** If neither option fits, the adapter reports that the requested scheduling mode cannot persist the scratchpad and asks the user to choose between a durable mode, stateless mode, or a different platform.

Per-tick report files inherit the same rule. On disposable runners they must be folded into the scratchpad — or into GitHub-side state under mode 2 — synchronously within the same tick, before the wake ends.

### Pruning

The tick log is append-only across the session, but older entries should be summarized into a single "cumulative since tick N" block when the user signals a phase or cluster is closed, OR the log exceeds roughly thirty entries. The thirty is soft guidance — the goal is to keep the scratchpad readable across context resets, not to grow unbounded.

### Checkpoint-then-/clear

The ledger — the scratchpad/progress file, git, and GitHub-side state (see *Durable Cross-Tick State*) — is the **ONLY resume state**. Chat history is never a state carrier. Serialize at every boundary the consuming command names, leaving the State block sufficient for a **fresh agent to resume from the ledger alone**: overwrite the State block and fold any open tick narrative into the log *before* moving on.

- Serialize into the active *Durability* mode's store — the on-disk scratchpad (mode 1) or the issue / PR bodies (mode 2). There is no third store. On disposable runners the boundary collapses to **every wake-end**: serialize before the wake ends, never defer across ticks (this is the same rule *Durability* states for per-tick report files).
- **A user `/clear` after a checkpoint is the supported, lossless reset.** The next tick re-derives every open item — plus any `blocked` nodes, which re-surface until resolved (`core/completion` → *The exit gate*) — from the ledger. Clearing changes nothing about durability (mode 1 already persists to disk; fresh sessions already restart cold); it only trims live context once the resume point is written. Never `/clear` before the State block is current. The truthseeker pause (*Shared Main Agent Rules*) still applies before rewriting orientation.
- **Never rely on `/compact`.** Summarized chat is not a state carrier; the loop must stay correct even if compaction loses everything, because the ledger holds the truth.
- **The tick report prompts the reset.** At heavy boundaries — a completed acceptance item, a failed-verification cluster, a self-review pass, an integration round — the tick report ends with the literal line `checkpoint complete — safe to /clear`, so the user knows exactly when clearing is lossless.

This adds no new persistence mechanism — it is the discipline of using the existing stores as the resume point at known boundaries, so the main thread stays lean between clears.

## Usage-Limit Resilience

A loop must survive the consuming seat hitting its usage / token limit mid-tick. The reset is temporary; losing the schedule is not — so a limit hit must pause the loop, never end it.

The hazard is specific to **self-rescheduling drivers** — those where each tick is responsible for arming the next one at the end of its own turn (Claude Code `/loop` dynamic mode via `ScheduleWakeup` is the canonical example). When a tick exhausts the seat mid-run, the turn is terminated *before* it reaches the re-arm call, so no next wake is queued and the entire loop stops — not just the tick that died. Recovery then requires a human to restart it.

Rules:

- **Prefer a driver whose schedule is standing data, independent of any single tick** — durable cron (`.claude/scheduled_tasks.json`), an OS timer, or first-party Routines. A limit-killed tick cannot delete a standing schedule, so the next fire still occurs on wall-clock; once a fire lands after the seat resets, it succeeds with no manual restart.
- **Do not build resilience on "remaining quota" detection.** No first-party signal reliably exposes remaining seat quota before a tick runs, and any in-band attempt to re-arm after detecting exhaustion runs as a model turn that the limit itself blocks. The reset time is only knowable reactively (from the limit error), and a self-pacing wake is clamped too short (≤1h on `ScheduleWakeup`) to reliably jump a longer reset. The robust "escape hatch" is simply *fire again next interval*, which a standing schedule provides for free.
- **If a self-rescheduling driver is unavoidable, treat it as office-hours-only** and tell the user a limit hit will stop the loop until they restart it. Do not present it as unattended-durable.

The platform adapter (`core/janitor-platforms`) names which drivers on each host are standing-schedule (resilient) versus self-rescheduling (fragile).

## Shared Main Agent Rules

These rules apply to the main agent in every recurring-loop command. Command-specific contracts (`/janitor`, `/smith`) add their own rules on top.

- **Use `/gh` for GitHub delivery** so changes to the GitHub flow remain centralized. Issue creation, branch/commit/push, PR opening, feedback handling, and self-review all route through the `/gh` router and its sub-skills.
- **Audit-to-verify cadence.** When the canonical verification command is slower than roughly twice the wake interval, run two audit-and-implement ticks per one verification tick. Prevents piling up unverified changes when verify is expensive.
- **Truthseeker pause on user critique.** When the user critiques the loop mid-flow, do not redirect on first read. Re-read the critique against the current orientation and the loop's evidence before changing course. Misreading "justify X" as "delete X" is the canonical failure mode.
- **Honest regression reporting.** If a tick's change regresses a metric the loop has been tracking, surface the regression in that same tick's output. Do not bury it in cumulative averages or wait for the next audit to notice. Each consuming command defines what "tracked metric" means for it.
- **Mid-loop pivot via scratchpad.** User redirections — "focus on perf only", "drop the dead-code thread", "this acceptance item changed shape" — propagate by updating the scratchpad's *Current orientation* (and any affected command-specific sections) at the end of the current tick. The next wake honors the new orientation. Live ticks must apply the truthseeker pause before rewriting orientation.

## Shared Audit Agent Rules

These rules apply to every audit sub-agent spawned by a recurring-loop command. Command-specific contracts add their own rules on top.

- Return findings as a per-tick report file at `<loop>/<slug>-tick-N.md`, not as inline output. The main agent reads, folds, and deletes.
- Describe conditions, not root-cause hypotheses. Stating a suspected cause up front biases what the audit verifies; let the audit reproduce the condition and report what it actually found.
- Do not return broad themes without evidence. Every finding cites a file path (and line when applicable), a caller, or a concrete reproduction; "the auth code feels fragile" is noise, not a finding.
- Do not edit files, stage, commit, push, or open PRs.
- Do not dump long logs.
- The main agent is the only loop owner. A sub-agent is a bounded reporter for one wake — it audits, inspects, maps, and reports; it does not make broad scope decisions, drive the loop as a separate owner, or take work the consuming command reserves for the main agent.

## Skip-Streak Guardrail

Whether or not end criteria are set, every loop protects against silent drift via a skip-streak counter:

- A tick counts as a *skip* when the audit returned no actionable findings AND no in-flight work was continued AND the next-tick plan is empty.
- A tick that does real work — commit, PR opened, blocker or feature-split recorded, metric moved, in-flight work continued — resets the counter to zero.
- When the streak reaches the threshold — default **eight ticks OR two hours of skip-only wakes, whichever comes first** — the loop pauses, summarizes what was reviewed during the streak, and asks the user: keep going / pivot orientation / stop.

Threshold is configurable per loop at setup. The "or two hours" floor protects against slow-cadence loops (daily/weekly cron) where eight ticks would be too long to wait for a drift signal.

## Initial Run Reporting

After creating the schedule, immediately start one tick so the user can confirm the automation works. The first report speaks in human terms. No cron strings, no flag names, no session IDs, no platform jargon. Each consuming command defines its specific report shape, but every initial report names:

- Target.
- Effective human cadence.
- What the first tick did (continued pending work / audited / opened PR / blocked / no safe action).
- Verification result if code changed.
- Management URL if the platform exposes one.

## What this doc is not

- Not a standalone workflow, and not a slash command — `core/loop` is kb_get-only. Invoke `/janitor` or `/smith` (or whichever recurring-loop command applies).
- Not a place to record command-specific safety boundaries, audit lenses, or end criteria. Those live in `flows/build/janitor` and `flows/build/smith` so each command's intent stays distinct.
- Not the platform adapter doc. Scheduler-specific behavior lives in `core/janitor-platforms`.

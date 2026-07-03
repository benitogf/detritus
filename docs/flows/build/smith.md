---
description: Recurring loop that takes a feature from /plan all the way to an open PR — the in-session single-agent instantiation of the universal pipeline. Sibling to /janitor; /janitor is the maintenance loop, /smith is the feature loop.
argument-hint: <feature description> [interval] [--platform auto|codex|claude-code|github-actions|cursor|windsurf|generic]
triggers:
  - smith
  - feature loop
  - implement feature
  - build feature
  - feature worker
  - feature smith
when: User invokes /smith with a feature description to build. The first tick passes through /plan to settle scope and acceptance criteria with the user; subsequent ticks run autonomously toward the agreed checklist and end at the open PR (one per impacted repo).
related:
  - core/flows
  - core/loop
  - core/build
  - core/completion
  - core/coordination
  - flows/plan/plan
  - flows/build/janitor
  - flows/github/gh
  - flows/github/gh-self-review
  - flows/github/gh-issue-work
  - flows/github/gh-issue-create
  - flows/github/gh-feedback-work
  - core/janitor-platforms
  - flows/principles/truthseeker
---

# /smith — Feature Loop with /plan Gate

Drive a feature from a fresh `/plan` conversation through to an open PR. `/smith` is the sibling to `/janitor`: same loop mechanics, opposite intent — `/janitor` is the maintenance loop (preserves behavior), `/smith` is the feature loop (adds behavior within an agreed spec).

`/smith` is the **in-session single-agent instantiation of the universal pipeline** (`core/flows`): plan → execute → review-with-rework → deliver, run sequentially with **no worker fan-out** — all build work happens in and is visible from the main session. The one sub-agent it spawns is the bounded build-phase audit **reporter** (*Build Phase Audit Agent Contract* below): an anti-bloat device returning a compact per-tick file, never a worker that builds.

Shared mechanics (scratchpad layout, durability rule, cadence guideline, skip-streak guardrail, mid-loop pivot, `/gh` delivery, shared main/audit-agent rules) live in `core/loop`, and the **build unit** (smallest delta → verification hard gate → commit) and **delivery** (self-review convergence → one PR per impacted repo via `gh-issue-work`) live in `core/build` — both referenced rather than restated here. `/smith` runs that build unit *sequentially*; the parallel loop (`/forge`, candyland) runs one per coder. This doc owns `/smith`'s `/plan` pass-through, feature-spec scratchpad sections, how the build unit applies within its acceptance-checklist/feature-branch structure, delivery, and `/smith`-specific safety boundaries.

> ## ⛔ The loop must never stall
>
> `/smith` is autonomous-to-done. The single most common — and most damaging — failure is **ending a turn with a status report and no live trigger for the next tick**. The loop then sits dead until the user pokes it, which defeats the entire command. A status report is *not* progress; only a committed delta plus a guaranteed next tick is.
>
> The only legitimate places to hand control back to the user are: **(a)** *Delivery* — the PR(s) are open and the loop is done, **(b)** a **capability blocker** — a failure no decision can resolve (missing credentials, absent permissions, unreachable infrastructure, a toolchain broken *outside* the repo), surfaced immediately with a full postmortem (`core/completion` → disposition 3; do not sit idle), or **(c)** an explicit user halt. **A decision is never a hand-back.** Ambiguity, a trade-off, scope interpretation, an unclear root cause, unexpected difficulty, "this is taking long" — these are *decisions*: post-plan, `/smith` makes the best decision given context and **records it** (see *Decisions made autonomously* below); it never surfaces one, never asks the user, and never stalls on one. `blocked` is a last breath for a capability failure only — never a shortcut to avoid work, skip investigation, or dodge root-cause analysis. **Anywhere a decision arises, the tick MUST end by guaranteeing the next tick fires** — see *Initial /plan Pass-Through* step 5 and *Self-continuation* below. If you catch yourself writing "I'll continue next" without having armed a trigger, you have already stalled.
>
> ✅ Root cause is ambiguous between two subsystems → pick the better-evidenced one, record the call in *Decisions made autonomously*, keep building.
> ❌ Root cause is ambiguous → stop and ask the user which subsystem to look at.

## Inputs

Free-form feature description, plus optional platform / interval hints. `/smith` reads the wording — no setup-time mode selection.

Accepted invocations:

- A natural-language feature ask: `/smith add typed cancellation to subscribe-list`.
- A feature ask with a schedule/platform hint: `/smith add SSE fallback to the live worker overnight --platform claude-code`.
- Nothing at all → ask one concise question about what to build before going further. `/smith` with no spec is meaningless.

Structured form is supported as a convenience, not as the primary interface:

```
/smith <feature description> [interval] [--platform auto|codex|claude-code|github-actions|cursor|windsurf|generic]
```

## Initial /plan Pass-Through

The first invocation of `/smith` does **not** schedule the loop. It dispatches to `/plan` in the user's live session so scope, acceptance criteria, file inventory, and risks get settled with the user before any autonomous work begins.

1. Hand the feature description to `/plan`. `/plan` runs its standard analysis → findings → plan → insights → questions flow.
2. The user answers `/plan`'s questions. The conversation iterates until `/plan` has a settled implementation plan with concrete steps and no open questions.
3. The user explicitly approves (`yes go`, `proceed`, `implement`, etc.) — this is `/plan`'s standard pre-implementation gate.
4. `/smith` **captures the settled state**: it reads the latest Plan section from `/plan`'s output, lists each step as an acceptance item in the scratchpad, copies the user's relevant constraints verbatim into the scratchpad's *User-stated rules*, and records any genuinely-separate features or hard blockers from `/plan`'s Insights into the scratchpad's *Blockers & feature-splits* (`core/completion` dispositions — in-scope risks are built, not parked).
5. `/smith` then loads the platform adapter (`core/janitor-platforms`) and schedules the recurring loop. Cadence handling is the same as `/janitor`; durability is **not** — the build phase requires a durable runner (see *Build Phase Durability* below). **A live trigger must actually exist before the first report.** Declaring "I'll be the in-session runner" is *not* a schedule: if no platform scheduler is created, the agent itself owns firing the next tick (see *Self-continuation* below). Skipping this is the canonical stall — one tick runs, a report is emitted, and nothing ever wakes the loop again.
6. The first scheduled tick begins the build phase.

`/plan` runs only at first invocation. Subsequent scheduled ticks read the captured spec from the scratchpad — no interactive `/plan` inside autonomous ticks, since fresh-session schedulers have no user to converse with.

The captured spec is the build-phase contract. Treat it as immutable after scheduling unless an explicit pivot revises the scratchpad. Implementation discoveries may update blockers & feature-splits, verification evidence, changed files, and next-tick plans, but they do not quietly rewrite the feature spec or acceptance criteria. If reality proves the spec wrong or incomplete, stop the build delta, record the evidence, and route the change through the pivot path below before continuing.

### Self-continuation (who fires the next tick)

Every tick must end by guaranteeing the next one fires. There are two valid arrangements, and exactly one of them must be true at all times the loop is live:

- **Platform scheduler present** (Desktop/Cloud Routines, an external cron/launchd/systemd job driving `claude -p`, a Codex automation, GitHub Actions): the scheduler fires subsequent ticks. The agent does its tick, updates the scratchpad, and lets the schedule wake it again. This is the default that *Initial /plan Pass-Through* step 5 sets up.
- **Interactive session with no external scheduler (the agent IS the runner):** the agent must self-arm the next tick before yielding — e.g. call `ScheduleWakeup` with a short delay and a prompt that re-enters the build loop — so work advances without the user typing anything. The on-disk scratchpad plus the pushed branch carry state across the gap (durability mode 1). Maximize work per tick (one self-wake should cover a meaningful, verified, committed delta, not a single line). **Never** end the turn with a report-and-wait; that is the exact stall this section exists to prevent.

In both arrangements the rule is identical: a tick that did not reach *Delivery*, hit a capability blocker, or get explicitly halted MUST leave a live trigger for the next tick. Report progress *in passing* when useful, never *instead of* continuing. Stop arming the trigger only at the three legitimate hand-back points named in *The loop must never stall*.

If `/plan` reaches a settled state but the user pushes back on scope or risk afterward, that's a mid-loop pivot via chat (`core/loop` → *Shared Main Agent Rules*) — the truthseeker pause applies before rewriting the scratchpad's spec, and the next scheduled tick honors the revised state.

## Build Phase Durability

The build phase requires a durable runner (`core/loop` → *Durability* mode 1). Disposable runners (Cloud Routines, Codex `worktree`, stateless GitHub Actions) cannot host build-phase state under either fallback mode:

- **Mode 2 (GitHub-state-only) does not apply.** The mode-2 fallback puts the State block "in the issue or PR bodies the loop maintains." `/smith` opens no PR until every acceptance item ticks green — there is nothing to write the State block into between ticks. A long-lived tracking issue isn't a /gh-router convention (issues describe problems, not loop state); using one anyway would create a documentation trail outside the established flow.
- **Mode 3 (report incompatibility) is the right fallback.** The platform adapter must check at setup that the selected scheduler is durable; if not, report the incompatibility and ask the user to pick a durable scheduler (Desktop Routines, external scheduler, Codex `local`) or a different platform. Do not silently fall through to mode 2.

Durable local mode 1 is therefore the default on every platform. Which scheduler is durable vs disposable is owned by `core/janitor-platforms`, not restated here: on Codex, `local` (or a thread heartbeat) is durable and `worktree` is disposable; on Claude Code, Desktop Routines or the external scheduler run against the real checkout while Cloud Routines are disposable. The disposable runners (Codex `worktree`, Claude Cloud Routines) stay incompatible with `/smith` — they remain valid for `/janitor`.

**Future — portable state mode (non-default).** A future GitHub-backed state mode could re-enable disposable runners by carrying the compact State block in a GitHub issue or PR body — `core/loop` → *Durability* mode 2, but with a host that exists *before* any PR opens. That needs new machinery and is only worth it if the disposable-runner cost is justified for a given loop. Until it exists, mode 2 stays inapplicable to `/smith` and mode 3 is the required fallback (see *Safety Boundaries*).

## Scratchpad — smith-specific sections

The shared scratchpad spine (current orientation, tick log, state block) is defined in `core/loop`. `/smith` adds these sections above the tick log, in order:

- **Feature spec** — the agreed Plan section from `/plan` (or `/vibe`'s readiness handoff), captured verbatim. One paragraph or short list describing what is being built and why. This is immutable during build unless an explicit pivot rewrites it.
- **User-stated rules** — verbatim constraints from the `/plan` conversation. Quoted, not paraphrased.
- **Acceptance criteria** — checklist of objectively-verifiable items derived from `/plan`'s steps or `/vibe`'s settled spec. Each item: `- [ ] <item description> — <verification: test name, function signature, endpoint shape, etc.>`. Items tick green during the build phase; when all are checked, the loop delivers. Do not add, remove, or reinterpret acceptance items during build except through an explicit pivot.

The State block (defined in `core/loop`) gets these `/smith`-specific fields added; the generic fields `core/loop`'s *State block* already requires (in-flight work, metric + delta, loop-end progress, skip-streak counter, blockers & feature-splits, next-tick plan, last directive) are not re-listed here:

- **Acceptance items checked** — `N/M`, naming the **current acceptance item** the build phase is working (the one-line summary of which item is next).
- **Changed files** — the files touched on `feat/<slug>` so far, so a fresh agent sees the build surface without re-diffing.
- **Verification command** — the canonical command this loop runs to gate a commit.
- **Last verification result** — `green` / `red`, dated; on red, the failure evidence (failing test name, assertion, or first error) the next tick retries against.
- **Feature branch** — `feat/<slug>` long-lived branch where build-phase commits accumulate.
- **PR(s) opened** — the PR URL(s) once *Delivery* opens them (one per impacted repo); otherwise empty.

### Decisions made autonomously

An append-only section recording every *decision* the loop resolved on its own (per *The loop must never stall* → the smith rule: post-plan a decision is decided-and-recorded, never surfaced). Each entry:

- **what** — the choice made, in one line.
- **why** — the reasoning that selected it given the current context.
- **evidence** — the file:line / test output / spec clause that grounds the call.
- **alternatives rejected** — the other options considered and why each lost.

These are **recorded here, not sent to the user.** The user reads them if and when they inspect the ledger; the loop does not pause for acknowledgement. Only a capability blocker (with its postmortem) ever surfaces mid-loop.

✅ Spec is silent on cache eviction policy → chose LRU (matches the sibling module at `cache/lru.go:12`), recorded here, kept building.
❌ Spec is silent on cache eviction policy → emitted a report and waited for the user to choose.

### Checkpoint-then-/clear

Every scratchpad update in build loop step 10 is a checkpoint per `core/loop` → *Checkpoint-then-/clear*: after it lands, the scratchpad + git are the complete resume state, and at the heavy boundaries `core/loop` names the tick report ends with the literal `checkpoint complete — safe to /clear` line. A user `/clear` at that point is lossless — the next tick resumes from the ledger. Never rely on `/compact`; summarized chat is not a state carrier.

## Completion

`/smith`'s definition of done is `core/completion`'s, not a local variant. The build phase's **exit gate is the four done-conditions** there — every acceptance box `[x]` with evidence, the verification gate green, a clean `/gh-self-review` pass, and no new deferral markers — computed from the durable ledger (the scratchpad's *Acceptance criteria*), never from memory. The loop continues until all four hold; it does not exit, hand back as "done", or convert a remaining in-scope item into a deferral (`core/completion` → *Exit gate*).

Disposition of anything the build encounters follows `core/completion`'s three dispositions: in-scope & handle-able now is **done now** (the default — no phases, no "future work", no stub); a **genuinely separate feature** is a disposition-2 feature-split surfaced in the State block's *Blockers & feature-splits* for the user to triage (never a silent park); a **capability blocker** — a failure no decision can resolve — is surfaced with its postmortem (disposition 3, capability-failures only per `core/completion`; a decision is never disposition 3). "Hazard" is not a parking lot for handle-able in-scope work, and a decision the loop can make is not a blocker. A feature whose in-scope work lands in **more than one repo** is still disposition 1 — `/smith` delivers **one PR per impacted repo** (see *Delivery*), never demoting the cross-repo half to a feature-split or a blocker.

## Smith Loop

### Build phase

Each scheduled wake during the build phase follows this order:

1. Resolve the target workspace and load local project instructions.
2. Read the scratchpad at `.smith/<slug>.md` (mechanism in `core/loop` → *Durable Cross-Tick State*). Honor the current orientation. Verify the State block's next-tick plan still matches reality.
3. Check whether the feature branch (`feat/<slug>`) has commits the next tick must continue, and whether any in-flight sub-agent work is still running.
4. If work is pending, continue or integrate that work.
5. If no work is pending, spawn an audit sub-agent with the **verification audit** brief (see *Build Phase Audit Agent Contract* below): read the spec and acceptance checklist, report current state per item, propose the smallest next delta toward the next unchecked item.
6. Critically review the audit findings. Drop deltas that fall outside the spec (those become disposition-2 feature-splits or disposition-3 blockers per `core/completion`, not commits).
7. Implement the smallest in-spec delta on `feat/<slug>`.
8. Run the canonical verification command. Verification must complete green on this tick — partial-tick verification does not count, and a tick that fails verification does **not** commit.
9. If verification is green and at least one acceptance item moves from `[ ]` to `[x]`, commit + push to `feat/<slug>`. Do **not** open a PR yet — commits accumulate on the branch until all acceptance items are checked.
10. Update the scratchpad — this is a **checkpoint** (`core/loop` → *Checkpoint-then-/clear*): tick log entry, State block (acceptance items checked, changed files, verification command + last result/failure evidence, next-tick plan, blockers & feature-splits if any), Acceptance criteria section (move ticked items to `[x]`), Decisions made autonomously section (append any decision resolved this tick) — left sufficient for a fresh agent to resume from scratchpad + git alone. The build phase's checkpoint events are: each completed acceptance item, each failed-verification cluster (serialize the failure evidence — this never softens step 8's hard gate or commits on red), the self-review pass (*Delivery* step 1), and the delivery itself. A user `/clear` is safe only after this update lands.
11. If all acceptance items are now checked, proceed to *Delivery* below. Otherwise, the next wake continues the build phase.

Do not pollute the user's main thread with raw audit logs.

### Delivery

When the final acceptance item ticks green:

1. **Loop `/gh-self-review` to a clean read.** Run the convergence in `core/build` → *Delivery* step 1 against the full `feat/<slug>` diff: on any amendment, fix it with build-phase ticks (commit + verify + update scratchpad) and re-review; stop only at a no-blocker pass on a diff unchanged since that pass. This is the same convergence `gh-issue-work` enforces in its Phase 8a — `/smith` inherits it verbatim, not a relaxed variant.
2. **Push the branch.** `git push -u origin feat/<slug>`. The hand-off below enters `gh-issue-work` at Phase 9 (open PR), which assumes the branch is already pushed.
3. **Open one PR per impacted repo via `gh-issue-work` Phase 9** (`core/build` → *Delivery* steps 2–3 and *Multi-repo delivery*; do not re-implement `gh pr create`). If no GitHub issue is linked yet, invoke `/gh-issue-create` first against the captured *Feature spec* + *Acceptance criteria*. `/smith` adds one carve-out to that delivery: skip `gh-issue-work` Phase 8a (step 1 above already produced a clean read on the exact diff being shipped) and Phase 8b (the user's Plan-time approval that initiated the run IS the authorisation, captured in the scratchpad's *Last user directive*). The "skip 8a/8b" carve-out is `/smith`-only — do not generalise it to other `gh-issue-work` callers.

   **Multi-repo:** when the feature's in-scope work spans **N repos** (N≥1, no cap), run steps 1–3 **once per impacted repo**, per `core/build` → *Multi-repo delivery* (disposition 1, not a feature-split). The loop is **not** done when the first repo's PR opens — it continues until every impacted repo's PR is open (or that repo's specific failure is surfaced).

**This holds even when `/smith` runs interactively** (the agent is the runner, a user is in the session). A live session does **not** convert the autonomous delivery into a gated one — the invocation is the authorisation regardless of who is watching. Do **not** pause to ask "want me to push / open the PR?": that is a stall (it is none of the three legitimate hand-back points in *The loop must never stall*), and it imports the ad-hoc "confirm before pushing/PR" default into a loop whose contract explicitly overrides it. Open the PR, then report the URL.

After the PR(s) open, `/smith` is **done** — record the URL(s) in the State block's *PR(s) opened*, report them, and end the loop. Later review feedback on those PRs is handled by `/gh-feedback-work` on a new invocation, not by this loop.

## Build Phase Audit Agent Contract

Shared audit rules in `core/loop` → *Shared Audit Agent Rules* apply. On top of those, the build-phase audit is a **verification + gap-finding** audit, not a discovery audit — a bounded anti-bloat reporter, never a worker:

- Read the scratchpad's *Feature spec* and *Acceptance criteria*.
- Code context is zero-setup (no pack/index): use `code_map`/`code_outline` for structure, `code_graph` for navigation, native Grep for text (`flows/project/code`).
- For each unchecked acceptance item, report current state: implemented (with evidence), partially implemented (with what's missing), or not started.
- Propose the smallest next delta toward the next unchecked item. The proposal includes: files to touch, what changes, what verification proves the item is met.
- Do **not** propose work outside the spec. Out-of-spec needs (a refactor that would help, a related cleanup) get reported as feature-splits/blockers into the State block's *Blockers & feature-splits*, not as deltas to implement.
- Do **not** propose multi-item deltas. One item at a time, in checklist order, unless two items share a single fix obviously and the agent can name both.

GOOD tasks to delegate — each a single bounded read against the current item: audit the **current acceptance item**, inspect the **failing verification output**, map the **affected files** for the next delta, review the `feat/<slug>` **diff for blockers**, summarize **missing tests** for the item. The BAD tasks — implementing the feature, deciding scope, or owning the loop — belong to the main agent, never the sub-agent (`core/loop` → *Shared Audit Agent Rules*: the main agent is the only loop owner).

Report shape follows `core/loop` → *Per-tick report files*, with the smith fields: finding/status per item, evidence (file:line / test name), affected files, smallest next delta (files + change + verification), and the verification command + result.

## Build Phase Main Agent Contract

Shared main-agent rules in `core/loop` → *Shared Main Agent Rules* apply. On top of those, during the build phase:

- Treat verification-audit findings as untrusted input. Prove an acceptance item is unmet by reading the code or running the verification before changing anything.
- Treat the captured Feature spec, User-stated rules, and Acceptance criteria as the source of truth. Code, tasks, and implementation notes conform to the scratchpad; they do not redefine it.
- Implement only the proposed in-spec delta. No drive-by refactors, no scope creep.
- Verification is a hard gate per commit — a failing verification does not commit, the tick logs the failure as next-tick context, and the next wake retries with the gap evidence in hand.
- If the proposed delta would require touching files or APIs outside what the spec named, **stop** and report as a feature-split or blocker (`core/completion` disposition 2/3). The user pivots the spec if it should be expanded; the loop does not silently expand its own scope.
- One feature branch (`feat/<slug>`) **per impacted repo** accumulates that repo's build-phase commits. Do not open intermediate PRs; the PR(s) open only when all acceptance items tick (see *Delivery*). A feature spanning N repos opens N coordinated PRs (`core/build` → *Multi-repo delivery*) — the cross-repo half is in-scope (disposition 1), never a feature-split.

## Safety Boundaries

Allowed:

- Feature work that matches the captured spec and ticks an acceptance item.
- Test additions that prove acceptance items (verification scaffolding).
- Refactoring strictly inside files the spec named, when the refactor is required to implement an acceptance item.
- Documentation updates for the new feature.

Not allowed:

- Work outside the spec (touches files the spec didn't name, adds behavior not in the acceptance checklist). This is a feature-split (disposition 2), not a finding.
- Quietly mutating the captured Feature spec, User-stated rules, or Acceptance criteria during build. Spec changes require an explicit pivot and scratchpad revision before more build work continues.
- Refactoring unrelated to acceptance items.
- API changes beyond what an acceptance item explicitly requires.
- Dependency additions not in the spec. If the spec implies a new dependency, name it as a feature-split and ask before adding.
- Skipping verification. A build-phase tick that doesn't verify green does not commit.
- Opening a PR before all acceptance items are checked.
- Opening a PR with a stale self-review (*restated from Delivery step 1*). If `/gh-self-review` forced any amendment to the diff — blocker fix, non-blocker cleanup, anything — the prior clean read is stale and the self-review MUST re-run before delegating to `gh-issue-work`'s Phase 9.
- Re-implementing `gh pr create` in the smith flow instead of delegating to `gh-issue-work` Phase 9 (*restated from Delivery step 3*). Drift between two PR-opening paths is exactly what the self-review-loop fix exists to prevent.
- Inserting a confirmation gate before the autonomous PR delivery — pausing to ask "should I push / open the PR?" once the self-review has converged, **even with a live user in the session**. Delivery opens the PR autonomously (*Delivery* steps 1–3); the only legitimate hand-back is *after* the PR is open (*The loop must never stall* point (a)). Gating before it bypasses the `/smith` flow the user invoked.
- Scheduling the loop on a disposable runner. See *Build Phase Durability* above — mode 2 has no PR body to host the State block until acceptance items tick, and mode 3 (report incompatibility) is the required fallback.

## Initial Run

The "initial run" for `/smith` is split across two events:

1. The interactive `/plan` conversation at first invocation. No autonomous work yet; this is the live gate.
2. The first scheduled build-phase tick, immediately after the loop is scheduled, so the user can confirm the automation works.

The post-`/plan` report follows `core/loop` → *Initial Run Reporting*. Concretely:

```
Smith scheduled on <target>: <one-line feature description>.
Cadence: <human cadence — e.g. "every 30 minutes" or "weeknights at 10pm">.
Acceptance items: <N total>; first tick will start <item 1 title>.
Feature branch will be feat/<slug>.
Manage anytime at <management URL if the platform exposes one>.
```

When the build completes and the PR(s) open, surface each PR URL on its own line — the loop is done.

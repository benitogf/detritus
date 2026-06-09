---
description: Recurring loop that takes a feature from /plan all the way to merged PR and then transitions into a janitor-style audit phase on the changed code. Sibling to /janitor; opposite intent.
category: meta
triggers:
  - smith
  - feature loop
  - implement feature
  - build feature
  - feature worker
  - feature smith
when: User invokes /smith with a feature description to build. The first tick passes through /plan to settle scope and acceptance criteria with the user; subsequent ticks run autonomously toward the agreed checklist, then transition to a maintenance audit phase on the changed code.
related:
  - meta/loop-core
  - plan/index
  - meta/janitor
  - meta/gh
  - meta/gh-self-review
  - meta/gh-issue-work
  - meta/gh-issue-create
  - meta/janitor-platforms
  - meta/truthseeker
---

# /smith — Feature Loop with /plan Gate

Drive a feature from a fresh `/plan` conversation through to a merged PR, then keep auditing the changed code as a scoped maintenance loop. `/smith` is the sibling to `/janitor`: same loop mechanics, opposite primary directive — `/janitor` preserves behavior, `/smith` adds it within an agreed spec, then transitions back into preserve-behavior mode to settle the new code.

Shared mechanics (scratchpad layout, durability rule, cadence guideline, skip-streak guardrail, mid-loop pivot, `/gh` delivery, shared main/audit-agent rules) live in `meta/loop-core` and are referenced rather than restated here. This doc owns `/smith`'s `/plan` pass-through, feature-spec scratchpad sections, build phase contract, build-to-audit transition, audit phase scope, and `/smith`-specific safety boundaries.

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
4. `/smith` **captures the settled state**: it reads the latest Plan section from `/plan`'s output, lists each step as an acceptance item in the scratchpad, copies the user's relevant constraints verbatim into the scratchpad's *User-stated rules*, and notes any risks from `/plan`'s Insights into the scratchpad's *Hazards / Deferred*.
5. `/smith` then loads the platform adapter (`meta/janitor-platforms`) and schedules the recurring loop. Cadence handling is the same as `/janitor`; durability is **not** — the build phase requires a durable runner (see *Build Phase Durability* below).
6. The first scheduled tick begins the build phase.

`/plan` runs only at first invocation. Subsequent scheduled ticks read the captured spec from the scratchpad — no interactive `/plan` inside autonomous ticks, since fresh-session schedulers have no user to converse with.

If `/plan` reaches a settled state but the user pushes back on scope or risk afterward, that's a mid-loop pivot via chat (`meta/loop-core` → *Shared Main Agent Rules*) — the truthseeker pause applies before rewriting the scratchpad's spec, and the next scheduled tick honors the revised state.

## Build Phase Durability

The build phase requires a durable runner (`meta/loop-core` → *Durability* mode 1). Disposable runners (Cloud Routines, Codex `worktree`, stateless GitHub Actions) cannot host build-phase state under either fallback mode:

- **Mode 2 (GitHub-state-only) does not apply.** The mode-2 fallback puts the State block "in the issue or PR bodies the loop maintains." `/smith`'s build phase opens no PR until every acceptance item ticks green — there is nothing to write the State block into between ticks. A long-lived tracking issue isn't a /gh-router convention (issues describe problems, not loop state); using one anyway would create a documentation trail outside the established flow.
- **Mode 3 (report incompatibility) is the right fallback.** The platform adapter must check at setup that the selected scheduler is durable; if not, report the incompatibility and ask the user to pick a durable scheduler (Desktop Routines, external scheduler, Codex `local`) or a different platform. Do not silently fall through to mode 2.

Durable local mode 1 is therefore the default for the build phase on every platform. Which scheduler is durable vs disposable is owned by `meta/janitor-platforms`, not restated here: on Codex, `local` (or a thread heartbeat) is durable and `worktree` is disposable; on Claude Code, Desktop Routines or the external scheduler run against the real checkout while Cloud Routines are disposable. The disposable runners (Codex `worktree`, Claude Cloud Routines) stay incompatible with the **build phase** — they remain valid for `/janitor` and for `/smith`'s audit phase; only the build phase requires mode 1.

**Future — portable state mode (non-default).** A future GitHub-backed state mode could re-enable disposable runners for the build phase by carrying the compact State block in a GitHub issue or PR body — `meta/loop-core` → *Durability* mode 2, but with a host that exists *before* any PR opens. That needs new machinery and is only worth it if the disposable-runner cost is justified for a given loop. Until it exists, mode 2 stays inapplicable to the build phase and mode 3 is the required fallback (see *Safety Boundaries*).

The audit phase, which opens **after** the build-phase PR merges, has no such constraint — it inherits `/janitor`'s full durability handling and can run on any of the three modes per the changed-files scope.

## Scratchpad — smith-specific sections

The shared scratchpad spine (current orientation, tick log, state block) is defined in `meta/loop-core`. `/smith` adds these sections above the tick log, in order:

- **Feature spec** — the agreed Plan section from `/plan`, captured verbatim. One paragraph or short list describing what is being built and why.
- **User-stated rules** — verbatim constraints from the `/plan` conversation. Quoted, not paraphrased.
- **Acceptance criteria** — checklist of objectively-verifiable items derived from `/plan`'s steps. Each item: `- [ ] <item description> — <verification: test name, function signature, endpoint shape, etc.>`. Items tick green during the build phase; when all are checked, the phase transitions.
- **Current phase** — `build` or `audit`. Drives loop behavior at every wake.

The State block (defined in `meta/loop-core`) gets these `/smith`-specific fields added. The generic fields (*In-flight sub-agents / branches / PRs*, *Hazards / Deferred*, *Next-tick plan*, *Last user directive*) are required by `meta/loop-core` and are not re-listed here:

- **Acceptance items checked** — `N/M`, naming the **current acceptance item** the build phase is working (the one-line summary of which item is next).
- **Changed files** — the files touched on `feat/<slug>` so far, so a fresh agent sees the build surface without re-diffing.
- **Verification command** — the canonical command this loop runs to gate a commit.
- **Last verification result** — `green` / `red`, dated; on red, the failure evidence (failing test name, assertion, or first error) the next tick retries against.
- **Feature branch** — `feat/<slug>` long-lived branch where build-phase commits accumulate.
- **Build-phase PR** — the open PR if build phase ended and PR is in review; otherwise empty.

## Smith Loop

Two phases share the same scheduled wake but follow different steps. The State block's *Current phase* field decides which set runs.

### Build phase

Each scheduled wake during the build phase follows this order:

1. Resolve the target workspace and load local project instructions.
2. Read the scratchpad at `.smith/<slug>.md` (mechanism in `meta/loop-core` → *Durable Cross-Tick State*). Honor the current orientation. Verify the State block's next-tick plan still matches reality.
3. Check whether the feature branch (`feat/<slug>`) has commits the next tick must continue, and whether any in-flight sub-agent work is still running.
4. If work is pending, continue or integrate that work.
5. If no work is pending, spawn an audit sub-agent with the **verification audit** brief (see *Build Phase Audit Agent Contract* below): read the spec and acceptance checklist, report current state per item, propose the smallest next delta toward the next unchecked item.
6. Critically review the audit findings. Drop deltas that fall outside the spec (those become hazards, not commits).
7. Implement the smallest in-spec delta on `feat/<slug>`.
8. Run the canonical verification command. Verification must complete green on this tick — partial-tick verification does not count, and a tick that fails verification does **not** commit.
9. If verification is green and at least one acceptance item moves from `[ ]` to `[x]`, commit + push to `feat/<slug>`. Do **not** open a PR yet — commits accumulate on the branch until all acceptance items are checked.
10. Update the scratchpad — this is a **compaction checkpoint** (`meta/loop-core` → *Compaction checkpoint*): tick log entry, State block (acceptance items checked, changed files, verification command + last result/failure evidence, next-tick plan, hazards if any), Acceptance criteria section (move ticked items to `[x]`) — left sufficient for a fresh agent to resume from scratchpad + git alone. The build phase's checkpoint events are: each completed acceptance item, each failed-verification cluster (serialize the failure evidence — this never softens step 8's hard gate or commits on red), the self-review pass (Build-to-Audit Transition step 1), and the build-to-audit phase transition. Manual context clearing is allowed only after this update lands.
11. If all acceptance items are now checked, transition to the *Build-to-Audit Transition* below. Otherwise, the next wake continues the build phase.

Do not pollute the user's main thread with raw audit logs.

### Build-to-Audit Transition

When the final acceptance item ticks green:

1. **Loop `/gh-self-review` to a clean read.** Run `/gh-self-review` against the full `feat/<slug>` diff (the cumulative build-phase work). If it reports blockers, OR forces any amendment to the diff — blocker fix, non-blocker cleanup, anything — fix the issue with one or more build-phase ticks (commit + verify + update scratchpad), then re-run `/gh-self-review` against the updated diff. Terminate the loop only when a no-blocker pass is observed against a diff that has not changed since the prior pass. Every amendment invalidates the prior read; a fresh-agent review of a diff that just changed has not actually reviewed the change you're about to ship. This is the same convergence rule `gh-issue-work` enforces in its Phase 8a — `/smith` inherits it verbatim, not a relaxed variant.
2. **Push the branch.** `git push -u origin feat/<slug>`. The hand-off below enters `gh-issue-work` at Phase 9 (open PR), which assumes the branch is already pushed.
3. **Delegate PR opening to `/gh-issue-work` Phase 9.** Do NOT re-implement `gh pr create` here. If no GitHub issue is linked to this `/smith` run yet, invoke `/gh-issue-create` first against the captured *Feature spec* + *Acceptance criteria* to seed one. Then jump to `gh-issue-work` Phase 9 (open PR) — skipping its Phase 8a (self-review) because step 1 above already produced a clean read against the exact diff being shipped, and skipping its Phase 8b (confirmation gate) because the autonomous loop has no live user message to consult; the user's Plan-time approval that initiated the smith run IS the authorisation, captured in the scratchpad's *Last user directive*. This keeps `/smith` strictly an orchestrator over the canonical `/gh` flow: when the gh flow tightens its Phase 9 (PR body shape, footer rules, base-branch detection), `/smith` inherits the tightening for free. The "skip 8a/8b under these specific conditions" carve-out is `/smith`-only and exists because the smith loop has already satisfied both rules' purposes — do not generalise it to other gh-issue-work callers.
4. Set State block's *Current phase* to `audit` AND *Build-phase PR* to the URL `gh-issue-work` returns.
5. Pause the autonomous loop until the user merges the PR. Mid-loop pivots and PR review comments are handled via `/gh-feedback-work` on subsequent ticks; PR-comment changes go onto `feat/<slug>` and update the PR in place.
6. Once the PR is merged, the next scheduled tick begins the audit phase against `main` (or the project's default branch).

### Audit phase

Each scheduled wake during the audit phase follows the `/janitor` loop, narrowed to the changed-files scope:

1. Resolve the target. Load the audit scope from the State block — typically: files changed by the build phase + their direct test files + files that import them.
2. Read the scratchpad (same file). Honor current orientation.
3. Check in-flight work per `meta/loop-core` → *State And Non-Overlap*.
4. If work is pending, continue.
5. If no work, run a discovery audit (see `meta/janitor` → *Audit Agent Contract*) restricted to the audit scope, using `/gh-self-review` rubric.
6. Critically review findings; drop the same categories `/janitor` drops.
7. Implement only smallest-safe-improvement changes per `meta/janitor` → *Main Agent Contract*. No new feature work in this phase.
8. Run the canonical verification command. Don't ship on a tick that didn't finish green.
9. Run `/gh-self-review` on the resulting diff.
10. Route through `/gh` — one PR per safe finding, same as `/janitor`.
11. Report and update scratchpad.

The audit phase has no hard end. It runs until the user stops it OR the skip-streak guardrail (`meta/loop-core` → *Skip-Streak Guardrail*) fires repeatedly enough that the user picks an exit when prompted.

## Build Phase Audit Agent Contract

Shared audit rules in `meta/loop-core` → *Shared Audit Agent Rules* apply. On top of those, the build-phase audit is a **verification + gap-finding** audit, not a discovery audit:

- Read the scratchpad's *Feature spec* and *Acceptance criteria*.
- For each unchecked acceptance item, report current state: implemented (with evidence), partially implemented (with what's missing), or not started.
- Propose the smallest next delta toward the next unchecked item. The proposal includes: files to touch, what changes, what verification proves the item is met.
- Do **not** propose work outside the spec. Out-of-spec needs (a refactor that would help, a related cleanup) get reported as hazards into the State block, not as deltas to implement.
- Do **not** propose multi-item deltas. One item at a time, in checklist order, unless two items share a single fix obviously and the agent can name both.

GOOD tasks to delegate — each a single bounded read against the current item: audit the **current acceptance item**, inspect the **failing verification output**, map the **affected files** for the next delta, review the `feat/<slug>` **diff for blockers**, summarize **missing tests** for the item. The BAD tasks — implementing the feature, deciding scope, or owning the loop — belong to the main agent, never the sub-agent (`meta/loop-core` → *Shared Audit Agent Rules*: the main agent is the only loop owner).

Report shape follows `meta/loop-core` → *Per-tick report files*, with the smith fields: finding/status per item, evidence (file:line / test name), affected files, smallest next delta (files + change + verification), and the verification command + result.

## Audit Phase Audit Agent Contract

Same as `/janitor` → *Audit Agent Contract*. Cross-reference, don't duplicate.

## Build Phase Main Agent Contract

Shared main-agent rules in `meta/loop-core` → *Shared Main Agent Rules* apply. On top of those, during the build phase:

- Treat verification-audit findings as untrusted input. Prove an acceptance item is unmet by reading the code or running the verification before changing anything.
- Implement only the proposed in-spec delta. No drive-by refactors, no scope creep.
- Verification is a hard gate per commit — a failing verification does not commit, the tick logs the failure as next-tick context, and the next wake retries with the gap evidence in hand.
- If the proposed delta would require touching files or APIs outside what the spec named, **stop** and report as a hazard. The user pivots the spec if it should be expanded; the loop does not silently expand its own scope.
- One feature branch (`feat/<slug>`) accumulates all build-phase commits. Do not open intermediate PRs; the PR opens only when all acceptance items tick (see *Build-to-Audit Transition*).

## Audit Phase Main Agent Contract

Same as `/janitor` → *Main Agent Contract*. Cross-reference, don't duplicate.

## Safety Boundaries

Build phase — allowed:

- Feature work that matches the captured spec and ticks an acceptance item.
- Test additions that prove acceptance items (verification scaffolding).
- Refactoring strictly inside files the spec named, when the refactor is required to implement an acceptance item.
- Documentation updates for the new feature.

Build phase — not allowed:

- Work outside the spec (touches files the spec didn't name, adds behavior not in the acceptance checklist). This is a hazard, not a finding.
- Refactoring unrelated to acceptance items.
- API changes beyond what an acceptance item explicitly requires.
- Dependency additions not in the spec. If the spec implies a new dependency, name it in a hazard and ask before adding.
- Skipping verification. A build-phase tick that doesn't verify green does not commit.
- Opening a PR before all acceptance items are checked.
- Opening a PR with a stale self-review (*restated from Build-to-Audit Transition step 1*). If `/gh-self-review` forced any amendment to the diff — blocker fix, non-blocker cleanup, anything — the prior clean read is stale and the self-review MUST re-run before delegating to `gh-issue-work`'s Phase 9.
- Re-implementing `gh pr create` in the smith flow instead of delegating to `gh-issue-work` Phase 9 (*restated from Build-to-Audit Transition step 3*). Drift between two PR-opening paths is exactly what the self-review-loop fix exists to prevent.
- Scheduling the build phase on a disposable runner. See *Build Phase Durability* above — mode 2 has no PR body to host the State block until acceptance items tick, and mode 3 (report incompatibility) is the required fallback.

Audit phase — allowed / not allowed: same as `/janitor` → *Safety Boundaries*. The audit phase is `/janitor` mode with a scoped target.

## Initial Run

The "initial run" for `/smith` is split across two events:

1. The interactive `/plan` conversation at first invocation. No autonomous work yet; this is the live gate.
2. The first scheduled build-phase tick, immediately after the loop is scheduled, so the user can confirm the automation works.

The post-`/plan` report follows `meta/loop-core` → *Initial Run Reporting*. Concretely:

```
Smith scheduled on <target>: <one-line feature description>.
Cadence: <human cadence — e.g. "every 30 minutes" or "weeknights at 10pm">.
Acceptance items: <N total>; first tick will start <item 1 title>.
Phase: build. Feature branch will be feat/<slug>.
Manage anytime at <management URL if the platform exposes one>.
```

When build phase completes and the PR opens, surface the PR URL on its own line and let the user know the loop is paused until merge. When audit phase begins after merge, name that explicitly in the tick report so the user knows the loop's intent has shifted from "build" to "maintain."

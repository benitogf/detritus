---
description: The single definition of "done" every builder inherits — definition of done, the three dispositions for any encountered work, the forbidden list, the exit gate, and the durable acceptance ledger that doubles as the coordination substrate. Kills silent deferral of handle-able in-scope work by contract. Do not invoke directly.
triggers:
  - completion
  - definition of done
  - deferral
  - defer
  - disposition
  - exit gate
  - durable ledger
  - hazard
when: Internal. Loaded via kb_get by every flow that builds or loops (/smith, /forge, /janitor, /vibe), by core/loop, core/build, core/planning, and by roles/tech-lead + core/coder, to define when a unit / loop / PR is done and what may (and may not) be surfaced for later.
related:
  - core/build
  - core/loop
  - core/planning
  - flows/build/smith
  - flows/build/forge
  - flows/build/janitor
  - flows/plan/vibe
  - roles/tech-lead
  - core/coder
  - core/memory
  - flows/github/gh-self-review
  - flows/principles/truthseeker
---

# Core — Completion: done, dispositions, and the exit gate

The one authoritative definition of "done." Every builder and every loop inherits it, so silent
deferral of handle-able in-scope work is impossible by contract — no one has to forbid it by hand each
run. The durable acceptance ledger this doctrine defines is also the **in-process coordination
substrate** (`core/coordination` Realization A) and the firewall for learned memory (only
verified-green work distils).

> ## ⛔ Do not invoke directly
> No slash command. Loaded via `kb_get` by the build/loop flows, `core/build`, `core/loop`,
> `core/planning`, `roles/tech-lead`, and `core/coder`. Those docs reference this one; they never
> restate a conflicting completion or deferral rule.

## Definition of done

A unit / loop / PR is **done** only when ALL of these hold, verified from **durable state** (files,
git, command output) — never from memory of the conversation:

1. **Every acceptance criterion is objectively met.** Each `[ ]` is ticked `[x]` with evidence — a
   passing test, command output, or a documented manual check. An unchecked box ⇒ not done.
1a. **A named requirement covers EVERY instance, not a sample.** When a criterion or plan names a
   *pattern* or *class* rather than one site — "convert the PR links", "handle multiple X",
   "rename all callers of Y", "remove every use of Z" — done requires covering **every** site that
   exhibits it, not the salient few. Enumerate the full set by grep/search **up front**, change all
   of them, then re-run the same search to prove **zero** remain before ticking the box (`grep-to-zero`).
   Converting the obvious sites and stopping silently drops part of a stated requirement and reads as
   done when it is not (e.g. a "make PR links copyable" pass that fixed the detail views but missed the
   table cell rendering the same link).
2. **The verification gate is green.** Build + tests + lint pass, *or there is a documented reason a
   check could not run* (silent skipping does not satisfy it). This extends `core/build`'s per-unit
   gate to the loop-exit gate.
2a. **In-scope features are wired + reachable.** "Suite green + clean self-review" is satisfiable with
   **dead-in-prod code** — a symbol/route/feature called only by its own test, or by nothing. So the gate
   *additionally* requires that each in-scope feature has a production caller: proven by running the
   assembled artifact or tracing from the entrypoint (`main`/router) into the change. **Unwired** code
   (no production caller) is a **BLOCKER**, not a nit and not "verify later"; "needs live verification" /
   "known non-blocker" may **not** cover it. Distinguish this from **wired-but-needs-a-browser** — a
   feature that *is* reachable but whose final confirmation wants a live UI/session, which is legitimately
   deferrable to `/verify`. The first is undelivered; the second is delivered-and-pending-confirmation.
3. **The critic pass is clean.** A fresh-context self-review (`/gh-self-review`) returns no actionable
   in-scope finding; it re-runs until clean (maker ≠ checker — see M3/M4).
4. **No new deferral markers.** The change introduces no unjustified `TODO`/`FIXME`/`XXX`/"future
   work"/"for now"/"in a later step" (a grep over the diff is clean).

Each acceptance criterion is written as a verifiable contract: the **desired end state**, the
**evidence required**, the **constraints not to violate**, and a **hard ceiling on turns/budget**.
"Done" is **not** "every possible improvement made" — it is "the agreed scope met and verified."

## The three dispositions

For ANY work a builder or loop encounters, exactly one disposition applies. This is a **closed enum —
`1 in-scope-now | 2 feature-split | 3 capability-blocker` (do NOT invent others)**. "Hazard" is **not** a
fourth option — it is retired as a catch-all and resolves only to disposition 2 or 3.

1. **In-scope & handle-able now → DO IT NOW.** The default; it covers the vast majority. No phases, no
   "future work", no `TODO`, no follow-up issue, no workplan-instead-of-work. Work that is in-scope but
   lands in **another repo** (a co-required cross-repo change) is still disposition 1 — delivered as its
   own PR in that repo (`core/build` → *Multi-repo delivery*: one feature → a PR per impacted repo, N≥1,
   no cap), never demoted to a feature-split or a blocker because it crosses a repo boundary.
2. **Genuinely separate feature (outside the agreed scope) → FEATURE-SPLIT.** A distinct, named new
   plan or issue — never a silent park.
   - Developer-facing (`/plan`, `/forge`, `/smith`): surface it for triage in the State block's
     *Blockers & feature-splits* section.
   - Autonomous (`/vibe`): the architect decides, records, and splits a truly separate feature into its
     own plan.
3. **Capability blocker = capability failure only → SURFACE as a blocker.** A failure **no decision can
   resolve**: missing credentials, absent permissions, unreachable infrastructure, a toolchain broken
   *outside* the repo. This is a **last breath, not a shortcut** — `blocked` may NOT be used to avoid
   work, skip investigation, or dodge root-cause analysis. It carries a **postmortem** (see *Closed
   blocker definition & postmortem schema*). The narrow exception, not the default escape hatch.

**A decision is NEVER disposition 3.** A *decision* — ambiguity, a trade-off, scope interpretation, an
unclear root cause, unexpected difficulty, "this is taking long" — is a choice the agent can resolve by
*choosing*. It never stops the flow and never reaches the user post-plan; it **falls down the escalation
ladder** (`core/coordination` → *Re-planning loop*), is decided at the lowest tier with authority, and
is **recorded** (`/smith`: the ledger's *Decisions made autonomously* section; `/forge`: the tech-lead
decides and re-spawns; candyland: exactly one tier up). Only a capability blocker is disposition 3.

## Closed blocker definition & postmortem schema

A **blocker (capability failure)** is a failure **no decision can resolve** — missing credentials,
absent permissions, unreachable infrastructure, a toolchain broken *outside* the repo. It is the ONLY
thing that legitimately reaches terminal `blocked`. It is a **last breath, not a shortcut**: `blocked`
may NOT be used to avoid work, skip investigation, or dodge root-cause analysis. Ambiguity, trade-offs,
difficulty, unclear root cause, and budget anxiety are **decisions**, never blockers — they fall down
the escalation ladder (`core/coordination`).

`blocked` is **invalid without ALL six postmortem fields** (missing field = incomplete → rejected/bounced):

| Field | Content |
|---|---|
| `attempts` | each attempt made and its result. |
| `failing_capability` | the exact capability that failed (one line). |
| `evidence` | verbatim commands + error output proving the failure. |
| `root_cause_so_far` | root-cause analysis as far as it got. |
| `human_unblock_action` | precisely what a human must provide to unblock. |
| `partial_work_state` | branch, commits, ledger refs for work already done. |

## Explicitly forbidden

- Splitting agreed scope into "phase 2" / "later" to exit sooner.
- Using `blocked` for a **decision** (ambiguity, trade-off, difficulty, unclear root cause) instead of
  falling down the escalation ladder — a decision is never a blocker.
- Using `blocked` as a **shortcut** to skip root-cause analysis or investigation — it is a
  capability-failure-only last breath, and only with a complete postmortem.
- Leaving `TODO`/`FIXME`/"future work" for a disposition-1 item.
- Filing a follow-up issue for in-scope work instead of doing it.
- Declaring done with an unchecked criterion or a red gate.
- Handling only the obvious instances of a named pattern/class while leaving others unchanged — with no
  final search proving the set is empty (see *Definition of done* 1a, `grep-to-zero`).
- Producing a "workplan" / "next steps" doc *in place of* doing the work.
- Using "hazard" as a parking lot for handle-able in-scope work.
- Shipping a placeholder / stub / "simplified for now" implementation of in-scope behavior — a stub is
  deferral in disguise. **Dead-in-prod code — wired only by its own test, or by nothing — is the same
  failure** (an unwired stub), a BLOCKER, never satisfied by "suite green + clean self-review."
- Reporting a no-op as a delivery: terminal `done`/"success" on a run/loop/quest/PR that delivered
  nothing in-scope (see *Honest terminal state*).
- Asserting a config-derived terminal the work was never observed to earn — e.g. `reviewed` on a
  `deliver=="review"` run that never examined its target PR (see *Honest terminal state*, carve-out 3).
- Parking an orchestrator in terminal `blocked` on the **first** failing review/verdict while the gap
  is still finishable within the remediation budget — deferral at the supervisor level (see *The exit
  gate*).
- Treating a co-required **cross-repo** change as a feature-split or blocker to exit one repo's PR
  sooner — it is in-scope (disposition 1), delivered as a PR per impacted repo (`core/build` →
  *Multi-repo delivery*).

## The exit gate (the forcing function)

A loop / PR MAY complete only when conditions (1)–(4) of *Definition of done* all hold. If any fails,
the loop **continues** — next tick, next critic pass — feeding the gap back as the next iteration's
work. It does not exit, does not hand back as "done", and does not convert remaining in-scope work into
a deferral.

**`blocked` is earned by bounded remediation, not declared on first failure.** When a verification /
review / verdict step reports an unmet acceptance criterion or commitment, the orchestrator must feed
that gap back as new work — spawn a unit targeting **exactly the unmet item** (carrying the reviewer's
evidence) and re-verify — bounded by a remediation budget (the K-attempt circuit breaker in M6). Only a
gap that survives the budget is a disposition-3 capability blocker. Parking in terminal `blocked` on the
**first** failing review while the gap is still finishable is the silent-deferral failure at the
**orchestrator / supervisor level**, forbidden exactly as it is at the single-unit level. This binds a
program-level supervisor reviewing per-commitment verdicts (e.g. a campaign's intent review) as much as
a per-task coder loop — an orchestrator never parks with finishable in-scope work undone.

The only legitimate hand-backs are three: **milestone / PR-open**, a **capability blocker** (disposition
3, postmortem-backed), or an **explicit user halt**. A **decision is not a legitimate hand-back** — it
falls down the escalation ladder (`core/coordination`), is decided at the lowest tier with authority,
and is recorded, never surfaced to the user post-plan. Anywhere else, the loop owes a next step (see
`core/loop` → self-continuation). For a **multi-repo feature** the milestone is *every* impacted repo's PR being open
(`core/build` → *Multi-repo delivery*) — opening the first repo's PR is not a stopping point while
another impacted repo still owes a PR.

This same verified-green gate is the firewall for **learned memory**: only verified-green work distils a
reusable, cross-project lesson (`core/memory` → *When to distil*); an unverified run distils nothing.

## Honest terminal state

Terminal states are a **closed enum — `done | surfaced-only | blocked | delivery-failed | stopped`
(do NOT invent others)**. Terminal `done` / "success" MUST reflect **actual in-scope delivery**. A run
/ loop / quest / PR that delivered **nothing** in-scope is a **distinct outcome, never `done`-as-success**
— for example (the 2026-07-03 review found candyland dropped the `"skipped"` disposition writer, so an
all-skip quest that executed zero items terminated plain `done` with an empty summary — the exact
looks-done-but-isn't class this rule kills):

- dead-in-prod code (passes condition 2, fails condition 2a — no production caller);
- a report-only / zero-execution tick (work surfaced and triaged, none executed);
- `prsOpened:0` while in-scope work is still open.

Reporting surfaces — CLI, dashboard, status field — must **name it as such** so neither a human nor a
downstream flow reads a no-op as a delivery: e.g. `surfaced-only`, or
`done (report-only: N surfaced, 0 executed, 0 PRs)`.

The discriminator across every carve-out below is the **delivery mode** (`run.deliver`, a **closed enum
— `pr | branch | feedback | review` (do NOT invent others)** — `core/build` → *Delivery modes*),
**not** the PR count. The zero-delivery
no-op detector flags only runs that delivered **nothing in-scope**; it must **never** flag a
`branch`, `feedback`, or `review` run as a no-op failure.

**Delivery attempted-but-FAILED is a failed terminal, distinct from a no-op — and never `done`.** The
no-op detector above is about work that was never *produced*. A separate, equally-forbidden state is work
that was produced but could **not be delivered at the mechanical layer**: `git push` rejected (branch
protection, secret-scanning push protection, non-fast-forward), a PR-open API error, an auth/permission
failure. This is **not** a no-op (in-scope work exists, committed) and **not** a clean `done` (the
delivery the mode promised did not land). Rules:
- **Name it** `delivery-failed (<verbatim reason>)` on every reporting surface — never fold it into
  `done`, and never let a green intent/review gate upstream mask a failed push downstream.
- **Feed the error back**, don't strand it. The git/gh error text is usually **machine-fixable** by an
  agent that reads it (e.g. a push blocked by secret scanning → remove the offending file/history; a
  non-fast-forward → rebase). In a loop/campaign this is a remediation round on the delivery error
  itself, bounded like any other; only a failure that survives the budget is a real hard blocker.
- **A multi-unit deliverable (e.g. one PR per repo) is not `done` until *every* impacted unit delivered.**
  One unit's delivery failure is isolated from the others but keeps the whole out of clean `done`.

**Carve-out 1 — branch delivery is legitimate `done` with `prsOpened:0`.** A run/child delivering to a
branch (`deliver=="branch"`) is *legitimately* `done` with no PR — its delivery **is** the branch commit,
and the parent campaign opens the PR (`core/build` → *Multi-repo delivery*).

**Carve-out 2 — feedback delivery is legitimate `done` with no NEW PR.** A `feedback` run
(`deliver=="feedback"`) updated an **existing** PR in place — its delivery **is** the pushed fix on that
PR's head branch (`core/build` → *Delivery modes*). The terminal reads e.g. `addressed N findings on
PR #M` and links the **updated** PR — never "opened a PR", and never a duplicate PR.

**Carve-out 3 — a review with no actionable findings is legitimate `done` with no PR by design.** A
`review` run (`deliver=="review"`) that found nothing actionable completes with **no PR at all** — a
valid "reviewed, nothing to do." The terminal reads e.g. `reviewed (no actionable findings)`. (Actionable
findings instead become `feedback` work against the PR — carve-out 2.)

This carve-out is **earned by observed work, never asserted from the delivery mode alone.** A terminal
is legitimate only when the work it names was *observed to happen* — never inferred from `deliver`. A
`review`/`feedback` run must have **actually examined its subject** (read the target PR's diff/comments)
before it may report "nothing actionable"; one that terminates without examining its target reviewed
nothing — a zero-delivery no-op (flag it as such, and **name the target** it did/didn't review), not a
valid "reviewed." The failure to prevent: a loop that reports `reviewed` on a PR it never opened because
the terminal keyed on configuration (`deliver=="review"`) instead of on evidence a review occurred.

## The durable ledger (also the coordination substrate)

- **The acceptance-criteria checklist in the loop's plan/scratchpad file IS the task ledger.** Each
  tick re-reads the contract fresh, works the highest-priority unchecked item, and ticks it `[x]` only
  when green. The stopping condition — "all boxes checked + gate green" — is **computed from the file**,
  not from the agent's self-assessment. An unchecked box is visible, durable state the next tick picks
  up; you cannot "quietly move on."
- **Re-grounding is selective.** Re-derive only the *open* items — a cheap grep of unchecked `[ ]`
  lines — never reload the whole document. This is the token-economy lever: the orchestrator sheds
  state to the ledger and re-derives open work each tick, keeping its own context lean.
- This is the substrate `core/coordination` Realization A uses **in place of an in-process bus**.
  The multi-process realization (Realization B) mirrors it in ooo: a server-side open-items read filter
  over `graph/nodes/*` returns only non-`done` nodes — same discipline, different transport.

## Harness & loop mechanics (M1–M6)

Framing (Osmani): *"Agent = Model + Harness. A decent model with a great harness beats a great model
with a bad harness."* This doctrine sharpens the harness's **Control** (loop convergence), **Observe &
Verify** (exit gates), and **Persist** (the durable ledger).

- **M1 — Ralph loop (Huntley).** Fresh context each tick; re-derive only the *open* checklist items;
  **one item per loop**; implement; run that unit's tests; tick `[x]` only when green; commit; exit
  only when zero open items AND the gate is green. Keep a tight budget (~170k usable; output quality
  degrades around 147–152k).
- **M2 — Persist.** Progress lives in files + git, never inferred from chat history; commit per green
  unit so git is the durable ledger.
- **M3 — Observe & Verify is truth; maker ≠ checker.** Completion requires observable green
  verification (or a documented reason a check could not run) + every box checked. The critic runs in a
  **fresh** context, not the implementer's (detritus does this via `/gh-self-review`). *"Success is
  silent, failures are verbose."*
- **M4 — Reflexion critic loop (arXiv:2303.11366).** The pre-exit critic re-runs, feeding findings back
  as the next iteration's work, until it returns no actionable in-scope finding. A "noted" finding does
  **not** satisfy the gate — findings are *fixed, not noted*.
- **M5 — Control / leanness (lushbinary loop-engineering).** Near context limits, compact to durable
  artifacts (M2) and continue from the re-derived open items (M1). Keep MCP tool surfaces tiny and skill
  docs terse — every tool is stamped into the prompt each request; 10 focused beats 50 overlapping;
  rules stay short.
- **M6 — Enforcement is orchestrator-driven (not a hook).** The **portable contract is this prose
  doctrine** — it ships with detritus and works everywhere with no per-machine setup. Enforcement is the
  orchestrator forcing continuation until the gate is green:
  - **In-process (`/smith`, `/forge`):** the loop itself re-derives the open `[ ]` items and continues;
    it exits only at zero-open + green (the exit gate above). No separate process controls it.
  - **Multi-process (candyland):** the **tech-lead / conductor** re-spawns a coder that stopped before
    its task's acceptance criteria + tests are green, and marks `blocked` only on a real blocker
    (`core/coordination`).
  - Both pair with a **circuit breaker**: after **K=3** failed attempts on one unit, escalate **one tier
    up** the ladder (`core/coordination` → *Re-planning loop*) — a *decision* is decided and recorded
    there, never surfaced; only a **capability failure** surviving the cap becomes a postmortem-backed
    `blocked` (push the branch + record it) — rather than thrash the subscription quota.
  - A Claude Code Stop hook (`setup-extra-rules`) is **optional local hardening** a developer may opt
    into — **not** a build deliverable and **not** the portable contract. Per-machine hooks are exactly
    the bespoke setup the portability goal avoids.

> Ratchet note (Osmani, good-spec): *"Every line in a good AGENTS.md should be traceable to a specific
> thing that went wrong."* This doctrine is one ratchet entry — it exists because the deferral failure
> recurred. Future tightenings should likewise be failure-driven (via `/grow`), not speculative.

## Sources

- Osmani — *agent-harness-engineering* (Agent = Model + Harness; Control / Observe & Verify / Persist)
  and *good-spec* (acceptance criteria as verifiable contracts; the ratchet rule).
- Huntley — the *ralph* loop (fresh context, one item per loop, re-derive open items).
- lushbinary — *loop-engineering* (control / leanness near context limits).
- Reflexion — arXiv:2303.11366 (the self-critique-and-retry loop).

## What this doc is not

- Not a slash command — `core/completion` is `kb_get`-only.
- Not the build unit (smallest delta → per-commit verification gate → commit) — that is `core/build`.
- Not the loop mechanics (scratchpad, durability, cadence, skip-streak, self-continuation) — those are
  `core/loop`.
- Not phase/scope/audit policy — that stays in each wrapping command (`flows/build/smith`,
  `flows/build/forge`, `flows/build/janitor`) so each command's intent stays distinct.

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
2. **The verification gate is green.** Build + tests + lint pass, *or there is a documented reason a
   check could not run* (silent skipping does not satisfy it). This extends `core/build`'s per-unit
   gate to the loop-exit gate.
3. **The critic pass is clean.** A fresh-context self-review (`/gh-self-review`) returns no actionable
   in-scope finding; it re-runs until clean (maker ≠ checker — see M3/M4).
4. **No new deferral markers.** The change introduces no unjustified `TODO`/`FIXME`/`XXX`/"future
   work"/"for now"/"in a later step" (a grep over the diff is clean).

Each acceptance criterion is written as a verifiable contract: the **desired end state**, the
**evidence required**, the **constraints not to violate**, and a **hard ceiling on turns/budget**.
"Done" is **not** "every possible improvement made" — it is "the agreed scope met and verified."

## The three dispositions

For ANY work a builder or loop encounters, exactly one disposition applies. "Hazard" is **not** a
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
3. **Hard blocker beyond the loop's authority → SURFACE as a blocker.** Irreversible/external action,
   a genuine spec contradiction, a missing credential. The narrow exception, not the default escape
   hatch.

## Explicitly forbidden

- Splitting agreed scope into "phase 2" / "later" to exit sooner.
- Leaving `TODO`/`FIXME`/"future work" for a disposition-1 item.
- Filing a follow-up issue for in-scope work instead of doing it.
- Declaring done with an unchecked criterion or a red gate.
- Producing a "workplan" / "next steps" doc *in place of* doing the work.
- Using "hazard" as a parking lot for handle-able in-scope work.
- Shipping a placeholder / stub / "simplified for now" implementation of in-scope behavior — a stub is
  deferral in disguise.
- Treating a co-required **cross-repo** change as a feature-split or blocker to exit one repo's PR
  sooner — it is in-scope (disposition 1), delivered as a PR per impacted repo (`core/build` →
  *Multi-repo delivery*).

## The exit gate (the forcing function)

A loop / PR MAY complete only when conditions (1)–(4) of *Definition of done* all hold. If any fails,
the loop **continues** — next tick, next critic pass — feeding the gap back as the next iteration's
work. It does not exit, does not hand back as "done", and does not convert remaining in-scope work into
a deferral.

The only legitimate hand-backs are three: **milestone / PR-open**, a **hard blocker** (disposition 3),
or an **explicit user halt**. Anywhere else, the loop owes a next step (see `core/loop` →
self-continuation). For a **multi-repo feature** the milestone is *every* impacted repo's PR being open
(`core/build` → *Multi-repo delivery*) — opening the first repo's PR is not a stopping point while
another impacted repo still owes a PR.

This same verified-green gate is the firewall for **learned memory**: only verified-green work distils a
reusable, cross-project lesson (`core/memory` → *When to distil*); an unverified run distils nothing.

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
  - Both pair with a **circuit breaker**: after **K=3** failed attempts on one unit, escalate to a
    blocker (push the branch + record it) rather than thrash the subscription quota.
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

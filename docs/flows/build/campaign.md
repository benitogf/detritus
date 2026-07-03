---
description: A Candyland program-level campaign — launches the full intent→delivery cycle in the sidecar from a high-level goal, partial brief, or detailed plan. Candyland owns the whole program: it produces an Intent Brief and runs its gates, decomposes the goal into child quests, reviews, and opens one PR per impacted repo. Does NOT require a prior /plan.
argument-hint: "[input-file] [folder ...]"
triggers:
  - campaign
  - candyland campaign
  - program-level build
  - run a whole program in the sidecar
  - decompose a goal into quests
when: User wants Candyland to own an entire program from a high-level goal (or partial brief, or detailed plan) — intent capture, decomposition into child quests, review, and one PR per repo — driven out-of-process in the sidecar and watched in the dashboard. Use it when the work is bigger than a single quest/run and you want Candyland to own the full intent→delivery cycle.
related:
  - flows/plan/vibe
  - core/planning
  - core/dream
  - core/intent-review
  - flows/build/candyland
  - flows/build/forge
  - flows/build/quest
---

# /campaign — a Candyland program-level execution

`/campaign` launches a **program-level campaign** in the candyland sidecar from a high-level goal, a partial brief, or a fully detailed plan. Unlike `/forge`/`/candyland` (which build a single settled `.plan/<slug>.md`) or `/quest`/`/janitor` (which tick one iterative loop), a campaign hands Candyland the **whole intent→delivery cycle**: Candyland captures intent (an Intent Brief and its gates per `core/intent-review`), decomposes the goal into **child quests** (a campaign's unit of decomposition is always a quest — never a bare run; runs live *inside* quests), drives and reviews them, and opens **one PR per impacted repo**.

A campaign does **not** require a prior `/plan`. The input may be as thin as a one-line goal or as thick as a detailed plan — Candyland owns the planning that the input does not already supply (`core/planning`, `core/dream`). detritus is only the client/launcher: it settles the input and autonomy, ensures the sidecar is up, and starts the campaign over REST; Candyland owns everything after.

## /campaign is NOT /vibe

`/campaign` and `/vibe` are **distinct flows, and neither is a legacy or delegating form of the other.**

- `/vibe` is an **in-session** flow. It runs in this conversation and drives the in-process build path (`/smith`); it does **not** use the Candyland multi-agent loop.
- `/campaign` is a **Candyland-run program execution**. It runs **out-of-process** in the sidecar as a multi-agent loop (conductor + coders over the ooo bus, `core/coordination`), watched in the dashboard.

They may share doctrine — intent capture, the planning altitude, the completion dispositions — but invoking one is never a substitute for the other. Pick `/vibe` when you want the work to happen in this session; pick `/campaign` when you want Candyland to own the program out-of-process.

## Settle the campaign input

Before launching, read the detritus KB guidance for program-level intent and decompose:

- **`core/intent-review`** — the Intent Brief and the gates Candyland runs against it; the campaign's intent is the north star every child quest inherits.
- **`core/planning`** / **`core/dream`** — the planning altitude Candyland owns for whatever the input does not already specify.
- **`core/completion`** — the three dispositions every child quest obeys (in-scope work done now, separate features become feature-splits, hard blockers surfaced — never silently deferred). This holds at the **program level** too: when the final intent review finds a commitment `missed` — **or a repo's delivery fails at the mechanical layer** (push rejected, PR-open errored; `core/completion` → *Honest terminal state*) — the campaign does **not** park in `blocked` on that first look, and **never reports `done`** while the gap stands. It spawns a **remediation quest targeting exactly that commitment or delivery error** (carrying the reviewer's evidence, or the verbatim git/gh error text — which is usually machine-fixable, e.g. a push blocked by secret scanning), and **re-reviews**, bounded by a remediation-round budget. Only a gap that survives the budget blocks (a real hard blocker); a lingering `partial` still delivers, annotating the PR. A campaign never blocks with finishable work left undone.

Refine the user's request into the campaign input written to the input file passed on argv (keeps a large goal/brief/plan off the command line, mirroring how `/candyland` and `/quest` pass their files), plus the scope folders. `/campaign` never invents the goal — it refines what the user asked for into the input Candyland's program-level loop can own.

## Decompose into quests — concurrent by default

A campaign's decomposition is a set of **child quests**, and only quests — a campaign never spawns a bare run directly (a quest owns its own runs as it ticks discover→triage→run→review→PR). The tech-lead's program-level partition (`roles/tech-lead` → *Phase choreography*) obeys two rules:

- **Every commitment becomes a quest.** Iterative, open-ended, or multi-PR work (a recurring triage loop, wave-based remediation, an audit that surfaces more work as it runs) is exactly what a quest is *for* — do not flatten it into a one-shot run. A bounded single-deliverable commitment is still a quest with a short life; it is not demoted to a run.
- **Concurrency is the default; sequencing is the exception you justify.** The partition **always attempts** to make quests fork-safe and independent (disjoint repos, files, modules — the gates in `core/todo-audit`) so they run concurrently, because independent work serialized is wasted wall-clock. Fall back to sequential ordering (via `deps`) **only** where the work genuinely depends on another quest's output — and that dependency is the justification, not a default.

**Anti-pattern (observed to fail):** decomposing a program into a flat, sequential list of one bare run per commitment — zero quests, no fan-out — when the commitments are independent (separate repos, disjoint files) and/or iterative (a triage loop, staged waves). That strands the iterative work with no loop to carry it and serializes work that had no dependency, and it is the tech-lead abdicating the program-level partition.

## Steps

1. **Settle the campaign input** (the goal/brief/plan and the scope folders) per the section above, and write the input to an input file.
2. **Ensure candyland is up, then start the campaign over REST.** Run `detritus --campaign-run <input-file> [folder ...]` (folders default to the cwd). detritus health-checks the sidecar, starts it if down, then `POST`s `/api/campaigns` with `{input, folders, autonomyLevel, deliver, targetPr}`, reads back the campaign id, and `POST`s `/api/campaigns/{id}/begin` to start it. A campaign launches at `autonomyLevel: L2` — campaigns are **never L1**, because a report-only campaign would strand with no PR. The `deliver`/`targetPr` fields carry the input's derived delivery intent (see *Delivery mode matches the input's intent* below).
3. **Hand off to the dashboard.** Report the campaign id and the dashboard URL — that is where the Intent Brief, the child quest graph, the per-task review, and the per-repo deliveries show, and where the campaign is stopped. A campaign opens **one PR per impacted repo**. A single repo's delivery failure is **isolated** — it does not fail the other repos — but it is **not** a clean terminal: the failing repo is surfaced *and* fed into a remediation quest (per the `core/completion` bullet above), and the campaign does **not** report `done` while an intended PR failed to open. Isolation means "don't fail the others," never "accept the failure."

## Delivery mode matches the input's intent

A campaign launches at `L2`, but its **delivery mode** still tracks what the input asks for, and that mode propagates to the child quests it decomposes into (`core/build` → *Delivery modes*: `pr|branch|feedback|review`):

- An **"address feedback on PR #N"** input → the affected child quest carries `deliver: feedback` with the **target PR(s)**: it updates **that existing PR in place** and **never opens a new PR** (mirroring `/gh-feedback-work`). Multi-repo feedback lands each repo's fix on that repo's existing PR.
- A **"review PR #N"** input → `deliver: review`: the review may end with **no PR** when nothing is actionable, and any actionable finding becomes `feedback` work on the PR in question. A no-PR `review` terminal is valid `done`, not a no-op (`core/completion` → *Honest terminal state*, carve-out 3).
- A plain new-program goal → `deliver: pr` (the default — one new PR per impacted repo, per the step above).

The `--campaign-run` launcher **derives** the delivery mode from the campaign input — reusing the same derivation the quest launcher uses (`deriveQuestDelivery`): a feedback/review **verb** plus a parsed **PR reference** (`#N`) selects `feedback`/`review` carrying that target PR, and anything else falls back to `deliver: pr` (new work). It then sends `deliver`/`targetPr` on the `POST /api/campaigns` body, and candyland propagates them to the affected child quests. Note this is **not** how autonomy is set: campaign autonomy is **fixed at `L2`** (never derived, never `L1`), whereas the delivery mode is derived per the input's verb + PR reference. The launcher **never** opens a duplicate PR for a feedback/review input, and never silently defaults a feedback/review input to `pr`.

### The program converges — one PR per repo

A campaign obeys `core/build` → *One deliverable, one PR — converge, don't spray*, and the program's shape enforces it: child quests **commit onto the shared campaign branch** (`deliver: branch`) and open **no PR of their own**, so the campaign delivers **one PR per impacted repo** at the end — never a scatter of competing child PRs. Re-attempts are the remediation the exit gate above already defines (a remediation quest targeting exactly the unmet commitment or delivery error onto the same branch), never a fresh "reconcile/consolidate/supersede" PR.

The settled candyland plan makes this structural (plan **Bucket B**: campaign→child-quests on the campaign branch, a stakeholder + tech-lead pair converging the program); until it lands, the `core/build` rule holds.

## Control (stop only)

Like `/candyland` and `/quest`, a campaign is lean: **observe + audit + stop**, no per-agent control, no resume. Halt a wrong or runaway campaign from the dashboard's Stop — candyland owns the spawned process tree, so it genuinely kills the conductor + child quests. Watch live state in the dashboard rather than polling.

## How /campaign relates to the other flows

- **`/vibe`** — the **in-session** flow that drives `/smith` in this conversation and does **not** use the Candyland loop. Distinct from `/campaign`; see *“/campaign is NOT /vibe”* above. Neither delegates to or supersedes the other.
- **`/quest`** — a **single** Candyland-native iterative loop (one objective, ticking discover→triage→run→review→PR). A campaign is a level up: it decomposes a program-level goal into **many** child quests (concurrent by default) and owns the intent cycle around them.
- **`/forge`** / **`/candyland`** — a **one-shot** plan-to-PR build of a settled `.plan/<slug>.md` (in-process for `/forge`, in the sidecar for `/candyland`). A campaign does not require a prior `/plan` and owns the planning the input does not supply.
- **candyland (the app)** — the out-of-process driver detritus hands the campaign to over REST: it owns the `/api/campaigns` contract, runs the program as a process tree over the ooo bus (`core/coordination`), and visualizes it. detritus is only the client/launcher — it ensures the sidecar is up and starts the campaign; candyland owns everything after.

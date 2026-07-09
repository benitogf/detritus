---
description: The settled flow model — every detritus build flow instantiates one pipeline (plan → execute → review-with-rework → deliver) on three orthogonal axes. Do not invoke directly; the shared reference every flow doc composes.
triggers:
  - flows-core
  - flow taxonomy
  - flow pipeline
  - orchestration ladder
  - PR policy
when: Internal. Loaded via kb_get by the flow docs (campaign, quest, adventure, janitor, candyland, forge, smith, vibe) for the shared model — the universal pipeline, the taxonomy, the three axes, the PR policy, and the safety rails.
related:
  - core/build
  - core/loop
  - core/completion
  - core/sidecar
  - flows/build/campaign
  - flows/build/quest
  - flows/build/adventure
  - flows/build/janitor
  - flows/build/candyland
  - flows/build/forge
  - flows/build/smith
  - flows/plan/vibe
  - flows/github/babysit
  - flows/github/gh
---

# Flows Core — one pipeline, many instantiations

> ## ⛔ Do not invoke directly
> No slash command. Loaded via `kb_get` by every flow doc; it holds the shared model so no flow doc restates it.

The flows are different ways to do the **same work**. Every flow instantiates one pipeline; no flow may skip a stage:

```
plan → execute → review-with-rework → deliver
```

- **plan** — intent settles BEFORE execution starts. Once planning settles, work runs to done **without stopping** — no pause gates, no approval gates, no mid-run user questions. What makes "never stop" true is the **decision-fallback ladder**: post-plan a *decision* (ambiguity, trade-off, scope interpretation, unexpected difficulty, unclear root cause) NEVER reaches the user — it falls to the lowest tier with authority, is decided there and **recorded**. `/smith` (single agent): decide the best option given context and record it in the ledger's *Decisions made autonomously* section. `/forge`: the coder emits `BLOCKED {json}`, the tech-lead (session) decides and re-spawns; the tech-lead's own fallback is the smith rule. candyland: escalate **exactly one tier up** — coder → tech-lead → quest-lead → tech-manager → intent-manager — decided at the lowest tier with authority (`core/coordination` → *Re-planning loop*). A **capability blocker** (a failure no decision can resolve — missing credentials, absent permissions, unreachable infrastructure, toolchain broken outside the repo) is the **only** thing that stops, and only with a postmortem (`core/completion`).
- **execute** — build units per `core/build` (smallest delta → verification hard gate → commit).
- **review-with-rework** — a fresh-context critic that can send work back, looping until clean, bounded: `/gh-self-review` convergence in-session; the reviewer's fix→re-review loop in the sidecar. Mandatory at every level.
- **deliver** — composes `/gh`; modes are a **closed enum — `pr | branch | feedback | review` (do NOT invent others)** per `core/build` → *Delivery modes*. Never merges — the human merge is the real gate.
- **watch (optional terminal phase)** — after `deliver` opens a PR, a flow may hand it to `/babysit` (`flows/github/babysit`) to carry it to merge: each tick folds in review feedback (via `/gh-feedback-work`) and merges the moment a **SHA-pinned human `APPROVED` review** covers the current HEAD. This does **not** violate "never merges — the human merge is the gate": the human approval *is* the gate, and `/babysit` only executes the mechanical merge it authorizes. Watch is opt-in per flow (see *PR-watch — the universal terminal phase* below), never a stage a flow may add on its own initiative to force a merge past review.

## Taxonomy

| Flow | Control | Where | What it is |
|---|---|---|---|
| **campaign** | intent manager + tech manager (convergence-gated) | sidecar | program → concurrent child quests on a shared branch → one PR per repo at the end |
| **quest** | quest-lead | sidecar | **bounded** iterative loop: ticks runs onto a quest branch → one PR per repo when the objective is met; terminating |
| **adventure** | quest-lead | sidecar | **open-ended freeseeking** loop: PR per accepted finding, perpetual until stopped/dry; janitor's sidecar homologue |
| **janitor** | single session | in-session | adventure's in-session homologue: discover→triage→fix→verify→deliver, one delivery at a time via /gh |
| **run** (via `/candyland`) | tech-lead + coders + reviewer | sidecar | one work unit: partition→build→integrate→review-loop→deliver |
| **forge** | session as tech-lead | in-session | run's homologue: this session partitions; coders are Agent-tool sub-agents |
| **smith** | single agent | in-session | same pipeline, no spawning; `/plan`-gated; ends at the open PR |
| **vibe** | — (intake style) | in-session | executive intake (`dream`) fronting an executor — default `/smith` |

## Three axes (orthogonal)

1. **Intake style** — developer (`/plan`: open-ended, user steers tech) vs executive (`dream`: multiple-choice, architect owns tech). `/vibe` IS the executive intake fronting an executor, not a rung on the ladder.
2. **Execution substrate** — in-session single agent (smith) / in-session multi-agent (forge) / sidecar processes (run, quest, adventure, campaign).
3. **Orchestration ladder** — run ⊂ quest ⊂ campaign. A campaign decomposes ONLY into quests (never bare runs); a quest owns its runs as it ticks. adventure sits beside quest — same machinery, different delivery policy.

In-session bounded homologues of quest are `/smith` (single agent) and `/forge` (parallel). Campaign has no in-session homologue.

## PR policy

- **Bounded objectives converge to one PR per impacted repo** (smith, forge, run, quest, campaign): iterate on a branch, deliver once.
- **Only open-ended freeseeking is multi-PR** (adventure / janitor): each accepted finding is a DISTINCT deliverable → its own run → its own PR.
- **Campaign-child quests open NO PRs** — the parent stamps `deliver: branch`; their runs commit onto the campaign branch; the campaign opens the per-repo PRs at its delivery gate.
- The bounded/open-ended split is **explicit at invocation** (which command was launched), never detected at runtime from objective wording.
- The shared doctrine all of this instantiates is `core/build` → *One deliverable, one PR — converge, don't spray*.

## PR-watch — the universal terminal phase

`/babysit` (`flows/github/babysit`) is the pipeline's single **watch-to-merge** step — the one place any flow's work reaches `merged`. It composes, it does not merge on its own authority: each tick it dispatches `/gh-feedback-work` for reviewer feedback and merges only when an `APPROVED` review's `commit_id` equals the current HEAD SHA (a push invalidates every prior approval). It is a **terminating loop** — merge is the exit — not a maintenance loop.

Because merge is irreversible and gated on a human's review, watch is **opt-in and uniform** across every PR-opening flow; the flow chooses how it offers the phase, but the merge gate is identical:

- **Offer-only flows** — `/smith` and `/forge` open the PR, report the URL, and **offer** `/babysit` as the one-command continuation that watches it to merge. They do not auto-run it: a developer often merges these by hand, and starting a watch loop unbidden would expand the flow's own scope.
- **Auto-chaining flows** — `/vibe` and `/gh-issue-work` **auto-chain** into `/babysit` per opened PR rather than merely offering. For `/vibe` the stakeholder is non-technical, so a "run `/babysit` next" note they can't action would be a silent drop (`core/completion`'s no-deferral rule). For `/gh-issue-work` (the interactive `/gh` flow) it is the deliberate default — safe because `/babysit` never merges without a fresh human approval and is bounded/haltable — with two carve-outs: an explicit "just open it, don't watch" opts out, and it does **not** auto-chain when entered as the PR-opener *for* an autonomous caller (`/smith`, `/forge`, sidecar), which own their own watch decision. In every case the human's review approval is the merge gate; autonomy ends at merge-on-approval, never at a self-approved merge.
- **Sidecar launchers** — `/candyland`, `/quest`, `/campaign` are observe-and-stop-only (`core/sidecar`) and **never merge inside the sidecar**; after the per-repo PRs open, the launcher names each PR and points the user at `/babysit` (in-session) to watch it to merge.

No flow ever self-approves. `/babysit`'s SHA-pinned approval invariant is the load-bearing guard — the watch phase can only merge what a human approved on the exact HEAD it merges.

## Safety rails

No flow merges a PR on its own authority — the human's review approval is the gate. The only path to `merged` is the optional `/babysit` watch phase above, and it merges solely on a SHA-pinned human `APPROVED` review (never self-approved, never admin-overridden). The dashboard Stop kills the sidecar's whole process tree; budget/concurrency caps and the bounded-loop family (every agentic loop has exactly one K-attempt bound, `core/completion`) contain runaways.

## In-session vs sidecar — the trade

In-session flows are **session-visible** and rely on the checkpoint-then-/clear discipline (`core/loop`): the ledger is the only resume state, and a `/clear` after a checkpoint is lossless. The sidecar is **process-fresh and dashboard-visible** (`core/sidecar`). Given enough work, any in-session context refills between clears — that boundary is where the sidecar is the right substrate: fresh processes AND observability.

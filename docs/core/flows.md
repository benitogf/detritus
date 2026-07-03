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

## Safety rails

No flow merges a PR — the human merge is the gate. The dashboard Stop kills the sidecar's whole process tree; budget/concurrency caps and the bounded-loop family (every agentic loop has exactly one K-attempt bound, `core/completion`) contain runaways.

## In-session vs sidecar — the trade

In-session flows are **session-visible** and rely on the checkpoint-then-/clear discipline (`core/loop`): the ledger is the only resume state, and a `/clear` after a checkpoint is lossless. The sidecar is **process-fresh and dashboard-visible** (`core/sidecar`). Given enough work, any in-session context refills between clears — that boundary is where the sidecar is the right substrate: fresh processes AND observability.

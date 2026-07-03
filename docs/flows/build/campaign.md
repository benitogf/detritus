---
description: A candyland program-level campaign — an intent manager and a tech manager drive a goal, partial brief, or detailed plan through two convergence gates: the brief partitions into concurrent child quests on a shared campaign branch, and the campaign opens one PR per impacted repo at its delivery gate. Does NOT require a prior /plan.
argument-hint: "[input-file] [folder ...]"
triggers:
  - campaign
  - candyland campaign
  - program-level build
  - decompose a goal into quests
when: User wants candyland to own an entire program from a goal, partial brief, or detailed plan — intent capture, decomposition into concurrent child quests, convergence gates, and one PR per repo — out-of-process in the sidecar, watched in the dashboard. Use it when the work is bigger than a single quest/run.
related:
  - core/flows
  - core/sidecar
  - flows/build/quest
  - roles/tech-lead
  - core/planning
  - core/dream
  - core/intent-review
  - core/completion
---

# /campaign — a program in the sidecar

A campaign hands candyland a whole program. **Two roles** drive it, each instantiated as fresh processes per stage (fresh instantiation = maker ≠ checker):

- **Intent manager** — owns intent gating end-to-end: produces the Intent Brief (stage agent id `intent-lead`; loads `core/planning` + `core/dream`) and runs the final per-commitment intent review (stage agent id `intent-reviewer`; loads `core/intent-review`).
- **Tech manager** — owns everything technical: partitions the brief into **child quests** via a machine-readable `QUESTS [...]` line (mirroring the run tech-lead's `PARTITION` convention), owns dependencies/concurrency, integration strategy across the shared branch, and remediation targeting. Loads `roles/tech-lead` (the program-altitude partition rules).

A campaign does **not** require a prior `/plan` — the input may be a one-line goal, a partial brief, or a detailed plan; the intent manager owns whatever planning the input does not already supply. (Neither does `/candyland` demand one: its intake settles a plan when the input is vague — `flows/build/candyland`.)

## Decompose into quests — concurrent by default

A campaign decomposes ONLY into **child quests**, never bare runs — a quest owns its runs as it ticks (`core/flows` → the orchestration ladder). Two partition rules:

- **Every commitment becomes a quest.** A bounded single-deliverable commitment is a short-lived quest, not a demoted run; iterative or discovery-shaped work is exactly what a quest is for.
- **Concurrency is the default; `deps` is the justified exception.** Quests are made fork-safe and independent (disjoint repos, files, modules) so they run concurrently; sequence only where one quest genuinely needs another's output — the dependency is the justification, never a default.

## Two convergence gates

1. **Gate 1 — before work launches.** The intent manager reviews the tech manager's quest partition against the brief ("would this partition plausibly deliver the intent?"); disagreement loops back to the tech manager, bounded. Only both-agree launches the quests.
2. **Gate 2 — before done.** Dual sign-off: the tech manager confirms **technical done** (integration green, review-loop clean); the intent manager runs the **per-commitment intent review** (`core/intent-review`). A `missed` verdict spawns a **remediation quest** targeted by the tech manager (carrying the reviewer's evidence), bounded, then re-reviews; a `partial` annotates the PR, never blocks. Only both-clean opens the per-repo PRs.

Every agentic loop has exactly one bound (`core/completion`'s K-attempt circuit breaker); a gap that survives its budget blocks honestly. The campaign never reports `done` past an unmet commitment, and never parks in `blocked` with finishable work left undone.

## Delivery — the program converges

Child quests carry `deliver: branch`: their runs commit onto the **campaign branch** (`campaign/<id>`) and open **no PRs of their own**. At the delivery gate the campaign opens **one PR per impacted repo** (`core/flows` → *PR policy*). A repo's delivery-mechanic failure (push rejected, PR-open errored) is isolated from the other repos but never accepted — it re-enters the bounded remediation loop carrying the verbatim git/gh error text (`core/completion` → *Honest terminal state*).

## Steps

1. **Settle the campaign input** (the goal/brief/plan plus scope folders), written to the input file passed on argv. An input that references a PR/issue is classified per the gh-mirror table (`core/sidecar` → *PR-link intake mirrors /gh*), and the resulting `deliver`/`targetPr` propagate to the affected child quests.
2. **Launch**: `detritus --campaign-run <input-file> [folder ...]` — ensure-up, REST create + begin (`core/sidecar`).
3. **Hand off to the dashboard**, printing the launch output contract (`core/sidecar`) — the Intent Brief, the quest graph, both gates, and the per-repo deliveries all show there.

## Control

Observe + stop only, per `core/sidecar` — Stop kills the whole campaign process tree, child quests included.

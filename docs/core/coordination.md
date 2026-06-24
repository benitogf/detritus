---
description: One portable agent-coordination protocol — orchestrator-single-writer + workers, message types, the task-graph, the re-planning loop with a K=3 escalation cap, and the verification gate — with two realizations (in-process native orchestration; the candyland bus). Do not invoke directly.
triggers:
  - coordination
  - coordination protocol
  - orchestrator
  - task-graph
  - re-planning
  - BLOCKED convention
when: Internal. Loaded via kb_get by /smith, /forge, roles/tech-lead, and core/coder to define how an orchestrator and workers coordinate a multi-task plan, what a worker may send back, and how the orchestrator re-plans — one contract, realized in-process or over the candyland bus.
related:
  - core/completion
  - roles/tech-lead
  - core/coder
  - flows/build/smith
  - flows/build/forge
---

# Core — Coordination: one protocol, two realizations

A single coordination protocol that both detritus (in-process) and candyland (multi-process) conform to,
so a developer experiences **one flow** across both. The **protocol is the portable artifact**; the
transport differs by where agents run. It reuses `core/completion`'s primitives — the durable acceptance
ledger, the verification gate, and the K=3 escalation cap — as its coordination substrate.

> ## ⛔ Do not invoke directly
> No slash command. Loaded via `kb_get` by `/smith`, `/forge`, `roles/tech-lead`, and `core/coder`.

## The shared protocol (the portable contract)

- **Roles — one orchestrator, many workers.** A single **orchestrator** is the only writer of the
  task-graph; **workers** do the partitioned work. Workers do **not** coordinate peer-to-peer for
  dependent work (conflicting peer decisions over unshared context are the dominant multi-agent failure
  mode for coding); they share context through the orchestrator and through git, not lossy peer messages.
- **Task-graph.** Nodes `{id, title, status ∈ {pending, in_progress, blocked, done}, owner, deps[],
  priority, version}`. The orchestrator single-writes; dependents auto-unblock when their deps reach
  `done`. (This is a superset of candyland's existing `partitionTask` — it adds
  `status`/`owner`/`priority`/`version`, reusing the same `PARTITION` parse path; see `roles/tech-lead`
  → *Partition emission format*.)
- **Message types** (FIPA-reduced, two-tier correlation): `{from, to, type, conversationId,
  correlationId?, ts, seq, body}`, `type ∈ {question, response, feedback, directive, task_mutation}`. A
  `question` carries a `correlationId`; the `response` echoes it; `feedback`/`directive` are one-way.
  `seq` is assigned by the transport, not the sender.
- **Re-planning loop.** A worker raises `question` / `feedback` / `task_mutation` → the orchestrator
  **regenerates the remaining plan** (it never diffs), commits the node add/split/reprioritize, and
  sends `directive`s. The **escalation cap is the single binding convergence guard**: retry → local
  patch → full replan, then **escalate to a blocker after K=3** attempts on a unit. This *is*
  `core/completion`'s circuit breaker — there is no separate timer or loop-detector; an unanswered
  `question` left `blocked` after K cycles is swept to a blocker.
- **Verification gate.** "Done" = acceptance criteria met + build/tests green, read from the **durable
  ledger** (`core/completion`), never from chat memory. The same gate governs which work distils into
  `core/memory`.

That role model + dispositions + escalation cap + verification gate is the entire portable surface.
What's portable is the **contract**, not the wire format.

## Realization A — detritus, in-process (native orchestration + the durable ledger; no bus)

In `/smith` and `/forge` the orchestrator is the main loop and workers are Agent-tool sub-agents (or
Workflow stages) in the same session. **This realization ships as skill-doc conventions over native
orchestration — no new Go, no daemon, no ooo bus, no comms tools.**

- **Transport is the Agent tool itself.** A sub-agent has an isolated context, receives exactly the
  slice the orchestrator hands it, and returns a final message the orchestrator reads. That *is*
  single-writer message-passing — no store, no inbox, no polling. (The detritus MCP is per-session stdio
  and cannot host a durable shared server, so an in-process bus is not even an option, and it would add a
  per-turn polling tax for no gain.)
- **The task-graph is the durable file ledger** from `core/completion`: the `.plan/<slug>.md` (or
  `.smith/<slug>.md`) acceptance checklist + git. The orchestrator sheds state to the ledger and
  re-derives only the open `[ ]` items each tick.
- **Worker-asks-orchestrator = return-and-respawn.** A worker that cannot proceed ends with a structured
  final line — a fenced `BLOCKED {json: {question, correlationId}}` — mirroring the `PARTITION` line
  convention so the orchestrator greps one shape:

  ```
  BLOCKED {"question":"interface X is undefined in my boundary — provide it or widen scope?","correlationId":"task-export-3"}
  ```

  The orchestrator answers, mutates the ledger if needed, and **re-spawns** the worker with the answer +
  its updated slice. (The base Agent tool has no mid-task resume, so respawn is the mechanism — the
  in-process realization of an interrupt→resume.)
- **Enforcement** is the loop itself: it re-derives the open items and continues until the gate is green
  (`core/completion`'s exit gate). No Stop hook required. The **K=3** escalation cap escalates a stuck
  unit to a blocker rather than thrashing quota.
- **Artifact hand-off is the filesystem / git**, not messages.

## Realization B — candyland, multi-process (the bus on the conductor)

In candyland the orchestrator is the long-lived conductor process; the tech-lead and coders are separate
`claude -p` processes that cannot see each other, so coordination rides an **ooo bus** on the conductor
(inboxes, an append-only graph-events log, orchestrator-only `graph/nodes/*`, a `WriteFilter` for
seq+auth and an `AfterWriteFilter` to react), beside the existing stdout loop. **Realization B is built
in the candyland repo (its own PR), not here** — detritus ships the protocol + Realization A; candyland
consumes the same contract.

## What this doc is not

- Not a slash command — `core/coordination` is `kb_get`-only.
- Not the completion doctrine (`core/completion`) — it reuses the ledger, gate, and K=3 cap defined there.
- Not the partition mechanics (`roles/tech-lead`) or coder contract (`core/coder`) — those compose this.

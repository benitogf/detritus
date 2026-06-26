---
description: Shared coder contract for implementation-loop role agents (test-engineer, backend, frontend, fullstack). Do not invoke directly — composed by roles/* under a tech-lead.
triggers:
  - coder core
  - coder contract
  - role agent
  - parallel coder
when: Internal. Loaded by an implementation-loop role agent (roles/coder-*) spawned by a tech-lead, to define how a single coder works one partitioned task in isolation.
related:
  - core/build
  - core/completion
  - core/coordination
  - roles/tech-lead
  - roles/coder-test-engineer
  - roles/coder-backend
  - roles/coder-frontend
  - roles/coder-fullstack
  - core/todo-audit
---

# Coder Core — one task, one worktree, one green test

Shared contract for every implementation-loop coder role. A coder is **not** a planner and **not** an integrator: it takes a single partitioned task and drives it to green inside an isolated worktree. The role-specific docs (`roles/coder-test-engineer`, `roles/coder-backend`, `roles/coder-frontend`, `roles/coder-fullstack`) compose this core and add only their domain delta.

> ## ⛔ Do not invoke directly
> This is an internal building block with no slash command. It is loaded via `kb_get` by a role agent that a tech-lead (`roles/tech-lead`) has spawned — either as an in-process sub-agent under `/forge` or as a candyland-launched process. A coder never owns the loop, never spawns other agents, and never opens a PR.

## Inputs a coder receives

The tech-lead hands each coder exactly four things — never the whole plan:

- **One task** — a single acceptance item or a slice of one, with its defining failing test named.
- **Only the context it depends on** — the files in its partition plus the interfaces/signatures it must call. Not the other coders' work.
- **A fork-safe boundary** — the exact set of files/modules this coder may touch (disjoint from every other coder; partition rules in `core/todo-audit` → fork-safe gates). A `Fullstack` task's boundary spans both server and client files, but is still disjoint from every other task.
- **A worktree** — an isolated git worktree so parallel coders never collide.

## The contract

- **TDD gate.** The task is defined by a failing test (written by the test-engineer role). "Done" means that test goes green and the canonical verification command passes — the same gate `core/build` enforces for a single build unit. A coder does not declare done on a red or unrun test.
- **Stay inside the partition.** Touch only the files in the assigned boundary. A change that needs a file outside the boundary is a **blocker to report**, not a license to reach across — reaching across is exactly the cross-dependency the fork-safe partition exists to prevent.
- **Smallest delta to green.** Implement the task, not adjacent improvements. Out-of-scope needs are reported, never folded in (see *Out-of-scope work* below).
- **No integration.** A coder never merges branches, resolves cross-task conflicts, or opens a PR. Integration is the tech-lead's sequential step.
- **Emit status, don't narrate.** A coder's output is a structured status the driver consumes (the `/forge` sub-agent return value, or a candyland event): `working` → `green` (task done, test + verification green, files changed) or `blocked` (with the precise evidence — failing assertion, missing interface, out-of-boundary dependency). In-process, a block ends the turn with `core/coordination`'s fenced `BLOCKED {json: {question, correlationId}}` line so the orchestrator can answer and re-spawn. Status is the only product; raw logs stay out of the user's thread.

## Out-of-scope work

A coder reports out-of-scope needs or risky adjacent code to the tech-lead as part of its `blocked`/status output — it never acts on them or folds them in. Disposition is the tech-lead's call per `core/completion`'s three dispositions (`roles/tech-lead` → *Dispositions*): in-scope work is done now, a genuinely separate feature becomes a feature-split, a hard blocker is surfaced. A coder never converts an in-scope need into a deferral.

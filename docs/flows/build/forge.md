---
description: Drive a settled plan to a PR with a parallel tech-lead + coders implementation loop, in-process. Consumes a .plan/<slug>.md contract from /plan or /dream; spawns coders as sub-agents. The plan-first, multi-agent sibling of /smith.
argument-hint: "[plan slug or .plan/<slug>.md path]"
triggers:
  - forge
  - implement the plan
  - build the plan
  - parallel build
  - run the implementation loop
when: User has a settled plan (from /plan or /dream) and wants it implemented by a parallel tech-lead + coder loop in this session, without an external conductor.
related:
  - roles/tech-lead
  - core/build
  - core/completion
  - core/coordination
  - core/coder
  - flows/plan/plan
  - core/dream
  - flows/build/smith
  - flows/build/janitor
---

# /forge — In-Process Implementation Loop

`/forge` takes an **already-settled plan** and drives it to a single PR using the parallel implementation loop: a tech-lead partitions the work into fork-safe tasks, a test-engineer defines each with a failing test, backend/frontend coders build them concurrently, and the tech-lead integrates and delivers. `/forge` does **not** plan — it consumes a plan contract produced by `/plan` (developer) or `/dream` (executive intake, via `/vibe`).

`/forge` is a **thin driver**: it composes `roles/tech-lead` (decisions + choreography), `core/build` (build unit + delivery), and `core/coder` (coder behavior). It restates none of them — when those tighten, `/forge` inherits it.

## Input — the plan contract

Resolve the plan to implement, in order:

1. The argument, if given — a slug or a `.plan/<slug>.md` path.
2. Otherwise the most recent `.plan/*.md` in the workspace.
3. If none exists, stop and say so: `/forge` needs a settled plan. Point the user at `/plan` (developer) or `/vibe` (non-technical) first.

The contract (shape in `flows/plan/plan`) carries the feature spec, acceptance criteria checklist, user-stated rules, decisions made on the user's behalf, and any feature-splits/blockers (`core/completion` dispositions — not a parking lot for in-scope work). It is the build-phase source of truth.

## Execution — be the tech-lead, in-process

Act as the tech-lead per `roles/tech-lead`, with the **in-process driver substrate**: spawn each coder as a **sub-agent via the Agent tool**, one per fork-safe task, each told only its single task and the context it depends on. This is `core/coordination` Realization A — the Agent tool *is* the single-writer transport, the `.plan` checklist is the durable task-graph, and a blocked coder returns the fenced `BLOCKED {json}` line for you to answer and re-spawn (no bus). Drive the full choreography — partition → test-first → parallel build → sequential integration (loop back to the owning coder on a dirty merge) → `/gh-self-review` convergence → one PR via `gh-issue-work` Phase 9.

## How /forge relates to /smith and candyland

- **`/smith`** — single-threaded and *fused*: it runs `/plan` first, then builds sequentially in one agent, then audits. Use `/smith` when you want one all-in-one loop from a description.
- **`/forge`** — *plan-first and parallel*: a plan already exists; multiple coders build it concurrently under a tech-lead. Use `/forge` to implement a settled plan fast with role agents.
- **candyland** — the **out-of-process** driver over the *same* `roles/tech-lead` + `core/*`. It spawns the tech-lead and coders as processes it monitors, controls, and visualizes. candyland never calls `/forge`; `/forge` is the way to run the same loop directly when you don't need a dashboard.

## Visibility trade-off

`/forge`'s coders are in-process sub-agents, so only **this terminal** sees them — there is no external DAG, no per-agent pause/kill, no live dashboard. That is the deliberate trade for running solo. When you need control and visibility over the running agents, that is candyland's job, not `/forge`'s.

## Boundaries

- Never plan inside `/forge` — consume the contract; if it's missing or ambiguous, stop and route to `/plan` or `/vibe`.
- Completion (the exit gate — every acceptance box green, verification green, clean self-review, no new deferral markers) and disposition of out-of-scope work follow `core/completion`, inherited via `roles/tech-lead` and `core/build` — not restated here. In-scope work is done now; only a genuinely separate feature or a hard blocker surfaces for the developer to triage.
- Don't reimplement `roles/tech-lead`, `core/build`, or PR creation — compose them.
- Deliver one PR for the plan; merging and anything irreversible stay the human's call.

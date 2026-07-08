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
  - core/flows
  - core/loop
  - roles/tech-lead
  - core/build
  - core/completion
  - core/coordination
  - core/coder
  - flows/plan/plan
  - core/dream
  - flows/build/smith
  - flows/build/janitor
  - flows/github/babysit
  - flows/maintainer/grow
  - flows/maintainer/learn
  - flows/maintainer/absorb
  - roles/reviewer
---

# /forge — In-Process Implementation Loop

`/forge` takes an **already-settled plan** and drives it to one PR per impacted repo using the parallel implementation loop: a tech-lead partitions the work into fork-safe tasks, a test-engineer defines each with a failing test, backend/frontend/fullstack coders build them concurrently, and the tech-lead integrates and delivers. `/forge` does **not** plan — it consumes a plan contract produced by `/plan` (developer) or `/dream` (executive intake, via `/vibe`).

`/forge` is a **thin driver**: it composes `roles/tech-lead` (decisions + choreography), `core/build` (build unit + delivery), and `core/coder` (coder behavior). It restates none of them — when those tighten, `/forge` inherits it.

## Input — the plan contract

Resolve the plan to implement, in order:

1. The argument, if given — a slug or a `.plan/<slug>.md` path.
2. Otherwise the most recent `.plan/*.md` in the workspace.
3. If none exists, stop and say so: `/forge` needs a settled plan. Point the user at `/plan` (developer) or `/vibe` (non-technical) first.

The contract (shape in `flows/plan/plan`) carries the feature spec, acceptance criteria checklist, user-stated rules, decisions made on the user's behalf, and any feature-splits/blockers (`core/completion` dispositions — not a parking lot for in-scope work). It is the build-phase source of truth.

## Execution — be the tech-lead, in-process

Act as the tech-lead per `roles/tech-lead`, with the **in-process driver substrate**: spawn each coder as a **sub-agent via the Agent tool with `subagent_type: detritus-coder`**, one per fork-safe task, each told only its single task and the context it depends on. That named subagent is the definition `detritus --setup` installs at `~/.claude/agents/detritus-coder.md`; spawning it (never a bare, unnamed Agent-tool sub-agent) is what activates its `effort: low` override, keeping the wide coder fan-out cheap while this tech-lead session runs at the session effort. If the definition is absent (setup not run), fall back to a generic Agent-tool sub-agent so the loop still runs. The `/gh-self-review` convergence spawns the same-named **`detritus-reviewer`** definition (pinned model+effort per `roles/reviewer`), with the same fall-back-to-generic rule when setup hasn't installed it, and receives the plan contract as its driving intent. This is `core/coordination` Realization A — the Agent tool *is* the single-writer transport, the `.plan` checklist is the durable task-graph, and a blocked coder returns the fenced `BLOCKED {json}` line for you to answer and re-spawn (no bus). Drive the full choreography — partition → test-first → parallel build → sequential integration (loop back to the owning coder on a dirty merge) → `/gh-self-review` convergence → one PR per impacted repo via `gh-issue-work` Phase 9 (`core/build` → *Multi-repo delivery*).

Size the work before partitioning with `code_graph`: `impacted_by` gives the blast radius of the symbols each task touches (feeding fork-safe boundaries) and `affected_tests` names the tests that must pass — surface both up front so every task and the `/gh-self-review` convergence carry their gating tests into the review, before delivery.

**The forge decision rule.** Post-plan, a *decision* — ambiguity, a trade-off, scope interpretation, an unclear root cause, unexpected difficulty — **never reaches the user**. A coder that hits one emits the fenced `BLOCKED {json}` line; **the tech-lead (this session) decides** and re-spawns that coder with the answer rendered into its brief. The tech-lead's own fallback, when nothing above it can decide, is the **smith rule**: decide the best option given context and **record it** in the PROGRESS ledger's *Decisions made autonomously* section (never surface it, never stall). The only thing that legitimately reaches terminal `blocked` is a **capability blocker** — a failure no decision can resolve (missing credentials, absent permissions, unreachable infrastructure, a toolchain broken *outside* the repo) — surfaced with its postmortem (`core/completion` → disposition 3, capability-failures only; a decision is never disposition 3). `blocked` is a last breath, not a shortcut to skip work or dodge root-cause analysis.

✅ Coder emits `BLOCKED {"question":"which retry backoff?"}` → tech-lead picks exponential (matches the sibling client), re-spawns with it rendered into the brief, records the call in the ledger.
❌ Coder's ambiguity is relayed to the user as a question, pausing the loop.

## The progress ledger — .plan/PROGRESS-&lt;slug&gt;.md

`/forge` maintains a durable progress ledger at `.plan/PROGRESS-<slug>.md`: the fork-safe **task table** (task, owner, deps, status), an append-only **tick log**, a **state block** (in-flight coders, integration status, next move, blockers/feature-splits), and a **Decisions made autonomously** section. It is **overwritten at checkpoints** per `core/loop` → *Checkpoint-then-/clear*: after each checkpoint the ledger + git + the `.plan/<slug>.md` contract are the complete resume state, so the tech-lead session itself can be `/clear`'d and a fresh context resumes from them — the tick report ends with `checkpoint complete — safe to /clear` at the heavy boundaries `core/loop` names. Never rely on `/compact`.

### Decisions made autonomously

An append-only ledger section recording every *decision* the tech-lead resolved on its own — whether a coder escalated it via `BLOCKED {json}` or the tech-lead hit it directly (per *The forge decision rule*). Each entry:

- **what** — the choice made, in one line (name the escalating coder if it came up via `BLOCKED`).
- **why** — the reasoning that selected it given the current context.
- **evidence** — the file:line / test output / spec clause / contract term that grounds the call.
- **alternatives rejected** — the other options considered and why each lost.

These are **recorded here, not sent to the user** — the ledger is the audit trail. Only a capability blocker (with its postmortem) ever surfaces.

## How /forge relates to /smith and /candyland

- **`/forge`** — the **in-session run** (`core/flows`): the session acts as tech-lead, coders are Agent-tool sub-agents building a settled plan concurrently.
- **`/candyland`** — the **sidecar homologue**: it launches the *same* choreography (tech-lead + coders + reviewer) as out-of-process agents over the candyland bus, watched in a dashboard. Same pipeline, different execution substrate.
- **`/smith`** — the **single-agent sibling**: same pipeline with no spawning, all work visible in the main session; it runs `/plan` first and builds sequentially.

## Visibility trade-off

`/forge`'s coders are in-process sub-agents, so only **this terminal** sees them — there is no external DAG, no per-agent pause/kill, no live dashboard. That is the deliberate trade for running solo. When you need control and visibility over the running agents, that is candyland's job, not `/forge`'s.

## Boundaries

- Never plan inside `/forge` — consume the contract; if it's missing or ambiguous, stop and route to `/plan` or `/vibe`.
- Completion (the exit gate — every acceptance box green, verification green, clean self-review, no new deferral markers, zero units left `blocked`) and disposition of out-of-scope work follow `core/completion`, inherited via `roles/tech-lead` and `core/build` — not restated here. In-scope work is done now; only a genuinely separate feature (disposition 2) or a capability blocker (disposition 3, with its postmortem) surfaces for the developer to triage — never a decision, which the tech-lead resolves per *The forge decision rule*.
- Don't reimplement `roles/tech-lead`, `core/build`, or PR creation — compose them.
- Deliver one PR per impacted repo for the plan; merging and anything irreversible stay the human's call. When the PR(s) open, **offer** `/babysit <pr>` (`flows/github/babysit`) as the optional watch-to-merge continuation (`core/flows` → *PR-watch — the universal terminal phase*) — each PR watched to merge on a SHA-pinned human approval. Offer it; don't auto-start it (an unbidden watch loop is scope the plan didn't grant).

## Incident hook — capture the lesson after delivery

A detected failure, misalignment, **self-acknowledged mistake/doctrine violation**, or user correction during the loop is a learning signal, but it never preempts the primary deliverable: **finish the PR(s) first — never trade the deliverable for the lesson.** Detection (including the agent's own acknowledgment: "you are right, I …", "I didn't follow …", "I ignored /…") and routing are canonical in `core/ego`: user correction/self-acknowledgment → `/grow`, a PR blocker (a gate miss) → `/absorb`, telemetry → `/learn`.

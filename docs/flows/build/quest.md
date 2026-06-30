---
description: A Candyland-native iterative build loop — the out-of-process, multi-PR homologue of /janitor. Settles a loop intent (objective, scope, safety boundary, verification command), ensures the candyland sidecar is up, then drives a quest over REST; the quest ticks discover→triage→run→review→PR and may open many PRs over time, watched in the dashboard.
argument-hint: "[objective] [folder ...]"
triggers:
  - quest
  - candyland quest
  - out-of-process loop
  - iterative build in the sidecar
  - long-running quest
when: User wants a recurring iterative build/maintenance loop that runs out-of-process in the candyland sidecar (watched in a dashboard, opening its own PRs over time) instead of an in-session /janitor loop that consumes this conversation.
related:
  - flows/build/janitor
  - flows/build/forge
  - flows/build/candyland
  - core/loop
  - core/todo-audit
  - core/completion
  - roles/tech-lead
---

# /quest — a Candyland-native iterative loop

`/quest` is the **out-of-process, multi-PR sibling of `/janitor`**. Where `/janitor` runs the discover→triage→fix→verify→deliver loop **in this session** (consuming the conversation, one delivery at a time through `/gh`), `/quest` hands the same loop to **candyland**, which ticks it as a standalone process tree over the ooo bus, watched live in the dashboard. A quest is long-running: it keeps ticking and may open **many PRs over time** — one per impacted repo, per tick that produces shippable work.

`/quest` is the **generalized** form of that loop. `/janitor`'s lens is safe maintenance; a quest carries an explicit **objective** plus its scope, safety boundary, and verification command, so it can drive feature-shaped iterative work as well as maintenance — always within the boundary it was given.

## Settle the loop intent

Before launching, read the detritus KB guidance for iterative loops (`core/loop` for the loop spine — durable state, cadence, skip-streak guardrail, non-overlap; `core/todo-audit` for triage/partition; `core/completion` for the three dispositions a tick obeys) and refine the loop intent into four things:

- **Objective** — what the loop is driving toward, in plain language. This is the quest's north star, written to the objective file passed on argv (keeps a large objective off the command line, mirroring how `/candyland` passes the plan file).
- **Scope** — which repos/areas the quest may touch. Each folder is a **candidate repo**: candyland branches and opens a PR in **each folder that receives changes**.
- **Safety boundary** — what the quest may and may not do (inheriting `core/completion`: in-scope work is done now, separate features become feature-splits, hard blockers are surfaced — never silently deferred).
- **Verification command** — the canonical green gate every tick must pass before it ships, the same gate `core/build` enforces.

`/quest` never invents the objective — it refines what the user asked for into a loop intent candyland's conductor can tick.

## Steps

1. **Settle the loop intent** (objective, scope, safety boundary, verification command) per the section above, and write the objective to an objective file.
2. **Ensure candyland is up, then start the quest over REST.** Run `detritus --quest-run <objective-file> [folder ...]` (folders default to the cwd). detritus health-checks the sidecar, starts it if down, then `POST`s `/api/quests` with `{objective, folders, autonomyLevel, deliver}`, reads back the quest id, and `POST`s `/api/quests/{id}/begin` to start it. A quest started via `/quest` is **standalone** — it opens its own PRs — so it launches conservatively (`autonomyLevel: L1`, `deliver: pr`).
3. **Hand off to the dashboard.** Report the quest id and the dashboard URL — that is where the live tick state, the task graph, and the per-tick verification audit show, and where the quest is stopped. A quest ticks **discover→triage→run→review→PR** and may open **more than one PR over time** (one per impacted repo, per shippable tick); a single repo's delivery failure is surfaced without failing the others.

## Control (stop only)

Like `/candyland`, a quest is lean: **observe + audit + stop**, no per-agent control, no resume. Halt a wrong or runaway quest from the dashboard's Stop — candyland owns the spawned process tree, so it genuinely kills the conductor + coders. Watch live state in the dashboard rather than polling.

## How /quest relates to the other loops and flows

- **`/janitor`** — the **in-session** variant of the same iterative loop: discover→triage→fix→verify→deliver, one delivery at a time through `/gh`, consuming this conversation. `/quest` is its **out-of-process, multi-PR** sibling.
- **`/forge`** / **`/candyland`** — a **one-shot** plan-to-PR build of a settled `.plan/<slug>.md` (in-process for `/forge`, in the sidecar for `/candyland`). A quest is **not** one-shot: it carries an objective and keeps ticking, opening PRs over time.
- **candyland (the app)** — the out-of-process driver detritus hands the quest to over REST: it owns the `/api/quests` contract, ticks the loop as a process tree over the ooo bus (`core/coordination`), and visualizes it. detritus is only the client/launcher — it ensures the sidecar is up and starts the quest; candyland owns everything after.

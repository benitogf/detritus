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
2. **Ensure candyland is up, then start the quest over REST.** Run `detritus --quest-run <objective-file> [folder ...]` (folders default to the cwd). detritus health-checks the sidecar, starts it if down, then `POST`s `/api/quests` with `{objective, folders, autonomyLevel, deliver}`, reads back the quest id, and `POST`s `/api/quests/{id}/begin` to start it. **Autonomy must match the invocation verb** (see below); `deliver: pr` is the standing safety rail — the quest opens its own PRs and **never merges**.
3. **Hand off to the dashboard.** Report the quest id and the dashboard URL — that is where the live tick state, the task graph, and the per-tick verification audit show, and where the quest is stopped. A quest ticks **discover→triage→run→review→PR** and may open **more than one PR over time** (one per impacted repo, per shippable tick); a single repo's delivery failure is surfaced without failing the others.

## Autonomy matches the objective's intent

The autonomy level decides whether a tick **acts** or merely **looks**:

- **Report-only / surface-only** (L1) — the quest discovers and triages but makes **no code changes and opens no PRs**; it only surfaces findings. Appropriate for an audit-shaped objective ("review", "find", "report on").
- **Executing** — the quest carries work through to changes and PRs within its safety boundary. Required for an execute-shaped objective.

`/quest` is almost always invoked with an **execute-shaped** objective ("solve", "fix", "wire", "update PR #N"). The launcher **derives** autonomy from the objective's intent — an execute verb selects an executing level — or **prompts once at launch** when the intent is ambiguous. It never silently defaults an execute-shaped objective to a report-only level.

If a report-only level **does** end up the default for an execute-shaped objective, the flow says so **loudly, before** "quest started" — never launches report-only against an execute verb without warning. Likewise, a **first-and-only tick that skips 100%** of an execute objective is a **misconfig signal to surface**, not a finish to report green (`core/completion`: a green terminal state must be earned, not a no-op).

## Delivery mode matches the objective's intent

Pairing with the autonomy-from-intent rule above, the launcher also **detects the delivery mode** from the objective and sets `deliver` accordingly (`core/build` → *Delivery modes*: `pr|branch|feedback|review`):

- An **"address feedback on PR #N"** intent ("fix the review on PR #N", "update PR #N") → `deliver: feedback`, with the **target PR(s)** passed through. The quest updates **that existing PR in place** (commit onto its head branch, push) and **never opens a new PR** — mirroring `/gh-feedback-work`. Multi-repo feedback lands each repo's fix on that repo's existing PR.
- A **"review PR #N"** intent ("check PR #N against the requirements doc", "review #N") → `deliver: review`. The quest may end with **no PR at all** when nothing is actionable; any actionable finding becomes `feedback` work on the PR in question. A no-PR `review` terminal is valid `done`, not a no-op (`core/completion` → *Honest terminal state*, carve-out 3).
- A plain new-work objective → `deliver: pr` (the standing default — new PR per impacted repo).

The launcher **never** opens a duplicate PR for a feedback/review intent, and **never** silently defaults a feedback/review objective to `pr`. The intent→delivery-mode detection is the same shape as the intent→autonomy detection above: derived from the verb, or prompted once when ambiguous.

## Launch output

The launch prints, before handing off, so the user can tell what they actually got:

- **quest id** and the **dashboard URL**;
- **autonomy level** and **deliver mode**, plus a one-line **what this will / won't do** (e.g. `L1: surfaces findings only — no code changes, no PRs`);
- the **correct ports**: API on **:8888**, UI on **:8080**. (The UI loads from :8080 but reads its data from the API on :8888 — printing only the UI URL leaves the dashboard blank.) On a remote/WSL host, forward **both** ports.

## Control (stop only)

Like `/candyland`, a quest is lean: **observe + audit + stop**, no per-agent control, no resume. Halt a wrong or runaway quest from the dashboard's Stop — candyland owns the spawned process tree, so it genuinely kills the conductor + coders. Watch live state in the dashboard rather than polling.

## How /quest relates to the other loops and flows

- **`/janitor`** — the **in-session** variant of the same iterative loop: discover→triage→fix→verify→deliver, one delivery at a time through `/gh`, consuming this conversation. `/quest` is its **out-of-process, multi-PR** sibling.
- **`/forge`** / **`/candyland`** — a **one-shot** plan-to-PR build of a settled `.plan/<slug>.md` (in-process for `/forge`, in the sidecar for `/candyland`). A quest is **not** one-shot: it carries an objective and keeps ticking, opening PRs over time.
- **candyland (the app)** — the out-of-process driver detritus hands the quest to over REST: it owns the `/api/quests` contract, ticks the loop as a process tree over the ooo bus (`core/coordination`), and visualizes it. detritus is only the client/launcher — it ensures the sidecar is up and starts the quest; candyland owns everything after.

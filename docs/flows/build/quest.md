---
description: A bounded, terminating iterative loop in the candyland sidecar — a quest-lead ticks discover→triage→launch child runs onto a quest branch until the objective is met, then opens ONE PR per impacted repo. Feedback/review intents work the target PR's head branch instead.
argument-hint: "[objective] [folder ...]"
triggers:
  - quest
  - candyland quest
  - bounded loop in the sidecar
  - iterative build in the sidecar
when: User wants a bounded, terminating iterative loop driven out-of-process in the candyland sidecar — discovery-driven work toward a settled objective, converging to one PR per impacted repo, watched in the dashboard.
related:
  - core/flows
  - core/sidecar
  - flows/build/adventure
  - flows/build/campaign
  - flows/build/smith
  - flows/build/forge
  - core/loop
  - core/build
  - core/completion
  - flows/github/babysit
  - flows/maintainer/grow
  - flows/maintainer/learn
  - flows/maintainer/absorb
---

# /quest — a bounded iterative loop in the sidecar

A quest is a **bounded, terminating** loop: the **quest-lead** ticks **discover → triage → launch child runs**; child runs commit onto the quest branch (`quest/<id>`, `deliver: branch`); when the objective is met, the quest opens **ONE PR per impacted repo** — reusing the campaign delivery shape. It converges rather than opening PRs as it goes (`core/flows` → *PR policy*). Its in-session bounded homologues are `/smith` (single agent) and `/forge` (parallel); its open-ended sibling is `/adventure`.

## Settle the loop intent

Refine the request into four things, written to the objective file passed on argv:

- **Objective** — what the loop is driving toward; the terminating condition.
- **Scope** — which repos/areas it may touch (the folders; every folder is a candidate repo).
- **Safety boundary** — what it may and may not do (`core/completion`'s dispositions — never silent deferral).
- **Verification command** — the green gate every child run must pass (`core/build`).

`/quest` never invents the objective — it refines what the user asked for into a loop intent the quest-lead can tick (`core/loop` for the loop spine).

## Steps

1. **Settle the loop intent** and write the objective file. An input that references a PR/issue is classified per the gh-mirror table (`core/sidecar` → *PR-link intake mirrors /gh*).
2. **Launch**: `detritus --quest-run <objective-file> [folder ...]` — ensure-up, REST create + begin (`core/sidecar`).
3. **Hand off to the dashboard**, printing the launch output contract (`core/sidecar`).

## Delivery

- **Standalone quest** — child runs accumulate on `quest/<id>`; the terminal opens one PR per impacted repo.
- **feedback/review intent (target PR)** — the quest works **that PR's head branch**: no quest branch, never a new PR (mirroring `/gh-feedback-work`). A review quest reviews the target PR before any "no findings" terminal — the review IS its work.
- **Campaign-child quest** — integrates onto the **campaign branch** and opens **no PR**; the campaign delivers (`flows/build/campaign`).

Triage never surfaces the quest's **own delivery artifacts** (its branch, its open PRs) as new work items.

**Watch-to-merge is in-session, never in the sidecar** (`core/sidecar` → *Watch-to-merge is in-session, never in the sidecar*). A standalone quest's per-repo PRs are handed to `/babysit <pr>` (`flows/github/babysit`) in the user's session to reach merge on a SHA-pinned human approval — the sidecar itself never merges. (A campaign-child quest opens no PR; the campaign delivers and its PRs are watched the same way.)

## Incident hook — capture the lesson after delivery

A detected failure, misalignment, **self-acknowledged mistake/doctrine violation**, or user correction during a quest is a learning signal, but it never preempts the primary deliverable: **finish the quest's PR(s) first, and capture is in-session, never in the sidecar (like watch-to-merge above) — never trade the deliverable for the lesson.** Detection (including the agent's own acknowledgment: "you are right, I …", "I didn't follow …", "I ignored /…") and routing are canonical in `core/ego`: user correction/self-acknowledgment → `/grow`, a PR blocker (a gate miss) → `/absorb`, telemetry → `/learn`. Route in the user's session post-delivery — never in the sidecar.

## Control

Observe + stop only, per `core/sidecar` — Stop kills the quest-lead + child-run tree.

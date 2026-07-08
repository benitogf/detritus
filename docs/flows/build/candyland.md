---
description: Launch ONE candyland run in the sidecar — a tech-lead agent partitions the work, coders build it concurrently in worktrees, a reviewer loops fix→re-review until clean, then the run delivers. Plan-gated by input shape - a settled .plan launches directly, a PR/issue link is classified per /gh, a vague description runs dream intake first.
argument-hint: "[.plan/<slug>.md | PR/issue link | feature description] [folder ...]"
triggers:
  - candyland
  - sidecar run
  - build in the sidecar
  - watch the build
when: User wants one unit of work built autonomously out-of-process in the candyland sidecar (watched in a dashboard) instead of consuming this session. The in-session homologue is /forge.
related:
  - core/flows
  - core/sidecar
  - roles/tech-lead
  - core/build
  - core/dream
  - flows/build/forge
  - flows/build/quest
  - flows/plan/plan
  - flows/github/babysit
  - flows/maintainer/grow
  - flows/maintainer/learn
  - flows/maintainer/absorb
---

# /candyland — one run in the sidecar

`/candyland` launches **one run** in the candyland sidecar: candyland's **tech-lead agent** partitions the settled plan into fork-safe tasks, **coders** build them concurrently in worktrees, a **reviewer** loops fix→re-review until clean, and the run **delivers** — the same pipeline every flow instantiates (`core/flows`). detritus only plans and launches (`core/sidecar`); candyland owns everything after. The in-session homologue is `/forge` (this session as tech-lead, coders as sub-agents, no dashboard).

The run's review→fix→re-review loop is the **atom of correctness at every level**: the same primitive gates run delivery, quest branch delivery, and campaign gate 2 — a level differs only in its (diff, task intent, root intent, remediation rungs). Campaign **intent review remains the completeness judge** (the reviewer at a gate judges contradiction, not union-completeness — `core/review-rigor` → Two briefing layers). Remediation is **cost-ordered**: a citable blocker runs a fix pass in the loop, a structural finding relaunches a child run, an unmet commitment spawns a remediation quest, and only exhaustion escalates.

## Plan-gated intake — by input shape

`/candyland` is gated on **knowing what will be done** before the handover. The input decides intake:

- **A settled `.plan/<slug>.md`** (or its slug) → launch directly.
- **A PR/issue link or `#N`** → classify per the gh-mirror table (`core/sidecar` → *PR-link intake mirrors /gh*); the classification decides `deliver`/`targetPr`. A feedback/review outcome works that PR's head branch and opens no new PR.
- **A vague/high-level description** → run the `dream` intake (`core/dream`) in-session first to settle a `.plan/<slug>.md`, then launch. `/candyland` never invents the plan.
- **No input at all** (no argument AND nothing in the conversation naming what to build) → **ask which plan; never infer the target from ambient IDE state** (an open editor file, a selection) — see `core/sidecar` → *No input is not an input — never launch on ambient context*.

## Steps

1. **Settle intake** per the input shape above.
2. **Launch**: `detritus --candyland-run <plan-file> [folder ...]` — ensure-up, REST create + begin; folders default to the cwd and every folder is a candidate repo (`core/sidecar`). Passing the file (not the text) keeps a large plan off argv.
3. **Hand off to the dashboard**, printing the launch output contract (`core/sidecar`): run id, dashboard URL, deliver mode, one-line what-it-will-do, both ports. The dashboard shows the live agents, task graph, and per-task verification audit.

## Delivery

A run is bounded and converges: **one PR per impacted repo** (`core/flows` → *PR policy*); a feedback/review run updates the target PR in place. A single repo's delivery failure is surfaced without failing the others.

**Watch-to-merge is in-session, never in the sidecar** (`core/sidecar` → *Watch-to-merge is in-session, never in the sidecar*). The sidecar never merges — merge is gated on a human review. Once the run's PR(s) open, point the user at `/babysit <pr>` (`flows/github/babysit`) to carry each to merge in their own session, on a SHA-pinned human approval.

## Incident hook — capture the lesson after delivery

A detected failure, misalignment, **self-acknowledged mistake/doctrine violation**, or user correction during a run is a learning signal, but it never preempts the primary deliverable: **finish the run's PR(s) first, and capture is in-session, never in the sidecar (like watch-to-merge above) — never trade the deliverable for the lesson.** Detection (including the agent's own acknowledgment: "you are right, I …", "I didn't follow …", "I ignored /…") and routing are canonical in `core/ego`: user correction/self-acknowledgment → `/grow`, a PR blocker (a gate miss) → `/absorb`, telemetry → `/learn`. Route in the user's session post-delivery — never in the sidecar.

## Control

Observe + stop only, per `core/sidecar` — Stop genuinely kills the tech-lead + coder tree.

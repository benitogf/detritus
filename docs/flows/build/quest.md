---
description: The one flexible, persistent iterative loop in the candyland sidecar — a quest-lead ticks discover→triage→launch serial child runs against an objective + scope lens, with two delivery modes (converge-until-clean → one PR per impacted repo; per-finding → one PR per accepted finding). Feedback/review intents work the target PR's head branch instead.
argument-hint: "[objective] [--per-finding] [folder ...]"
triggers:
  - quest
  - candyland quest
  - persistent loop in the sidecar
  - iterative build in the sidecar
  - per-finding
when: User wants a flexible, persistent iterative loop driven out-of-process in the candyland sidecar — discovery-driven work toward a settled objective under a scope lens, delivered either by converging to one PR per impacted repo or by opening one PR per accepted finding, watched in the dashboard.
related:
  - core/flows
  - core/sidecar
  - flows/build/candyland
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

# /quest — the one persistent iterative loop in the sidecar

A quest is the sidecar's **one flexible, persistent loop**: the **quest-lead** ticks **discover → triage → launch child runs** against an **objective + scope lens**, iterating until the objective is met. Child runs commit onto the quest branch (`quest/<id>`, `deliver: branch`); a single run (`/candyland`, `flows/build/candyland`) is the atom the quest launches. Its in-session homologues are `/smith` (single agent) and `/forge` (parallel).

## Child runs are serial — persistence and isolation, never parallelism

The quest holds **one child run in flight at a time**. Its justification is **persistence + isolation**, not parallelism: the loop survives across many ticks, and each child run builds in its own worktree/branch off the quest branch. A quest is never a way to fan work out concurrently — that is `/candyland`'s job (independent tasks on one repo, each in its own worktree). The quest's value is that it keeps going.

### Survives connection loss and usage limits

Because the quest is persistent, a spawned agent that dies on a **usage limit** or a **connection loss** does not fail the quest: the conductor pauses the affected child run with a truthful `PauseReason` and auto-resumes it in place when the window reopens (`core/sidecar` → *Auto-pause and resume*). Loop state — stage, gate, branch, accumulated commits — is durable across a sidecar bounce, so the quest picks up where it left off rather than restarting.

## Settle the loop intent

Refine the request into four things, written to the objective file passed on argv:

- **Objective** — what the loop is driving toward; the terminating condition.
- **Scope lens** — which repos/areas it may touch (the folders; every folder is a candidate repo), and the aspect it looks through.
- **Safety boundary** — what it may and may not do (`core/completion`'s dispositions — never silent deferral).
- **Verification command** — the green gate every child run must pass (`core/build`).

`/quest` never invents the objective — it refines what the user asked for into a loop intent the quest-lead can tick (`core/loop` for the loop spine).

## The triage rule — a missing prerequisite is a blocker, never re-derived

When triage finds that a work item depends on a prerequisite that is **missing** (an undecided design point, an absent artifact, an unmet dependency), that item is a **blocker** — it is surfaced as such, never silently re-derived or invented to keep the loop moving (`core/completion` — no silent deferral, no fabrication). The quest routes the blocker; it does not paper over the gap.

## Delivery modes

A quest delivers one of two ways, chosen at launch:

- **converge** (default) — the loop iterates until it is **clean** (no findings come back / the objective terminates), accumulating child-run commits on `quest/<id>`, then opens **ONE PR per impacted repo**. It converges rather than opening PRs as it goes (`core/flows` → *PR policy*).
- **per-finding** (`--per-finding`) — each **accepted finding** becomes its own child run and its own PR: **one PR per accepted finding**. Open-ended, useful for reviewing a repo under a scope lens and shipping fixes independently.

### feedback/review intent (target PR)

When the input references a PR with a feedback/review intent, the quest works **that PR's head branch**: no quest branch, never a new PR (mirroring `/gh-feedback-work`). A review quest reviews the target PR before any "no findings" terminal — the review IS its work. An input that references a PR/issue is classified per the gh-mirror table (`core/sidecar` → *PR-link intake mirrors /gh*).

Triage never surfaces the quest's **own delivery artifacts** (its branch, its open PRs) as new work items.

## Steps

1. **Settle the loop intent** and write the objective file.
2. **Launch**: `detritus --quest-run <objective-file> [--per-finding] [folder ...]` — ensure-up, REST create + begin (`core/sidecar`). `--per-finding` selects the per-finding delivery mode; without it the quest converges.
3. **Hand off to the dashboard**, printing the launch output contract (`core/sidecar`).

## Watch-to-merge

**Watch-to-merge is in-session, never in the sidecar** (`core/sidecar` → *Watch-to-merge is in-session, never in the sidecar*). A quest's PR(s) are handed to `/babysit <pr>` (`flows/github/babysit`) in the user's session to reach merge on a SHA-pinned human approval — the sidecar itself never merges.

## Incident hook — capture the lesson after delivery

A detected failure, misalignment, **self-acknowledged mistake/doctrine violation**, or user correction during a quest is a learning signal, but it never preempts the primary deliverable: **finish the quest's PR(s) first, and capture is in-session, never in the sidecar (like watch-to-merge above) — never trade the deliverable for the lesson.** Detection (including the agent's own acknowledgment: "you are right, I …", "I didn't follow …", "I ignored /…") and routing are canonical in `core/ego`: user correction/self-acknowledgment → `/grow`, a PR blocker (a gate miss) → `/absorb`, telemetry → `/learn`. Route in the user's session post-delivery — never in the sidecar.

## Control

Observe + stop only, per `core/sidecar` — Stop kills the quest-lead + child-run tree.

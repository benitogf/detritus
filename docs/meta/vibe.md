---
description: Executive intake for autonomous delivery. The user describes a requirement (any length); the agent treats them as a non-technical stakeholder, clarifies intent through quick multiple-choice questions while planning, owns every technical decision as the software architect, and once the plan is ready drives the work autonomously through /smith to an open PR — with no separate "go"/approval gate.
category: meta
triggers:
  - vibe
  - vibe code
  - vibe build
  - just build it
  - just build me
  - build me this
  - build this for me
when: A non-technical stakeholder describes a desired outcome and wants it planned (with minimal, multiple-choice clarification), built, and PR'd without being asked to make technical decisions or to give a separate approval.
related:
  - plan/index
  - meta/vibe-plan
  - meta/smith
  - meta/gh
  - meta/gh-issue-create
  - meta/gh-issue-work
  - meta/gh-self-review
  - meta/loop-core
  - meta/truthseeker
---

# /vibe — Executive Intake → Plan (multiple-choice) → Autonomous Build → PR

The user is a **non-technical stakeholder**. They describe a requirement — **any length**, from a sentence to several paragraphs — and that description is the spec. `/vibe` clarifies intent through quick **multiple-choice** questions while planning, owns every technical decision as the software architect, and once the plan is ready drives the work through `/smith` to an open PR — with **no separate "go"/approval gate** between a settled plan and the PR.

`/vibe` is a **planning intake plus an autonomous build**, not a way to skip planning. It **never starts building without a plan.** It composes `/vibe-plan` (executive-mode planning intake), `/smith` (build → PR → audit), and `/gh` (delivery) — it does not reimplement them. In one line: **`/vibe` = `/vibe-plan`, then `/smith` run autonomously.**

## Planning — run /vibe-plan

`/vibe`'s planning phase **is** `/vibe-plan` (`meta/vibe-plan`): executive, multiple-choice intake that refines the user's intent into a buildable, verifiable plan — feature spec, acceptance criteria, captured rules, and decisions made on the user's behalf — with the architect owning every technical decision. `/vibe` does **not** redefine that contract; it runs `/vibe-plan` to produce the settled plan, then proceeds to the build. The full intake rules (multiple-choice only, the architect owns the technical layer, the readiness and consistency checks, the restate-before-handoff step) live in `meta/vibe-plan`.

## Plan first, then autonomous — no separate go-gate

- `/vibe` **always runs `/vibe-plan` first**. The multiple-choice Q&A *is* the planning — it produces the settled scope, acceptance criteria, and the architect's technical decisions. `/vibe` never hands work to `/smith` without a ready plan.
- There is **no separate approval step.** The user does not say "go" / "proceed" / "approve." Once `/vibe-plan` has produced a concrete plan, `/vibe` proceeds to `/smith` automatically — the user's answers to the planning questions are the only interaction.
- **Aim at autonomy — especially after the build has started.** Once `/smith` is running, do not pause for clarification: the plan already resolved the unknowns. A question that arises mid-build is the architect's to decide and record (see *Decisions made on your behalf*), not the user's to answer.

## Execution — delegate to /smith

- Hand the settled plan from `/vibe-plan` to `/smith` as its captured **Feature spec** + **Acceptance criteria**, plus any **User-stated rules**, architect decisions, and hazards/deferred items identified during the readiness and consistency checks. Because `/vibe-plan` already did the planning, `/smith` does not re-run its interactive `/plan` pass — it proceeds straight to the build phase against the captured spec.
- Everything downstream is **inherited from `/smith` verbatim**, not reimplemented: the build phase, per-tick verification as a hard commit gate, the `/gh-self-review` convergence loop, and — at the *Build-to-Audit Transition* — **issue creation and PR opening** (`/smith` seeds a product-level issue via `/gh-issue-create` if none is linked, opens the PR via `/gh-issue-work` Phase 9, then runs the post-merge audit phase). `/vibe` does not front-run that and does not override `/gh-issue-create`'s confirmation gate — issue+PR creation happens inside `/smith`'s autonomous transition. When `/smith` or `/gh` tighten, `/vibe` inherits the tightening for free.
- **Precondition — durable runner.** `/smith`'s build phase requires a durable runner (`meta/smith` → *Build Phase Durability*). If only a disposable scheduler is available, `/smith`'s setup surfaces that and asks the user to pick a durable one — a one-time setup precondition before any build, not a mid-flow gate.
- **`/vibe`'s deliverable is an open PR.** It builds the change and opens the PR for human review; **merging, deploying, and any irreversible or external action stay outside `/vibe`'s scope** and remain the human's call. That boundary — not a mid-build pause — is the safety floor.
- **One vibe = one feature = one PR.** If the requirement is really several independent features, split into multiple `/smith` runs / PRs and say so when you restate — don't build a mega-PR.

## Decisions made on your behalf

`/vibe-plan` records every non-trivial architect decision as part of the plan (`meta/vibe-plan` → *Decisions made on your behalf*). `/vibe` routes those decisions into the **PR body** under a `## Decisions made on your behalf` section — what was chosen and the one-line why. This is the audit trail a technical reviewer (and a future maintainer) needs in place of the technical conversation the user didn't have. The issue body stays product-level (the requirement); the decisions live in the PR.

## Quality is not gated away

No approval gate ≠ no quality gate. `/vibe` inherits `/smith`'s **mandatory `/gh-self-review` convergence** before the PR opens (`meta/smith` → *Build-to-Audit Transition* step 1). That loop may run many iterations over a long time — **that is expected and is not a reason to stop or surface**; it runs to a clean read autonomously. The fresh-agent self-review, plus the human review of the resulting PR, are what make autonomous delivery safe.

## Guardrails

- **Planning is `/vibe-plan`'s job.** The intake rules (multiple-choice only, the architect owns the technical layer, the readiness/consistency checks) live in `meta/vibe-plan` — `/vibe` runs them, it does not restate or override them.
- **Never require a separate "go".** A ready plan (from `/vibe-plan`) is the authorization to build and open the PR.
- **Plan before building, always.** `/vibe` never hands work to `/smith` without a settled plan — planning is where ambiguity and scope get resolved, so they don't become mid-build stops.
- **Manage scope as the architect.** Keeping the build inside the planned scope is your job, not a reason to bail mid-build. If the requirement genuinely turns out larger than planned, that is a planning miss to fold back into the plan, not a silent expansion.
- **Never open the PR with a stale self-review** (inherited from `/smith` / `gh-issue-work` Phase 8a's convergence rule).
- **Don't reimplement** `/vibe-plan`, `/smith`, or `/gh` mechanics — compose them, so improvements propagate.
- **`/vibe` is for real features.** For a trivial change (a typo, a constant bump), `/gh-issue-work` or a direct edit is cheaper — don't spin up the full autonomous loop.
- **Record, don't narrate.** Technical decisions belong in the PR body's `## Decisions made on your behalf`, not scattered through chat the user won't read.

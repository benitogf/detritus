---
description: Executive intake for autonomous delivery. The user describes a requirement (any length); the agent treats them as a non-technical stakeholder, clarifies intent through quick multiple-choice questions while planning, owns every technical decision as the software architect, and once the plan is ready drives the work autonomously through /smith to an open PR — with no separate "go"/approval gate.
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
  - core/flows
  - flows/plan/plan
  - core/dream
  - flows/build/smith
  - flows/github/gh
  - flows/github/gh-issue-create
  - flows/github/gh-issue-work
  - flows/github/gh-self-review
  - flows/github/babysit
  - core/loop
  - core/completion
  - flows/principles/truthseeker
---

# /vibe — Executive Intake → Plan (multiple-choice) → Autonomous Build → PR

The user is a **non-technical stakeholder**. They describe a requirement — **any length**, from a sentence to several paragraphs — and that description is the spec. `/vibe` clarifies intent through quick **multiple-choice** questions while planning, owns every technical decision as the software architect, and once the plan is ready drives the work through `/smith` to an open PR — with **no separate "go"/approval gate** between a settled plan and the PR.

`/vibe` **IS the executive intake style** (`core/flows` → *Axes*): it is not a rung on the orchestration ladder but an intake fronting an executor — it runs `dream` to a settled plan, then fronts `/smith` autonomously. The same intake `/candyland` runs in-session when its input is vague. `/vibe` **never starts building without a plan.** It composes `dream` (executive-mode planning intake), `/smith` (build → open PR), and `/gh` (delivery) — it does not reimplement them. In one line: **`/vibe` = `dream`, then `/smith` run autonomously.**

## Planning — run dream

`/vibe`'s planning phase **is** `dream` (`core/dream`): executive, multiple-choice intake that refines the user's intent into a buildable, verifiable plan — feature spec, acceptance criteria, captured rules, and decisions made on the user's behalf — with the architect owning every technical decision. `/vibe` does **not** redefine that contract; it runs `dream` to produce the settled plan, then proceeds to the build. The full intake rules (multiple-choice only, the architect owns the technical layer, the readiness and consistency checks, the restate-before-handoff step) live in `core/dream`.

## Plan first, then autonomous — no separate go-gate

- `/vibe` **always runs `dream` first**. The multiple-choice Q&A *is* the planning — it produces the settled scope, acceptance criteria, and the architect's technical decisions. `/vibe` never hands work to `/smith` without a ready plan.
- There is **no separate approval step.** The user does not say "go" / "proceed" / "approve." Once `dream` has produced a concrete plan, `/vibe` proceeds to `/smith` automatically — the user's answers to the planning questions are the only interaction.
- **Stay autonomous — especially after the build has started.** Once `/smith` is running, do not pause for clarification: the plan already resolved the unknowns. A question that arises mid-build is the architect's to decide and record (see *Decisions made on your behalf*), not the user's to answer.

## Execution — delegate to /smith

Hand the settled plan from `dream` to `/smith` as its captured **Feature spec** + **Acceptance criteria**, plus any **User-stated rules**, architect decisions, and feature-splits/blockers (`core/completion` dispositions) from the readiness and consistency checks. Because `dream` already did the planning, `/smith` skips its interactive `/plan` pass and proceeds straight to the build phase. Everything downstream — build phase, verification gates, `/gh-self-review` convergence, issue creation and PR opening at *Delivery* — is **inherited from `/smith` verbatim** (`flows/build/smith`), not reimplemented; when `/smith` or `/gh` tighten, `/vibe` inherits the tightening for free.

**`/vibe`'s deliverable is a PR carried to merge-on-approval** — one per impacted repo (`core/build` → *Multi-repo delivery*, inherited via `/smith`). Once `/smith` opens the PR(s), `/vibe` **auto-chains into `/babysit`** (`flows/github/babysit`) — the pipeline's universal watch-to-merge phase (`core/flows` → *PR-watch — the universal terminal phase*) — per opened PR. This is the one flow that auto-starts the watch rather than offering it: the stakeholder is non-technical, so leaving a "run `/babysit` next" note they can't action would be a silent drop (`core/completion`'s no-deferral exit gate — the same rule that forbids parking a hazard). The watch loop folds in any reviewer feedback and merges **only** when a SHA-pinned human `APPROVED` review covers HEAD.

**The human review approval is still the gate.** `/vibe` never self-approves and never merges an unreviewed tree — auto-chaining `/babysit` automates the *mechanical* merge a human's approval authorizes, not the *decision* to merge. Deploying and any other irreversible/external action stay outside `/vibe`'s scope and remain the human's call; that boundary — not a mid-build pause — is the safety floor. One vibe = one feature: if the requirement is really several *independent* features, split into multiple `/smith` runs and say so when you restate.

## Decisions made on your behalf

`dream` records every non-trivial architect decision as part of the plan (`core/dream` → *Decisions made on your behalf*). `/vibe` routes those decisions into the **PR body** under a `## Decisions made on your behalf` section — what was chosen and the one-line why. This is the audit trail a technical reviewer (and a future maintainer) needs in place of the technical conversation the user didn't have. The issue body stays product-level (the requirement); the decisions live in the PR.

## Quality is not gated away

No approval gate ≠ no quality gate. `/vibe` inherits `/smith`'s **mandatory `/gh-self-review` convergence** before the PR opens (`flows/build/smith` → *Delivery* step 1); it runs to a clean read autonomously, however many iterations that takes. The fresh-agent self-review, plus the human review of the resulting PR, are what make autonomous delivery safe.

## Hazards are dealt with, never deferred

`/vibe` goes from *intent* to *a PR that does the thing* — no extra steps, no delegation, no deferral. This is `core/completion`'s no-deferral exit gate on the autonomous path: a non-technical stakeholder cannot action a "deferred" note, an auto-filed issue, or a "left for later" item, so producing one is a silent drop rather than a hand-off. Across both phases:

- **Planning (`dream`)** resolves in-scope hazards into the plan and splits genuinely separate features into separate plans — it never parks a hazard for the user (`core/dream` → *Hazards — deal with them, never defer*).
- **Build (`/smith`)** deals with anything that surfaces mid-build inside the PR, as the architect — never auto-files an issue, never defers, never asks the non-technical user to decide. The same rule binds the parallel loop (`roles/tech-lead` → *Dispositions*).

The only things that legitimately leave `/vibe`'s scope are the human's review-approval decision and any external action (deploy) — the human's call, not deferred work. The *mechanical* merge is not deferred: `/babysit` performs it in-flow the moment the human's approval lands (see *Execution*).

## Guardrails

- **Planning is `dream`'s job.** The intake rules (multiple-choice only, the architect owns the technical layer, the readiness/consistency checks) live in `core/dream` — `/vibe` runs them, it does not restate or override them.
- **Never require a separate "go".** A ready plan (from `dream`) is the authorization to build and open the PR.
- **Plan before building, always.** `/vibe` never hands work to `/smith` without a settled plan — planning is where ambiguity and scope get resolved, so they don't become mid-build stops.
- **Manage scope as the architect.** Keeping the build inside the planned scope is your job, not a reason to bail mid-build. If the requirement genuinely turns out larger than planned, that is a planning miss to fold back into the plan, not a silent expansion.
- **Don't reimplement** `dream`, `/smith`, `/gh`, or `/babysit` mechanics — compose them, so improvements propagate. The watch-to-merge gate (SHA-pinned human approval) lives in `/babysit`; `/vibe` chains it, never restates or relaxes it.
- **`/vibe` is for real features.** For a trivial change (a typo, a constant bump), `/gh-issue-work` or a direct edit is cheaper — don't spin up the full autonomous loop.
- **Record, don't narrate.** Technical decisions belong in the PR body's `## Decisions made on your behalf`, not scattered through chat the user won't read.

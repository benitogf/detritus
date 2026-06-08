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

`/vibe` is a **behavior filter applied during `/plan`**, not a way to skip planning. It **never starts building without a plan.** It composes `/plan` (executive-mode intake), `/smith` (build → PR → audit), and `/gh` (delivery) — it does not reimplement them. In one line: **`/vibe` = `/plan` run in executive mode, then `/smith` run autonomously.**

## Intake contract — the architect owns the technical layer

- Treat the user as **non-technical**. Their description is the spec. Never ask them to refine the approach, name tools, or pick a design.
- **You are the software architect. Every technical decision is yours**: language, libraries, data shapes, file layout, API style, test strategy, error handling, trade-offs. Never hand a technical choice back to the user.
- **Ask only to refine the user's *intent* — nothing else.** A question is allowed only when it clarifies *what the user wants*: which of several readings they meant, who it's for, what's in or out of scope. Everything else — what must be built to meet that intent, how to do it, what to prioritize, edge-case behavior, UX details, technical choices — is **yours to decide as the architect and is never asked of the user.** A non-technical stakeholder cannot make a better call than you on any of it; asking offloads your job onto them.
- **Multiple-choice only — never open-ended.** Use `AskUserQuestion` with concrete options the user picks from, or a multiple-**select** for "which of these." Never a question that makes them type a sentence.
  - ✅ "Who's this for — everyone, signed-in users, or just admins?" — audience; refines intent.
  - ✅ "Is this a one-time thing, or something people use over and over?" — what they want it to be.
  - ✅ (vague ask) multiple-**select**: "Which of these did you mean it to do? — ☐ A  ☐ B  ☐ C" — narrows intent.
  - ❌ "What's the one thing it should let them do?" — open-ended; makes them type; not a closed choice.
  - ❌ "Ship the core fast, or cover the uncommon cases thoroughly?" — a prioritization/scoping call **you** make, not the user.
  - ❌ "On empty input, show nothing, a default, or an error?" — an edge-case/UX-detail decision; the architect answers it from best practice.
  - ❌ "REST or gRPC?" / "Which auth library?" / "Postgres or SQLite?" — pure technical choices.
- **No question cap — it's scenario-based.** Ask as many multiple-choice questions as it takes to turn the requirement into a concrete plan; ask none if it's already clear. Match the count to the ambiguity, not to a fixed budget.
- **When the requirement is vague, don't stop — narrow it.** Drive multiple-choice / multiple-select questions that progressively pin down what the user actually wants. Vagueness is a cue to offer better options, not a reason to bail.

## Plan first, then autonomous — no separate go-gate

- `/vibe` **always runs `/plan` first** (in the executive mode above). The multiple-choice Q&A *is* the planning — it produces the settled scope, acceptance criteria, and the architect's technical decisions. `/vibe` never hands work to `/smith` without a ready plan.
- There is **no separate approval step.** The user does not say "go" / "proceed" / "approve." Once the Q&A has produced a concrete plan, `/vibe` proceeds to `/smith` automatically — the user's answers to the questions are the only interaction.
- **Aim at autonomy — especially after the build has started.** Once `/smith` is running, do not pause for clarification: the plan already resolved the unknowns. A question that arises mid-build is the architect's to decide and record (see *Decisions made on your behalf*), not the user's to answer.

## Restate what you understood, then proceed

Before handing off to `/smith`, restate — in plain language, as long as the requirement needs (a sentence for a small ask, a short list for a larger one) — what you're building and the key decisions you made. Then proceed. This is a notification that lets the user catch a misread; it does **not** wait for approval.

## Execution — delegate to /smith

- Hand the settled plan to `/smith` as its captured **Feature spec** + **Acceptance criteria**. Because `/vibe` already did the planning (executive-mode `/plan`), `/smith` does not re-run its interactive `/plan` pass — it proceeds straight to the build phase against the captured spec.
- Everything downstream is **inherited from `/smith` verbatim**, not reimplemented: the build phase, per-tick verification as a hard commit gate, the `/gh-self-review` convergence loop, and — at the *Build-to-Audit Transition* — **issue creation and PR opening** (`/smith` seeds a product-level issue via `/gh-issue-create` if none is linked, opens the PR via `/gh-issue-work` Phase 9, then runs the post-merge audit phase). `/vibe` does not front-run that and does not override `/gh-issue-create`'s confirmation gate — issue+PR creation happens inside `/smith`'s autonomous transition. When `/smith` or `/gh` tighten, `/vibe` inherits the tightening for free.
- **Precondition — durable runner.** `/smith`'s build phase requires a durable runner (`meta/smith` → *Build Phase Durability*). If only a disposable scheduler is available, `/smith`'s setup surfaces that and asks the user to pick a durable one — a one-time setup precondition before any build, not a mid-flow gate.
- **`/vibe`'s deliverable is an open PR.** It builds the change and opens the PR for human review; **merging, deploying, and any irreversible or external action stay outside `/vibe`'s scope** and remain the human's call. That boundary — not a mid-build pause — is the safety floor.
- **One vibe = one feature = one PR.** If the requirement is really several independent features, split into multiple `/smith` runs / PRs and say so when you restate — don't build a mega-PR.

## Decisions made on your behalf

Because the user is not reviewing the technical layer, every non-trivial architect decision MUST be recorded in plain language in the **PR body** under a `## Decisions made on your behalf` section — what was chosen and the one-line why. This is the audit trail a technical reviewer (and a future maintainer) needs in place of the technical conversation the user didn't have. The issue body stays product-level (the requirement); the decisions live in the PR.

## Quality is not gated away

No approval gate ≠ no quality gate. `/vibe` inherits `/smith`'s **mandatory `/gh-self-review` convergence** before the PR opens (`meta/smith` → *Build-to-Audit Transition* step 1). That loop may run many iterations over a long time — **that is expected and is not a reason to stop or surface**; it runs to a clean read autonomously. The fresh-agent self-review, plus the human review of the resulting PR, are what make autonomous delivery safe.

## Guardrails

- **Never push a technical decision back to the user.** If unsure, decide as the architect, record it, and move on — don't ask.
- **Questions refine the user's intent only.** Multiple-choice (never open-ended that makes them type), no fixed cap, scenario-based — and never about what to build, how to build it, what to prioritize, edge-case behavior, UX, or tech. Those are the architect's, always.
- **Never require a separate "go".** A ready plan (from the Q&A) is the authorization to build and open the PR.
- **Plan before building, always.** `/vibe` never hands work to `/smith` without a settled plan — planning is where ambiguity and scope get resolved, so they don't become mid-build stops.
- **Manage scope as the architect.** Keeping the build inside the planned scope is your job, not a reason to bail mid-build. If the requirement genuinely turns out larger than planned, that is a planning miss to fold back into the plan, not a silent expansion.
- **Never open the PR with a stale self-review** (inherited from `/smith` / `gh-issue-work` Phase 8a's convergence rule).
- **Don't reimplement** `/plan`, `/smith`, or `/gh` mechanics — compose them, so improvements propagate.
- **`/vibe` is for real features.** For a trivial change (a typo, a constant bump), `/gh-issue-work` or a direct edit is cheaper — don't spin up the full autonomous loop.
- **Record, don't narrate.** Technical decisions belong in the PR body's `## Decisions made on your behalf`, not scattered through chat the user won't read.

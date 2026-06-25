---
description: Shared planning core — what a settled plan is, how to know it's ready, and the .plan/<slug>.md contract that hands it to implementation. Do not invoke directly; composed by /plan (developer intake) and /dream (executive intake).
triggers:
  - planning core
  - settled plan
  - plan contract
  - acceptance criteria
  - readiness check
when: Internal. Loaded by /plan or /dream to define the settled-plan deliverable, the readiness/consistency checks, and the plan-contract artifact. Both wrappers add only their intake style and role.
related:
  - flows/plan/plan
  - core/dream
  - core/completion
  - flows/build/forge
  - flows/build/smith
  - core/todo-import
---

# Planning Core — the settled plan and its contract

`/plan` and `/dream` are two intakes over **one destination**: a settled, buildable, verifiable plan. This core holds what that plan *is*, how to know it's *ready*, and the artifact that hands it to implementation. The wrappers differ only in intake style and who owns technical decisions — `flows/plan/plan` (developer: open-ended, the user steers tech) and `core/dream` (executive: multiple-choice, the architect owns tech). Neither restates what's below.

> ## ⛔ Do not invoke directly
> No slash command. Loaded via `kb_get` by `/plan` or `/dream`.

## The deliverable — a settled plan

A settled plan captures, in plain language:

- **Feature spec** — what is being built and why.
- **Acceptance criteria** — a checklist, each item objectively verifiable by test, command output, UI check, or a documented manual check.
- **User-stated rules** — verbatim constraints from the conversation.
- **Decisions made on the user's behalf** — each non-trivial technical choice with a one-line why (the audit trail a reviewer needs in place of a conversation they didn't see).
- **Feature-splits & blockers** — genuinely separate features and hard blockers per `core/completion`'s three dispositions. This is **not** a parking lot for in-scope work, which is always built (disposition 1). Disposition is intake-specific: `/plan` records genuinely-separate features for the developer to triage; `/dream` (autonomous, non-technical path) resolves in-scope concerns into the plan and splits genuinely-separate features into their own plans rather than handing the user a note they can't action (`core/dream` → *Hazards — deal with them, never defer*).

## Readiness check

A plan is ready only when:

- **Intent clarity** — desired outcome, audience, and scope boundary are clear.
- **Acceptance clarity** — every criterion is objective enough to verify.
- **Constraint capture** — user-stated rules, repo guidance, KB guidance, and known external constraints are captured as rules or decisions, not left as chat-only context.
- **One-feature boundary** — the spec fits one coherent feature; independent features split into separate plans.

## Consistency check

Before declaring ready:

- Feature spec, acceptance criteria, verification plan, and decisions must agree with each other.
- Any requirement that cannot be verified, contradicts another, or expands beyond the feature boundary is resolved first.
- Helpful but out-of-scope work is handled per the intake's disposition rule (`core/completion`) — a genuinely separate feature is split out, never silently folded into the plan; in-scope work is built, never parked.

## The plan contract — `.plan/<slug>.md`

When planning settles **and** the build will be driven by an implementation loop — `/forge`, the candyland conductor, or `/smith` via `/vibe` — write the settled plan to `.plan/<slug>.md` so the loop can ingest it without replaying the planning conversation. This is the **only artifact that crosses planning → implementation**.

- `slug` is a kebab-case summary of the feature.
- Contents are the deliverable fields above, with **acceptance criteria as a `[ ]` checklist** (the implementation loop ticks them).
- The contract is the build-phase source of truth: the loop conforms to it and never silently rewrites it.

A developer implementing in-session straight from `/plan` doesn't need the contract — the `/todo` import in `flows/plan/plan` is their resumable view. The contract is specifically what a *separate* loop reads; writing both when both apply is fine.

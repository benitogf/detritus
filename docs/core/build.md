---
description: Shared build contract — the single build unit (smallest delta → verification hard gate → commit) and delivery (self-review convergence → one PR via gh-issue-work). Do not invoke directly; composed by /smith (sequential) and the implementation loop (parallel, per-coder) under /forge / candyland.
triggers:
  - build unit
  - build contract
  - verification gate
  - delivery
  - self-review convergence
when: Internal. Loaded by /smith and by implementation-loop roles (roles/coder-*, roles/tech-lead) to define how one unit of build work is verified and committed, and how a finished branch is delivered as a PR.
related:
  - flows/build/smith
  - flows/build/forge
  - roles/tech-lead
  - core/coder
  - flows/github/gh-self-review
  - flows/github/gh-issue-work
  - flows/github/gh-issue-create
---

# Build Core — the build unit and delivery

One definition of "make a verified change and ship it," shared by every builder so they don't drift. `/smith` runs the build unit **sequentially** in one agent; the implementation loop runs **one build unit per coder** in parallel under a tech-lead. Both deliver through the same path. Neither restates what's below.

> ## ⛔ Do not invoke directly
> No slash command. Loaded via `kb_get` by `/smith`, `roles/tech-lead`, and `roles/coder-*`.

## The build unit

A single unit of build work — one acceptance item, or one partitioned task — is committed only when it is green:

1. **Smallest in-spec delta.** Implement just the change the current item/task needs. No drive-by refactors, no scope the spec didn't name; out-of-scope needs are hazards (disposed of per the caller's hazard rule), never folded in.
2. **TDD where a defining test exists.** When the item is defined by a failing test (always true in the parallel loop — `roles/coder-test-engineer` writes it; usual in `/smith`), "done" means that test goes green.
3. **Verification is a hard gate, per commit.** Run the canonical verification command. It must complete **green** on this unit. A unit that fails verification does **not** commit — log the failure evidence (failing test, assertion, first error) as the next attempt's context and retry against it. Partial or unrun verification does not count.
4. **Commit on green.** When verification is green and the item/task is actually met, commit. Accumulate units on one feature branch; do not open intermediate PRs.

## Delivery — converge a clean self-review, then one PR

When all the work for the branch is green:

1. **Loop `/gh-self-review` to a clean read.** Review the full branch diff. If it reports blockers **or forces any amendment** — blocker fix, non-blocker cleanup, anything — make the fix as more build units, then **re-review the updated diff**. Stop only when a no-blocker pass is observed against a diff that has not changed since that pass. Every amendment invalidates the prior read: a review of a diff that just changed has not reviewed what you're about to ship.
2. **Push the branch.**
3. **Open exactly one PR via `gh-issue-work` Phase 9.** Do not reimplement `gh pr create`. If no issue is linked, seed one via `gh-issue-create` first. Entering at Phase 9 keeps PR shape, footer, and base-branch detection owned by the `/gh` flow, so tightening there propagates for free.

Merging, deploying, and any irreversible/external action stay outside the build contract — the human's call.

## What this is not

- Not the loop mechanics (scratchpad, durability, cadence, skip-streak) — those are `core/loop`.
- Not the orchestration (partition, parallel coordination, integration) — that's `roles/tech-lead`.
- Not phase/scope/audit policy — those stay in the wrapping command (`flows/build/smith`, `flows/build/forge`) so each command's intent stays distinct.

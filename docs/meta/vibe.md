---
description: Executive intake → autonomous build → PR. The user states an outcome in one line; the agent owns every technical decision as the software architect, asks only shallow product-level questions, and drives the work through /smith to an open PR with no go-gate and no blocking until the PR is up.
category: meta
triggers:
  - vibe
  - vibe code
  - just build it
  - just build me
  - build me this
  - ship it no questions
  - no questions just build
when: A non-technical stakeholder states a desired outcome and wants it built and PR'd without being asked to make technical decisions or to approve a plan. Use when the user explicitly wants zero gates between the ask and the PR.
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

# /vibe — Executive Intake → Autonomous Build → PR (no go-gate)

The user is a non-technical stakeholder. They state an **outcome** in one line; that line IS the spec. `/vibe` owns every technical decision as the software architect, asks at most a couple of shallow *product-level* questions (only if it genuinely cannot proceed), then drives the work through `/smith` to an open PR — **without a "go" gate and without blocking until the PR is up.**

`/vibe` is a thin orchestrator. It does **not** re-implement planning, building, review, or PR mechanics. It composes `/plan` (intake), `/smith` (build → PR → audit), and `/gh` (delivery), changing exactly two things: the **approval gates are removed**, and a small set of **hard stops + a non-blocking restatement** are added. In one line: **`/vibe` = `/smith` run gateless, fronted by executive intake.**

## Intake contract — the architect owns the technical layer

- Treat the user as **non-technical**. Their message is the spec: `{outcome}`. Do not ask them to refine the approach, name tools, or pick a design.
- **You are the software architect. Every technical decision is yours**: language, libraries, data shapes, file layout, API style, test strategy, error handling, trade-offs. Never hand a technical choice back to the user.
- Questions to the user, if any, are **shallow and product-level only** — about the outcome, audience, or priority, never implementation:
  - ✅ "Should this be visible to everyone, or just admins?"
  - ✅ "Is this a one-off, or does it need to run on a schedule?"
  - ❌ "REST or gRPC?" / "Which auth library?" / "Postgres or SQLite?"
- **Question cap: at most 2, batched once up front** via `AskUserQuestion`, *before any work begins*. Never ask mid-build. If a reasonable product assumption is available, **make it and record it** (see *Decisions made on your behalf*) instead of asking.
- The only mandatory question is when the one-liner is too vague to know **what outcome** is wanted (not *how* — *what*). Ask one clarifying product question, then proceed.

## No go-gate

- Do **not** ask the user to approve the plan. Do **not** wait for "go" / "proceed" / "yes". **Invoking `/vibe` is the standing authorization** to plan, implement, and open the PR.
- This is the deliberate difference from `/smith`, whose first pass gates on `/plan` approval (`meta/smith` → *Initial /plan Pass-Through* step 3). `/vibe` removes that gate and records the `/vibe` invocation as the authorizing *Last user directive* in the smith scratchpad.

## Non-blocking restatement (a notification, never a gate)

Before starting, emit exactly one plain-language line:

```
Building: <one-sentence restatement of the outcome>. I'll make the technical calls and surface a PR when it's ready.
```

This is a **notification the user may interrupt — not a question, and it waits for nothing.** Proceed immediately. Its only purpose is to catch a catastrophic misread of the one-liner for free. If the user says nothing, that is not a gate being satisfied — work was already underway.

## Execution — delegate to /smith

- Hand the settled outcome plus your architect plan to `/smith` as its captured **Feature spec** + **Acceptance criteria**, **skipping** `/smith`'s interactive `/plan`-approval gate. Synthesize the acceptance criteria yourself from the outcome — that is the architect's job, not the user's.
- Everything downstream is **inherited from `/smith` verbatim**, not reimplemented: the build phase, per-tick verification as a hard commit gate, the `/gh-self-review` convergence loop, and — at the *Build-to-Audit Transition* — **issue creation and PR opening**. `/smith` already seeds a product-level issue via `/gh-issue-create` (if none is linked) and opens the PR via `/gh-issue-work` Phase 9, then runs the post-merge audit phase. `/vibe` does **not** front-run that: it does not separately call `/gh-issue-create` up front. This matters for the gate — `/gh-issue-create`'s "post as-is" confirmation (Phase 4) is non-skippable and `/vibe` must not override it; by leaving issue+PR creation inside `/smith`'s autonomous transition, the issue is created in the same no-live-user context `/smith` already operates in, not bypassed by `/vibe` fiat. When `/smith` or `/gh` tighten, `/vibe` inherits the tightening for free.
- **Precondition — durable runner.** Because `/vibe` delegates the build to `/smith`, it inherits `/smith`'s hard requirement of a durable runner (`meta/smith` → *Build Phase Durability*; disposable runners like Cloud Routines / `worktree` cannot host build-phase state). If the only available scheduler is disposable, `/smith`'s setup surfaces mode-3 (report incompatibility, ask the user to pick a durable scheduler). That is a **setup-time precondition, not a mid-flow gate** — it happens before any build and is the one interaction `/vibe` cannot avoid; surface it plainly rather than letting it read as a broken "no-blocking" promise (it is also hard stop #4's sibling — see *Hard stops*).
- **One vibe = one feature = one PR.** If the outcome implies several independent features, split into multiple `/smith` runs / PRs and say so in the restatement — do not build a mega-PR.

## Decisions made on your behalf

Because the user is not reviewing the technical layer, every non-trivial architect decision MUST be recorded in plain language in the **PR body** under a `## Decisions made on your behalf` section — what was chosen and the one-line why. This is the audit trail a technical reviewer (and a future maintainer) needs in place of the approval conversation that was skipped. The issue body stays product-level (the outcome); the decisions live in the PR.

## Hard stops — the only times /vibe breaks "no blocking"

`/vibe` runs gateless **except** the following, where it MUST pause and surface to the user in one line before proceeding. Unbounded autonomy is a hope, not a plan (`meta/truthseeker` → *Reject Fragility*):

1. **Irreversible or external side-effects** — anything outside the PR: production deploy, data deletion, force-push to a shared branch, dropping a database, sending email/money, or any action that can't be undone by closing the PR.
2. **Security-sensitive surface** — auth, secrets, access control, crypto. State the approach in one line before implementing.
3. **Unresolvable ambiguity** — the outcome can't be pinned down with a reasonable assumption *and* a shallow product question can't phrase the gap. Ask once; if still unclear, stop.
4. **Scope explosion** — the outcome turns out to need materially more than the one-liner implied (multiple subsystems, a data migration, a new external dependency). The baseline this is measured against is the **acceptance criteria + assumptions you recorded up front** (the *Decisions made on your behalf* set) — since the user never settled a spec, that recorded interpretation is what makes "explosion" falsifiable. A new external dependency you did not record as in-scope is an explosion, not a free architect call. Surface the expanded scope; do not silently build it.
5. **Self-review can't converge** — the `/gh-self-review` loop keeps returning blockers. Surface rather than ship a known-flawed PR.

A hard stop is a one-line surface-and-wait, not a routine approval. Everything outside these five proceeds gateless.

## Quality is not gated away

No-go-gate ≠ no quality gate. `/vibe` inherits `/smith`'s **mandatory `/gh-self-review` convergence** before the PR opens (`meta/smith` → *Build-to-Audit Transition* step 1). Removing the human approval does not remove the fresh-agent review — that review is what makes gateless delivery safe.

## Guardrails

- **Never push a technical decision back to the user.** If unsure, decide as the architect and record it — don't ask.
- **Never exceed the shallow-question cap** (≤2, batched up front, product-level), and never ask mid-build.
- **Never wait for "go".** The invocation is the authorization.
- **Never silently expand scope** past the one-liner — that is hard stop #4.
- **Never open the PR with a stale self-review** (inherited from `/smith` / `gh-issue-work` Phase 8a's convergence rule).
- **Don't reimplement** `/plan`, `/smith`, or `/gh` mechanics — compose them, so improvements propagate.
- **`/vibe` is for real features.** For a trivial one-line change (a typo, a constant bump), `/gh-issue-work` or a direct edit is cheaper — don't spin up the full autonomous loop.
- **Record, don't narrate.** The technical decisions belong in the PR body's `## Decisions made on your behalf`, not scattered through chat the user won't read.

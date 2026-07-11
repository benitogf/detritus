---
description: Shared build contract — the single build unit (smallest delta → verification hard gate → commit) and delivery (self-review convergence → one PR per impacted repo via gh-issue-work). Do not invoke directly; composed by /smith (sequential) and the implementation loop (parallel, per-coder) under /forge / candyland.
triggers:
  - build unit
  - build contract
  - verification gate
  - delivery
  - self-review convergence
when: Internal. Loaded by /smith and by implementation-loop roles (roles/coder-*, roles/tech-lead) to define how one unit of build work is verified and committed, and how a finished branch is delivered as a PR.
related:
  - core/completion
  - core/flows
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

1. **Smallest in-spec delta.** Implement just the change the current item/task needs. No drive-by refactors, no scope the spec didn't name; out-of-scope needs are disposed of per `core/completion`'s dispositions (a genuinely separate feature is a feature-split, a hard blocker is surfaced), never folded in.
2. **TDD where a defining test exists.** When the item is defined by a failing test (always true in the parallel loop — the owning coder writes it failing-first as the task's first step; usual in `/smith`), "done" means that test goes green.
3. **Verification is a hard gate, per commit.** Run the canonical verification command. It must complete **green** on this unit. A unit that fails verification does **not** commit — log the failure evidence (failing test, assertion, first error) as the next attempt's context and retry against it. Partial or unrun verification does not count.
   - **Fresh worktree → satisfy declared build preconditions first.** At a fresh checkout/worktree, run the repo's declared generators/installs (`go generate ./...`, `npm ci`) before judging verification — a build failing on missing generated or vendored artifacts is an **environment step to perform, not a diff failure**. Silently substituting a weaker check (parse-only, vet-only) without attempting the precondition is a verification-gate miss.
   - **Observing another tree is read-only — never mutate the working branch to peek.** A throwaway check that compares against another state (is this failure pre-existing on `<base>`? does the suite pass without my change?) must not disturb the tree the build lives on. Use an isolated view — a detached `git worktree add` on the base, a fresh clone, or `git -C <other-checkout>` — then discard it. **Never** `git checkout <base>` / `git stash` on the working branch to look and swap back: it races any concurrent worker in the same tree and can strand uncommitted work. In particular **never `git stash pop` blind** — pop targets the *top of the stash stack*, which may be a pre-existing unrelated stash, and a conflicting apply dumps foreign content into the tree (worse: when your changes are gitignored-only, `git stash -u` saves *nothing*, so the later `pop` operates on someone else's stash entirely). If a stash is unavoidable, `git stash list` first and push/pop a **named** entry (`git stash push -m <tag>` → pop by ref), never the bare stack.
3a. **Wired + reachable, not just green.** A green suite is satisfiable with **dead-in-prod code**, so a unit's in-scope feature is deliverable only when it has a production caller — confirmed by running the assembled artifact or tracing from the entrypoint into the change (`core/completion` → *Definition of done* 2a). Unwired code is a blocker, not a deliverable unit; "wired-but-needs-a-browser" is reachable and may defer final confirmation to `/verify`.
4. **Commit on green.** When verification is green and the item/task is actually met, commit. Accumulate units on one feature branch; do not open intermediate PRs.

## Delivery — converge a clean self-review, then a PR per impacted repo

When all the work for the branch is green:

1. **Loop `/gh-self-review` to a clean read.** Review the full branch diff. If it reports blockers **or forces any amendment** — blocker fix, non-blocker cleanup, anything — make the fix as more build units, then **re-review the updated diff**. Stop only when a no-blocker pass is observed against a diff that has not changed since that pass. Every amendment invalidates the prior read: a review of a diff that just changed has not reviewed what you're about to ship.
2. **Push the branch.**
3. **Open one PR per impacted repo via `gh-issue-work` Phase 9.** Do not reimplement `gh pr create`. If no issue is linked, seed one via `gh-issue-create` first. Entering at Phase 9 keeps PR shape, footer, and base-branch detection owned by the `/gh` flow, so tightening there propagates for free.

What a branch / PR reports as **delivered** must be honest: a branch carrying only unwired (dead-in-prod) units, or a delivery step that opened no PR while in-scope work is still open, is **not** a delivery and is not reported as `done`-success — it carries the distinct terminal state `core/completion` → *Honest terminal state* defines. The one legitimate `prsOpened:0` is **branch delivery** (`deliver=="branch"`): the unit is committed onto the shared branch by design and the parent quest opens the PR (see *Multi-repo delivery*).

### One deliverable, one PR — converge, don't spray

A builder or loop iterating toward a single deliverable **converges on one PR** — it never opens a pile of competing PRs for the same work. This is the delivery-side companion to `core/completion`'s exit gate (re-attempts are remediation, not new artifacts) and the build-scope analogue of `/gh`'s **one-issue-one-PR** rule. Every builder that composes this contract — a single run, a converging loop (`/quest`, `/smith`, `/forge`), or an open-ended loop (a `/quest --per-finding`, `/janitor`) — inherits it (`core/flows` → *PR policy*):

- **One PR per DISTINCT shippable item, per impacted repo** — never repeated PRs for the *same* item. Converging work (a converge `/quest`, `/smith`, `/forge`) **converges**: iterate on one branch, deliver one PR per repo. Only open-ended freeseeking (a `/quest --per-finding`, `/janitor`) opens many PRs over time — and that means one per distinct accepted finding, not many attempts at one.
- **Once a PR is open for a deliverable, keep working it in place.** Every later attempt at that same deliverable delivers **onto the existing PR** — `deliver: feedback` on its head branch (commit, push, update in place), never a fresh PR. New PRs are only for genuinely new, non-overlapping work.
- **Re-attempts are remediation, not new artifacts.** Opening PR #B to "reconcile" #A, then #C to "supersede #A/#B", is **thrash** — it externalizes iteration as conflicting PRs a human must untangle. The tell is a repo accumulating **reconcile / consolidate / fold #N / supersede #N** PR titles for one deliverable instead of a single PR iterated to done.

Merging, deploying, and any irreversible/external action stay outside the build contract — the human's call.

## Multi-repo delivery — one feature, a PR per impacted repo

A single feature may legitimately span **N repos** (N ≥ 1, no cap) — e.g. a backend change in one repo plus the doctrine/docs that describe it in another. This is **disposition 1** (in-scope, handle now per `core/completion`), delivered as **one PR per impacted repo**, branched and committed in each repo independently:

- Each impacted repo gets its own branch, its own self-review convergence, and its own PR via `gh-issue-work` Phase 9 — the same delivery, run once per repo. Cross-reference the sibling PRs in each body so the set reviews as one feature.
- The cross-repo nature is **never** a reason to feature-split (disposition 2), defer, or surface as a blocker (disposition 3). A change co-required in another repo is part of *this* feature, not a separate one.
- **Partial failure is honest, not contagious.** If one repo's PR can't open (missing remote, auth), report that repo precisely and still open every PR that can — one repo's delivery failure does not fail the others.
- The feature is **done** only when every impacted repo's PR is open (or its specific failure is surfaced) — not when the first repo's PR opens.

## Delivery modes — how a run hands off its work

A run carries a **delivery mode** (`run.deliver`) that decides where its work lands. There are four first-class modes; the mode — not the PR count — is what tells an honest terminal state from a no-op (`core/completion` → *Honest terminal state*):

- **`pr`** — new work. Branch fresh, commit the units, open a **NEW PR per impacted repo** via `gh-issue-work` Phase 9 (the *Delivery* and *Multi-repo delivery* sections above). The default for feature-shaped work.
- **`branch`** — a quest's child run commits its units onto the shared quest branch and opens **no PR by design**; the parent quest opens the PR. The one legitimate `prsOpened:0` for executing work (see *Multi-repo delivery*).
- **`feedback`** — handling review feedback/findings on an **EXISTING PR**. Deliver onto **that PR's head branch**: check it out, commit the fix, push → the PR updates **in place**. **Never open a duplicate PR.** Multi-repo: each affected repo's findings land on **that repo's existing PR** — no new PRs in any repo. Mirrors `/gh-feedback-work` (push fixes, update in place, never comment, never open a new PR).
- **`review`** — produce **findings, not code**. The outcome may be **no PR at all** (e.g. "check PR #N against a requirements doc" with nothing actionable) — a valid "reviewed, nothing to do." Findings that *are* actionable are then handled per `feedback` mode against the PR in question.

The terminal states these modes earn — `feedback` updating an existing PR, `review` ending with no PR — are valid `done`, not no-ops; `core/completion` → *Honest terminal state* defines each.

## What this is not

- Not the loop mechanics (scratchpad, durability, cadence, skip-streak) — those are `core/loop`.
- Not the orchestration (partition, parallel coordination, integration) — that's `roles/tech-lead`.
- Not phase/scope/audit policy — those stay in the wrapping command (`flows/build/smith`, `flows/build/forge`) so each command's intent stays distinct.

---
description: Apply the TDD-per-item work protocol to a single /todo item — sync, prove with a failing test, smallest fix, measure, sweep dead code + gofmt, ship via /gh, advance on "next". Opt-in; doesn't gate /todo done. Absorbs /audit-handle's job into the /todo family.
triggers:
  - todo-work
  - work todo
  - work this item
  - tdd this item
  - next todo item via tdd
  - process todo with discipline
when: User wants to drive a single /todo item (typically a code-fix-shaped one) through a strict TDD-per-item protocol that ends in a /gh PR. Equivalent to what /audit-handle did for review.md entries, but operates on a /todo item with its richer state. Opt-in per item — /todo done remains the path for items that don't fit the protocol.
related:
  - flows/project/todo
  - core/todo-audit
  - core/todo-import
  - flows/github/gh
  - flows/github/gh-issue-work
  - flows/github/gh-issue-create
  - flows/github/gh-self-review
  - flows/principles/truthseeker
  - flows/testing/testing-go-backend-async
  - flows/principles/go-modern
  - flows/principles/coding-style
---

# /todo-work — TDD-per-item work protocol on a /todo item

_Follows `flows/project/todo` convention #13: main session validates input + drives the per-item loop + calls TodoWrite + prints confirmation; the phases below describe the work the protocol enforces._

Run the strict TDD-per-item protocol on a single `/todo` item — the same loop `/audit-handle` ran on `review.md` entries, adapted to operate on a `/todo` item with its richer state (priority, group, scope, evidence, deps, fork-eligibility).

**Opt-in by design.** Not every `/todo` item is a code fix that fits this protocol. Items like *"Reply to Bo about the staging deploy"* or *"Investigate why X is slow"* don't have a failing-test reproduction step. The user invokes `/todo-work` only when the item IS code-fix-shaped (`scope.concreteness == "concrete"`, file:line evidence in scope, action is a bug fix / behavior correction / refactor with verifiable outcome). For everything else, `/todo done` after working the item however it needs to be worked.

`/todo-work` does NOT gate `/todo done`. Users can mark items done without going through the protocol. The protocol is available; it's not mandatory.

## Inputs

- `/todo-work <id-or-fuzzy>` — resolve to one /todo item (internal id or fuzzy substring match against item title/body). The top open item by sort order if no argument is given.
- `/todo-work --next` — same as no-arg: pick the top open item and start the protocol on it.
- `/todo-work --no-pr` — run the protocol but stop before opening the PR. Useful for local-only fixes the user wants to bundle later.

## Pre-flight: item eligibility check

The skill validates the item before starting the protocol. **Refuse to start if any of these hold**:

- `scope.concreteness != "concrete"` — exploratory/ambiguous items don't have a clear failing-test target.
- `scope.repos` is empty — no target codebase.
- `scope.paths` is empty AND `scope.evidence` is empty — no concrete code surface to work.
- `status == "in-progress"` with `forkSession != null` — item is claimed by another conversation.
- `deps` contains an open item — prerequisites haven't shipped.

On refusal, surface the failure reason and suggest the alternative path:
- *"Item is ambiguous — use `/todo edit --title --body --evidence` to sharpen it first, or `/todo done` if you've worked it without the TDD protocol."*

## Per-item loop

Apply these steps in order to the resolved item. If a step blocks, stop and report — do not improvise around it.

1. **Resolve the item** (per Pre-flight) and **claim it via `/todo claim <id>` (`core/todo-audit` → *Claim*)** so the parent and other sessions see it as in-flight. The claim writes `status: in-progress`, stamps `claimedAt`, and (in a forked session) sets `forkSession`.

2. **Sync the branch.** `git checkout <default-branch>` and `git pull --ff-only` so work starts from current head. New branch is cut from the fetched default, never from the current working branch. Branch name derived from the item's `title` (kebab-case, scoped to the change shape — `fix/`, `feat/`, `refactor/`, `docs/`, `chore/` prefix per /gh-issue-work convention).

3. **Verify the issue is real and actionable.** Write a test (or tests) that *fail* against the current code and replicate the reported behavior. Use the item's `scope.evidence` file:line refs as the starting point — drop a breakpoint or failing assertion at the cited location. If the test cannot be made to fail, the item may be stale — stop, report, and offer to evict it via `/todo done` (no PR) or downgrade it via `/todo edit`. Don't silently advance on a stale item.

   - For async or subscription code, follow `flows/testing/testing-go-backend-async` patterns: precise `wg.Add(N)`, callbacks only update state, no sleeps, no polling.
   - Tests must be fast and low-resource. No long sleeps, no heavyweight load generators, no high goroutine counts unless essential.

4. **Implement the smallest fix that makes the failing test pass.** Don't expand scope. The item is one item; sibling concerns become separate `/todo add` items captured during this step, NOT silently absorbed into the current PR.

5. **Measure performance impact.** A throwaway benchmark or microbench to confirm the fix doesn't regress hot paths. Do not commit measurement code — strip it before opening the PR unless it has durable value as a permanent bench.

6. **Dead-code check on the changed files.** `deadcode ./...` or scoped to the touched packages. Delete unreachable AND unexported functions. Public-API "unreachable" findings are noise — library exports are reachable from external consumers.

7. **gofmt the changed files.** `gofmt -l` to detect drift, `gofmt -w` on the touched files only. Don't reformat unrelated files inside a single-item PR.

8. **Open issue + PR via `/gh`**. Let the router dispatch to `gh-issue-create` then `gh-issue-work`. Issue body is product-focused (no code identifiers); PR body describes the final state. **Skip this step if `--no-pr` was passed.**

9. **Mark the item done via `/todo done <id>`.** This is the protocol's terminal step. The item is removed from the store (eviction — no retained `done` status), the forkSession lock (if any) is released with it, and the next-up survivors are re-ranked per the quick re-rank in `flows/project/todo` → */todo done*.

10. **Hand back to the user.** Stop there. The user reviews the PR and either gives feedback or says "next".

11. **On "next"** (or any equivalent advance signal): resolve the new top open item (per the view's sort (`flows/project/todo` → */todo view*)) and restart at step 1 on it. Continue until the open set is empty or the user stops.

## When an item turns out to be stale

If step 3 cannot produce a failing test, the item was a false positive or has already been addressed elsewhere. Report:

- What was tried (the candidate failing-test scenarios).
- Current code state at the cited `scope.evidence` refs, showing the issue is not present.
- Recommendation: evict it via `/todo done` (it won't open a PR), or — if you want it to stay visible — downgrade its priority via `/todo edit`. (A `stale-on-arrival` tag is pointless now: `/todo done` removes the item, so the tag wouldn't persist.)

Wait for explicit user confirmation before any status mutation. No silent done-flips.

## When multiple items collapse into one PR

Occasionally two `/todo` items share a single fix (e.g., a regex constant referenced from two places). If a single failing test proves both items and a single fix resolves both, bundle them. Note both in the PR body and `/todo done` BOTH ids on "next". This is the exception — default is one item per PR.

## TodoWrite sync

Per `flows/project/todo` convention #10:
- After Step 1 (the claim), TodoWrite re-syncs showing the item as `in_progress`.
- After Step 9 (`/todo done`), TodoWrite re-syncs with the item gone and the survivors re-ranked.

The skill's chat report ends with the standard one-line confirmation: `TodoWrite refreshed — N items now active across M groups.`

## What this skill is not

- Not a router. `/gh` already routes between create / work / feedback / self-review / pr. This skill drives ONE `/todo` item and hands off to `/gh` per item.
- Not a /todo mutator beyond claim/done. It does NOT change priority, scope, group, evidence, deps, or any other field of the item it's working. Those are the user's calls via the `/todo-*` family.
- Not a code-style enforcer. Per-file `gofmt` and `deadcode` checks are scoped to the worked-item PR; project-wide cleanups happen separately.
- Not a planner. The item's priority is already settled by the audit/idle passes (`core/todo-audit`) or */todo add*'s inference (`flows/project/todo`); this skill processes the one item the user (or `--next`) targets.

## Guardrails

- Never start step 4 (implement) without a failing test from step 3. The truthseeker prove-before-acting rule applies.
- Never expand scope past the one item being processed. Sibling concerns become separate `/todo add` items captured during step 4, not silently absorbed.
- Never silently mark a `/todo` item done. The terminal `/todo done` (step 9) is the user's signal-of-completion; refuse if the user hasn't given an advance signal.
- Never push a `replace` directive against a local path to a long-lived branch. Local replace is fine for cross-repo testing but must be removed before the PR is ready for review.
- Never run the protocol on an in-flight (`forkSession != null`) item that you didn't claim. Refuse and surface the conflict.
- Stop and ask if the worked branch has unexpected uncommitted state — the item should be a clean from-default branch.
- The protocol is OPT-IN per invocation. Don't auto-invoke `/todo-work` from the view or the audit pass; the user always chooses.

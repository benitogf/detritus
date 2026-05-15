---
description: Process a flat list of audit / review / feedback items one entry at a time — sync, prove with a failing test, fix, measure, sweep dead code + gofmt, ship via /gh, advance on "next".
category: meta
triggers:
  - audit handle
  - handle audit
  - review list
  - feedback list
  - work the audit
  - next audit item
  - process audit
  - process review
  - process feedback
when: A working file (review.md, audit.md, feedback.md, ...) holds a flat checkbox list of items to address. User wants the same protocol applied to every item without restating it. Each item earns its own PR only if it survives the relevance check.
related:
  - meta/audit-add
  - meta/gh
  - meta/gh-issue-work
  - meta/gh-issue-create
  - meta/gh-self-review
  - meta/truthseeker
  - testing/go-backend-async
  - patterns/go-modern
  - patterns/coding-style
---

# /audit-handle — Process an audit / review / feedback list

A working file (typically `review.md`, `audit.md`, or `feedback.md`) holds a flat checkbox list of items to address. This skill defines the loop applied to each item so the protocol does not need to be re-explained per entry.

The list lives in the repo root by convention and is not committed — entries get removed as they're addressed. The same loop works on any flat-checkbox audit-style list, regardless of file name.

To *populate* such a list from raw feedback (prose, transcripts, pasted reviews, structured output), use `/audit-add`. This skill is the consumer; `/audit-add` is the producer.

## Per-item loop

Apply these steps in order to the top entry of the list. If a step blocks, stop and report — do not improvise around it.

1. **Sync the branch.** `git checkout main` (or the repo's default branch) and `git pull --ff-only` so work starts from current head. New branch is cut from the fetched default, never from the current working branch.

2. **Verify the issue is real and actionable.** Not by assumption. Write a test (or tests) that *fail* against the current code and replicate the reported behavior. If the test cannot be made to fail, the audit entry may be stale — stop, report, and offer to remove the entry without opening a PR.

   - For async or subscription code, follow `testing/go-backend-async` patterns: precise `wg.Add(N)`, callbacks only update state, no sleeps, no polling.
   - Tests must be fast and low-resource. Default GitHub free runners have constrained CPU/RAM — no long sleeps, no heavyweight load generators, no high goroutine counts unless essential.

3. **Implement the smallest fix that makes the failing test pass.** Don't expand scope. A bug fix doesn't need surrounding cleanup; one item per PR.

4. **Measure performance impact.** A throwaway benchmark or microbench to confirm the fix doesn't regress hot paths. Do not commit measurement code — strip it before opening the PR unless it has durable value as a permanent bench.

5. **Dead-code check on the changed files.** `deadcode ./...` or scoped to the touched packages. Delete unreachable AND unexported functions. Public-API "unreachable" findings are noise — library exports are reachable from external consumers.

6. **gofmt the changed files.** `gofmt -l` to detect drift, `gofmt -w` on the touched files only. Don't reformat unrelated files inside an audit-item PR — file project-wide formatting passes separately.

7. **Open issue + PR via `/gh`.** Let the router dispatch to `gh-issue-create` then `gh-issue-work`. Issue body is product-focused (no code identifiers); PR body describes the final state. The `plane` label is applied automatically by `gh-issue-create`.

8. **Hand back to the user.** Stop there. The user reviews and either gives feedback or says "next".

9. **On "next"** (or any equivalent advance signal): remove the just-merged entry from the list file, then restart at step 1 on the next top-of-list item. Continue until the list is empty.

## When an item turns out to be stale

If step 2 cannot produce a failing test, the audit entry was a false positive or has already been addressed elsewhere. Report:

- What was tried (the candidate failing test scenarios).
- Current code state at the cited file:line, showing the issue is not present.
- Recommendation: remove the entry without a PR, or downgrade priority.

Wait for explicit user confirmation before deleting the entry. No silent removals.

## When multiple items collapse into one PR

Occasionally two items share a single fix (e.g., a regex constant referenced from two places). If a single failing test proves both items and a single fix resolves both, bundle them. Note both in the PR body and remove both entries on "next". This is the exception — default is one item per PR.

## What this skill is not

- Not a router. `/gh` already routes between create / work / feedback / self-review / pr. This skill drives the *list* and hands off to `/gh` per item.
- Not a producer. `/audit-add` populates the list from raw feedback; this skill only consumes existing entries.
- Not a code-style enforcer. Per-file `gofmt` and `deadcode` checks are scoped to the audit-item PR; project-wide cleanups happen separately.
- Not a planner. Decisions about priority ordering live in the list itself (and `/audit-add` enforces the ordering rule on insert); this skill processes the top entry as-is.

## Guardrails

- Never start step 3 without a failing test from step 2. The truthseeker prove-before-acting rule applies.
- Never expand scope past the one entry being processed. Sibling concerns become separate entries.
- Never silently delete an audit entry. Even "stale" entries get explicit user confirmation before removal.
- Never push a `replace` directive against a local path to a long-lived branch. Local replace is fine for cross-repo testing but must be removed before the PR is ready for review.
- Stop and ask if the list file disappears mid-loop — it's an unwritten convention not to commit it, and an in-progress branch should not silently re-introduce it.

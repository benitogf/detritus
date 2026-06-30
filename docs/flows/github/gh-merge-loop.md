---
description: Watch a single PR on an interval — fix any review feedback via /gh-feedback-work, and merge once an approval lands on the latest commit. Terminating loop; merge is the exit.
triggers:
  - gh-merge-loop
  - watch pr
  - watch this pr
  - merge when approved
  - keep checking the pr
  - merge loop
  - auto-merge pr
argument-hint: "[pr] [interval]"
when: User wants a PR watched until it is merged — fix reviewer feedback as it arrives, then merge the moment an approval covers the latest commit. Accepts a full PR URL, `<owner>/<repo>#<n>`, or bare `#<n>` when cwd is a clone of the target repo.
related:
  - flows/github/gh
  - flows/github/gh-feedback-work
  - flows/github/gh-self-review
  - core/loop
  - core/janitor-platforms
---

# /gh-merge-loop — Watch a PR Until Merged

A watch-until-merge loop over **one** PR. Each tick re-checks the PR; if a reviewer posted feedback after the last commit, it fixes it (push + body rewrite); once an `APPROVED` review covers the current HEAD and the PR is mergeable with green required checks, it merges and stops.

This skill **composes** existing parts and adds only one new piece:

- **`/gh-feedback-work`** is the per-tick "fix it" action — dispatched, never reimplemented. It finds feedback posted after the last commit, classifies it, implements + tests, gates the push on a clean `/gh-self-review`, rewrites the PR body in place, and never posts comments. Its linked-issue preflight and attribution-footer rules are inherited.
- **`core/loop`** supplies the loop mechanics — re-fetch-live-state-every-tick, `/gh` delivery, usage-limit resilience, the skip-streak guardrail (reframed below).
- **`core/janitor-platforms`** owns the scheduler choice — this skill delegates there rather than hardcoding a primitive.
- The **only genuinely new logic** is the SHA-pinned approval merge gate. Everything else is borrowed.

Do not restate the internals of `/gh-feedback-work` or `core/loop` here — reference them and pass control.

## Inputs

Same input shapes as `/gh-feedback-work`. First match wins:

- Full GitHub PR URL: `https://github.com/<owner>/<repo>/pull/<n>`.
- Fully qualified: `<owner>/<repo>#<n>`.
- Bare `#<n>` — valid only when cwd is inside a clone of the target repo; derive `<owner>/<repo>` from `git remote get-url origin`.
- No PR argument → auto-detect the PR for the current branch: `gh pr view --json number,url --jq '{number,url}'` (or `gh api repos/<owner>/<repo>/pulls?head=<owner>:<branch>`). If none is found, ask which PR via `AskUserQuestion` — do not guess.

`[interval]` — the wake cadence, default **60s**. Human forms (`90s`, `5m`) are fine; pass them through to the platform adapter in human terms.

## Per-tick algorithm

**Re-fetch live state every tick — never act on a stale snapshot.** (`core/loop` → re-fetch-live-state.) Each wake:

1. **Fetch the live state.** Reviews (each review's `commit_id` + `state`), feedback posted after the last commit, `mergeable` / `mergeable_state`, required check-runs, and the HEAD SHA:

   ```
   # HEAD SHA + mergeability (pulls endpoint is the source for mergeable/mergeable_state)
   gh api repos/<owner>/<repo>/pulls/<n> --jq '{head_sha: .head.sha, mergeable, mergeable_state, merged, state, base: .base.ref}'

   # Reviews — each carries the commit_id it was submitted against + its state
   gh api --paginate repos/<owner>/<repo>/pulls/<n>/reviews --jq '.[] | {user: .user.login, state, commit_id, submitted_at}'

   # Required check-runs against HEAD
   gh api repos/<owner>/<repo>/commits/<head_sha>/check-runs --jq '.check_runs[] | {name, status, conclusion}'
   ```

   `mergeable` can be `null` while GitHub computes it; if so retry once after a few seconds before treating the tick as inconclusive. Feedback collection (the post-last-commit cutoff and the three comment sources) is owned by `/gh-feedback-work` Phase 1 — do not duplicate the queries here; dispatching to it in step 3 runs them.

2. **Merged or closed?** If `merged == true` (or `state == "closed"`) → **stop** and report the outcome (terminal: merged-success, or closed-without-merge).

3. **New feedback since the last commit?** If there is feedback posted after the last commit (the `/gh-feedback-work` Phase-1 timestamp cutoff) → **dispatch `/gh-feedback-work`** (fix → `/gh-self-review` → push → rewrite body). This pushes a commit, which **resets the merge gate** (any earlier approval is now stale — see below). Continue polling on the next tick. Let `/gh-feedback-work` own its own stop/hand-back behavior; if it cannot fold in a request (out-of-scope), handle per *Stuck = pause & hand back*.

4. **Approved on the current HEAD, mergeable, checks green?** Else if there is an `APPROVED` review whose `commit_id == HEAD SHA` **AND** the PR is mergeable **AND** all required checks are green → **merge** (see *Merge mechanics*) → **stop** (terminal: merged-success).

5. **Otherwise idle.** Waiting on a review, or the only approval is stale on an older SHA, or checks are still running → do nothing this tick; wake again next interval. (Track the skip wall-clock per *Skip-streak reframing*.)

## Merge gate — the SHA-pinned approval invariant (the one new piece)

A GitHub approval is bound to a commit (`review.commit_id`). **Merge ONLY when an `APPROVED` review's `commit_id` equals the current HEAD SHA.**

This is the load-bearing invariant of the whole skill — the "approve AFTER all the commits" crux:

- The moment the loop pushes a fix (step 3), HEAD advances and **any earlier approval is stale** — it covers an older SHA. A stale approval must be ignored until a fresh approval lands on the new HEAD.
- Merging on a pre-fix approval is the one way this skill can do real damage: it would merge a commit the human never approved. Guard it explicitly — `commit_id == HEAD SHA` is a hard precondition of the merge in step 4, not a heuristic.
- `state == "APPROVED"` alone is **not** sufficient. An approval on an old SHA + a later push = no merge.

## CI = gate only

The loop fixes posted **review feedback**, not pipelines.

- Required checks must be green for the merge gate to fire (step 4) — they gate the merge.
- The loop does **not** auto-fix a red pipeline. A persistently-red required check is a hand-back, not a fix target (see *Stuck = pause & hand back*).
- A flaky / still-running check is just "not green yet" — idle and re-check next tick (step 5). Distinguish "running" from "failed" via each check-run's `status` vs. `conclusion`.

## Merge mechanics

Merge with the repo's configured / allowed merge method — **never force a method the repo disallows.** Query the allowed methods first, then merge:

```
# Allowed merge methods + whether merged branches are auto-deleted
gh api repos/<owner>/<repo> --jq '{merge: .allow_merge_commit, squash: .allow_squash_merge, rebase: .allow_rebase_merge, delete_branch_on_merge}'

# Merge (set merge_method to an allowed one; --jq surfaces the result)
gh api -X PUT repos/<owner>/<repo>/pulls/<n>/merge -f merge_method=<merge|squash|rebase> -f sha=<head_sha> --jq '{merged, message}'
```

- Pass `sha=<head_sha>` so the merge applies to the exact commit the gate verified — if HEAD moved between the gate check and the merge call, GitHub rejects it rather than merging an unverified tree; re-run the tick.
- Pick an allowed `merge_method`. If the repo allows exactly one, use it; if it allows several, prefer the repo's configured default. Never pass a disallowed method.
- **Delete the head branch** after a successful merge **only if** the repo deletes merged branches (`delete_branch_on_merge == true`); otherwise leave it.
- **Respect branch protection.** Never use admin-override to merge past failing required checks or unmet required reviews — if protection blocks the merge, that is a hand-back, not a force. The merge gate already requires green checks + an approval, so a protection block here means state shifted under the tick; idle or hand back rather than override.

## Stuck = pause & hand back

Two conditions stop the loop and return control to the human — leave the PR untouched, report exactly what is blocking, never expand scope, never force-merge:

- **Out-of-scope review ask** — a reviewer requests something `/gh-feedback-work` will not fold in (it classifies the item out-of-scope). Stop; report the ask and offer the follow-up (`/gh-issue-create`) `/gh-feedback-work` surfaces. Do not stretch the PR to cover it.
- **Non-clean merge** — a conflict (`mergeable == false` / `mergeable_state == "dirty"`), or a persistently-red required check, or a branch-protection block the gate cannot satisfy. Stop; report what is unmergeable.

The human resolves and can relaunch the loop. Pausing is the safe default — the loop never forces past a blocker.

## Scheduling

**Delegate to `core/janitor-platforms`** — do not hardcode a scheduler. Load it, pick the adapter for the host, and report the effective cadence in human terms.

- **Default: the in-session loop (attended).** Be honest that a self-rescheduling driver (`/loop`-style) stops if the session ends or a usage limit kills a tick — see `core/loop` → *Usage-Limit Resilience*. Present it as office-hours / attended, not unattended-durable.
- **Resilient option: durable cron** (`CronCreate durable: true`) — a standing schedule that survives a usage-limit kill where `/loop` does not, seat-billed, against the local checkout. Offer this when the user wants the watch to survive a limit hit.
- **Cloud Routines are ineligible at 60s** — their floor is 1 hour, so a sub-hour interval is rejected at routine-creation time. Do not route a 60s watch there.

The interval arg defaults to 60s; if a chosen platform's minimum exceeds the requested interval, round up and state the effective cadence (per the platform adapter's rules).

## Skip-streak reframing

`core/loop`'s skip-streak guardrail trips at **eight idle ticks OR two hours of skip-only wakes, whichever comes first** — calibrated for `/janitor`, where that signals drift. **Neither limb fits here.** "No new feedback, no fresh approval" is the **normal waiting state** of this loop — at 60s while waiting on a human reviewer, every tick is a legitimate skip, so the eight-tick limb would trip after just eight minutes of normal waiting.

So the nag is a **long wall-clock idle threshold** — the two-hour floor recalibrated upward, and not gated on a tick count: after roughly N hours of skip-only wakes (default a few hours, configurable at setup), pause and ask — *"still waiting after N hours — keep watching / stop?"*. A tick that does real work (a fix dispatched, the PR merged) resets the idle clock.

## Terminal conditions

The loop ends on exactly one of:

- **Merged** — the merge gate fired and the PR merged (success).
- **PR closed without merge** — someone closed it; report and stop.
- **Pause & hand back** — out-of-scope review ask, a non-clean merge / conflict, or a persistently-red required check (see *Stuck = pause & hand back*). The PR is left untouched.
- **Max-duration cap (optional)** — if the user set a cap, stop at it and report the PR's current state.

## Guardrails

- **Never post comments.** Inherited from `/gh-feedback-work` — this skill writes only the PR body (via that skill) and performs the merge. No `POST .../comments`, ever.
- **The merge is the only outward action this skill adds.** Everything else is fixes + body rewrites, all owned by `/gh-feedback-work`.
- **Never merge on a stale approval.** The merge gate's `commit_id == HEAD SHA` precondition is non-negotiable — a push invalidates every prior approval.
- **Only the owned PR.** Act on the single target PR; never touch others.
- **Never force-merge, never admin-override, never expand scope.** A blocker is a hand-back, not something to power through.
- **Don't reimplement `/gh-feedback-work` or `core/loop`** — compose them. The new surface here is the merge gate and the merge call; the rest is delegation.

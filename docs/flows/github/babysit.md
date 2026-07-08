---
description: Watch a single PR on an interval — fix any review feedback via /gh-feedback-work, and merge once an approval lands on the latest commit. Terminating loop; merge is the exit.
triggers:
  - babysit
  - babysit pr
  - babysit this pr
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

# /babysit — Watch a PR Until Merged

A watch-until-merge loop over **one** PR. Each tick re-checks the PR; if a reviewer posted feedback after the last commit, it fixes it (push + body rewrite); once an `APPROVED` review covers the current HEAD and the PR's `mergeable_state` is `clean` (or `has_hooks`), it merges and stops.

This skill **composes** existing parts and adds only one new piece:

- **`/gh-feedback-work`** is the per-tick "fix it" action — dispatched, never reimplemented. It finds feedback posted after the last commit, classifies it, implements + tests, gates the push on a clean `/gh-self-review`, rewrites the PR body in place, and never posts comments. Its linked-issue preflight and attribution-footer rules are inherited.
- **`core/loop`** supplies the loop mechanics — re-fetch-live-state-every-tick, `/gh` delivery, usage-limit resilience, the skip-streak guardrail (reframed below).
- **`core/janitor-platforms`** owns the scheduler choice — this skill delegates there rather than hardcoding a primitive.
- The **only genuinely new logic** is the SHA-pinned approval merge gate. Everything else is borrowed.

Do not restate the internals of `/gh-feedback-work` or `core/loop` here — reference them and pass control.

> ## ⛔ The loop must never stall
>
> `/babysit` is a polling loop. Its **normal** state is "nothing changed this tick" — no new feedback, no fresh approval, not yet mergeable. **An idle tick is not a stop**: it re-arms the next wake and keeps polling. The canonical failure this contract exists to prevent is a tick that reads the PR, reports "still waiting", and **never arms the next wake** — the loop silently dying after one tick.
>
> Control returns to the user at exactly three points: **(a)** a terminal outcome — the PR merged, or was closed without merging; **(b)** a **pause & hand back** — an out-of-scope review ask or a non-clean merge the loop cannot resolve (*Stuck = pause & hand back*), or the **20-tick cap** (*Tick cap*); **(c)** an explicit user halt. **Everywhere else — including every ordinary idle tick — the tick MUST end with a live continuation path**: a standing schedule, a firing self-wake, or (on-demand) an explicit "re-invoke to continue" instruction (*Scheduling* → *Self-continuation*). Ending a tick with "waiting for review" and **none** of those three is the stall this contract exists to prevent.

## Inputs

Same input shapes as `/gh-feedback-work`. First match wins:

- Full GitHub PR URL: `https://github.com/<owner>/<repo>/pull/<n>`.
- Fully qualified: `<owner>/<repo>#<n>`.
- Bare `#<n>` — valid only when cwd is inside a clone of the target repo; derive `<owner>/<repo>` from `git remote get-url origin`.
- No PR argument → auto-detect the PR for the current branch with `gh api repos/<owner>/<repo>/pulls?head=<owner>:<branch> --jq '.[0] | {number, url: .html_url}'` — **`gh api`, never `gh pr view`** (per `flows/github/gh` convention #2: the Projects-classic GraphQL deprecation breaks `gh pr view` / `gh pr edit` on some repos even when the REST call works). If none is found, ask which PR via `AskUserQuestion` — do not guess.

`[interval]` — the wake cadence. **If no `[interval]` is passed, the default is 60 seconds (1 minute).** Human forms (`90s`, `5m`) are fine; pass them through to the platform adapter in human terms. Whatever the interval, the run is hard-capped at **20 ticks** (see *Tick cap*) — a longer interval buys more wall-clock, not more ticks.

## Per-tick algorithm

**Re-fetch live state every tick — never act on a stale snapshot.** (`core/loop` → re-fetch-live-state.) Each wake:

1. **Fetch the live state — including feedback.** Compute the last-commit cutoff first, then fetch reviews (for the gate), **all three feedback sources** (for detection), mergeability, required check-runs, and the HEAD SHA:

   ```
   # Last commit on the PR branch — the cutoff for "new" feedback (same rule as /gh-feedback-work Phase 1)
   gh api repos/<owner>/<repo>/pulls/<n>/commits --jq '.[-1].commit.committer.date'

   # HEAD SHA + mergeability (pulls endpoint is the source for mergeable/mergeable_state)
   gh api repos/<owner>/<repo>/pulls/<n> --jq '{head_sha: .head.sha, mergeable, mergeable_state, merged, state, base: .base.ref}'

   # Reviews — each carries the commit_id it was submitted against + its state (used by the merge gate AND as one feedback source)
   gh api --paginate repos/<owner>/<repo>/pulls/<n>/reviews --jq '.[] | {user: .user.login, state, commit_id, submitted_at, body}'

   # Inline review comments + issue-thread comments — the OTHER two feedback sources
   gh api --paginate repos/<owner>/<repo>/pulls/<n>/comments --jq '.[] | {user: .user.login, path, line, created_at, body}'
   gh api --paginate repos/<owner>/<repo>/issues/<n>/comments --jq '.[] | {user: .user.login, created_at, body}'

   # Required check-runs against HEAD (diagnostic only, not the gate)
   gh api repos/<owner>/<repo>/commits/<head_sha>/check-runs --jq '.check_runs[] | {name, status, conclusion}'
   ```

   **Feedback detection is the loop's own job — not something delegated away.** Build the feedback set from the three sources, **strictly after the last-commit cutoff** and excluding the current user's own comments (the `/gh-feedback-work` Phase 1 rule), with one crucial refinement — **key review bodies on `state`, not on presence**:

   - **Inline review comments** (`pulls/<n>/comments`) and **issue-thread comments** (`issues/<n>/comments`) → always candidate feedback.
   - **Review bodies** are feedback **only when `state` is `CHANGES_REQUESTED` or `COMMENTED`**. An **`APPROVED`** (or `DISMISSED`) review body is **not** feedback — an approval is a *merge signal*, handled by step 4. This is load-bearing: an approval's `commit_id == HEAD`, so it is always submitted *after* HEAD's commit and is therefore always post-cutoff. Counting its body as feedback would trip step 3 on every "LGTM"-style approve-with-comment, `/gh-feedback-work` would be dispatched with nothing actionable, no commit would push, the cutoff would never advance, and the same approval would re-trip every tick — **an approved, mergeable PR would never merge.** Key on state and that whole failure disappears.

   This detection read is what step 3 decides on; only the *fixing* is delegated to `/gh-feedback-work` (which re-runs its own Phase 1). **Detecting ≠ fixing** — do the detection queries every tick; never fall through to `mergeable_state` without them.

   `mergeable_state` can be `"unknown"` (and `mergeable` `null`) while GitHub computes mergeability; if so retry once after a few seconds. **If any feedback query errors or can't complete, the tick is *inconclusive* — re-arm and retry next tick; never treat an unread source as "no feedback."**

2. **Merged or closed?** If `merged == true` (or `state == "closed"`) → **stop** and report the outcome (terminal: merged-success, or closed-without-merge).

3. **Unaddressed change-requests since the last commit? (mandatory — this gate precedes the merge gate.)** If the step-1 feedback set is non-empty — an inline or issue comment, or a `CHANGES_REQUESTED`/`COMMENTED` review body, all post-cutoff and not your own (**an `APPROVED` body is not in this set** — see step 1) → **dispatch `/gh-feedback-work`** (fix → `/gh-self-review` → push → rewrite body). A pushed fix **resets the merge gate** (any earlier approval is now stale — see below). Re-arm and continue. An out-of-scope ask `/gh-feedback-work` won't fold in → *Stuck = pause & hand back*.

   **No-progress guard — keyed on `/gh-feedback-work`'s disposition, never on the bare no-commit signal.** A dispatch resolves one of three ways, and the guard must distinguish them by *what `/gh-feedback-work` reports*, not by "did a commit appear" (cases ii and iii both produce no commit):
   - **(i) actionable** → it implements + pushes → HEAD and the cutoff advance naturally. Nothing to guard.
   - **(ii) in-body / purely informational** — a question it answered in the PR body, or a non-actionable remark → **no commit, but the item is handled**. Advance the loop's feedback cutoff past it (record the handled-through timestamp in the loop's State — the same durable place as the tick count; in on-demand mode it rides in the report the user re-invokes with), so it does not re-trip, then proceed to the merge/idle gate. Without this, an in-body comment at HEAD would re-trip forever (the cutoff only advances on a commit).
   - **(iii) out-of-scope** → **`Stuck = pause & hand back`, always.** An out-of-scope ask is a standing block, **never** "acknowledged," and it **takes precedence over this guard** even though it too produces no commit. The guard never advances the loop past an out-of-scope change-request — doing so would merge past an unaddressed blocker whenever an approval happens to sit at HEAD.

   **The merge gate (step 4) is only reachable when the feedback set is empty — or every item in it was disposition-(ii) handled per the no-progress guard — AND was read successfully.** A single disposition-(iii) out-of-scope item keeps you out of the merge gate (it pauses per *Stuck*). Never advance to the merge/idle branches on a tick that skipped feedback detection or couldn't read a source — that tick is inconclusive, not "no feedback." Reading `mergeable_state` is never a substitute for reading the comments.

4. **Approved on the current HEAD and `mergeable_state` in `clean` / `has_hooks`?** Else if there is an `APPROVED` review whose `commit_id == HEAD SHA` **AND** `mergeable_state` is `"clean"` **or** `"has_hooks"` → **merge** (see *Merge mechanics*) → **stop** (terminal: merged-success). Gate on the composite `mergeable_state`, **not** the bare `mergeable` bool — `mergeable` is conflict-only (it stays `true` with an outstanding `REQUEST_CHANGES` or a failing check), whereas `clean` is GitHub's authoritative "this will merge now" state: no conflict, required checks passed, no unmet required review. `has_hooks` is `clean`'s sibling on repos with pre-receive hooks — such repos never report `clean`, so excluding it would leave a fully-approved green PR idling forever.

5. **Behind base?** Else if `mergeable_state == "behind"` (the base advanced and the repo requires branches be up to date before merge) → **update the branch from base** (`gh api -X PUT repos/<owner>/<repo>/pulls/<n>/update-branch`) and continue. Updating is a push, so it **resets the merge gate** — any prior approval is now stale; poll for a fresh approval on the new HEAD next tick. If the update surfaces a conflict (state flips to `dirty`), or `behind` recurs across many ticks without ever reaching approval (base thrash), hand back per *Stuck = pause & hand back*.

6. **Otherwise idle — but re-arm, don't stop.** Waiting on a review, the only approval is stale on an older SHA, or `mergeable_state` is not yet merge-ready (`clean`/`has_hooks`) for a transient reason (`unstable` / `unknown` — a check still running or mergeability not yet computed) → do no work this tick, **arm the next wake** (*Scheduling* → *Self-continuation*), and continue. This is the loop's normal state; it is **not** a terminal condition. (Every wake counts toward the *Tick cap*.)

**Every tick emits one status line** so the loop is observably alive and its decision is legible — e.g. `babysit #<n> tick 3/20 @<head7>: 0 new comments, approval=none, mergeable_state=blocked → idle, re-armed (60s)` or `… tick 4/20 … 2 new comments since <sha7> → dispatching /gh-feedback-work`. The line names: the **tick count (N/20)**, the count of new post-cutoff feedback items, the approval-vs-HEAD state, `mergeable_state`, the action taken, and — on any non-terminal tick — the continuation path (next wake armed, or the on-demand re-invoke instruction). A tick that reports without an action AND without a continuation path is the stall signature.

## Merge gate — the SHA-pinned approval invariant (the one new piece)

A GitHub approval is bound to a commit (`review.commit_id`). **Merge ONLY when an `APPROVED` review's `commit_id` equals the current HEAD SHA.**

This is the load-bearing invariant of the whole skill — the "approve AFTER all the commits" crux:

- The moment the loop pushes a fix (step 3), HEAD advances and **any earlier approval is stale** — it covers an older SHA. A stale approval must be ignored until a fresh approval lands on the new HEAD.
- Merging on a pre-fix approval is the one way this skill can do real damage: it would merge a commit the human never approved. Guard it explicitly — `commit_id == HEAD SHA` is a hard precondition of the merge in step 4, not a heuristic.
- `state == "APPROVED"` alone is **not** sufficient. An approval on an old SHA + a later push = no merge.

## CI = gate only

The loop fixes posted **review feedback**, not pipelines.

- Required checks gate the merge **through `mergeable_state`** (step 4 — `clean`, or `has_hooks` on hooked repos): GitHub reports these only once required checks have passed, so the composite state — not a local check-runs tally — is the gate. This is why the gate keys on the state, not `mergeable`: the bool ignores checks entirely, and a local check-runs query can't tell required from optional checks nor see legacy commit statuses.
- The loop does **not** auto-fix a red pipeline. A persistently-failing required check surfaces as `mergeable_state == "blocked"` and is a hand-back, not a fix target (see *Stuck = pause & hand back*).
- The check-runs query (step 1) is a **diagnostic**, not the gate — use it to explain *why* the state isn't merge-ready (distinguish a still-running check via `status` from a failed one via `conclusion`) so the idle-vs-hand-back call and the hand-back message are precise.

## Merge mechanics

Merge with the repo's configured / allowed merge method — **never force a method the repo disallows.** Query the allowed methods first, then merge:

```
# Allowed merge methods + whether merged branches are auto-deleted
gh api repos/<owner>/<repo> --jq '{merge: .allow_merge_commit, squash: .allow_squash_merge, rebase: .allow_rebase_merge, delete_branch_on_merge}'

# Merge (set merge_method to an allowed one; --jq surfaces the result)
gh api -X PUT repos/<owner>/<repo>/pulls/<n>/merge -f merge_method=<merge|squash|rebase> -f sha=<head_sha> --jq '{merged, message}'
```

- Pass `sha=<head_sha>` so the merge applies to the exact commit the gate verified — if HEAD moved between the gate check and the merge call, GitHub rejects it rather than merging an unverified tree; re-run the tick.
- On a `has_hooks` repo the pre-receive hook runs at this call and can still reject the merge — that rejection is a hand-back (report the hook's message), not something to retry past.
- Pick an allowed `merge_method`. If the repo allows exactly one, use it; if it allows several, prefer the repo's configured default. Never pass a disallowed method.
- **Delete the head branch** after a successful merge **only if** the repo deletes merged branches (`delete_branch_on_merge == true`); otherwise leave it.
- **Respect branch protection.** Never use admin-override to merge past failing required checks or unmet required reviews — if protection blocks the merge, that is a hand-back, not a force. The merge gate already requires `mergeable_state` `clean`/`has_hooks` + an approval on HEAD, so a protection block here means state shifted under the tick; idle or hand back rather than override.

## Detected feedback is a gate miss (incident)

Any review feedback this loop detects on the watched PR (step 3) is, on a detritus-authored PR, a blocker that survived the `/gh-self-review` gate — i.e. a **gate miss**, which per `core/ego` trigger ② is an incident. Babysit already routes the *fix* via `/gh-feedback-work`; the incident itself is captured per `core/ego` (→ `/absorb`) **after the fix lands**, so the merge-on-approval loop below is unaffected — the incident distillation rides alongside it, it does not gate the merge.

## Stuck = pause & hand back

Two conditions stop the loop and return control to the human — leave the PR untouched, report exactly what is blocking, never expand scope, never force-merge:

- **Out-of-scope review ask** — a reviewer requests something `/gh-feedback-work` will not fold in (it classifies the item out-of-scope). Stop; report the ask and offer the follow-up (`/gh-issue-create`) `/gh-feedback-work` surfaces. Do not stretch the PR to cover it.
- **Non-clean merge** — `mergeable_state` settles on a blocking value: `dirty` (conflict), or a persistently `blocked` state (a failing required check or an unmet branch-protection requirement the gate cannot satisfy). Stop; report what is unmergeable. (Transient non-clean states — `unstable` / `unknown` — are idle, not hand-back; see step 6. `behind` is neither idle nor an immediate hand-back — step 5 updates the branch and continues, handing back only if that update conflicts or thrashes. A non-required check that never goes green pins `unstable` indefinitely; that is bounded by the 20-tick cap (*Tick cap*), which pauses and hands back for the user to decide, not an infinite silent wait.)

The human resolves and can relaunch the loop. Pausing is the safe default — the loop never forces past a blocker.

## Scheduling

**Delegate to `core/janitor-platforms`** — do not hardcode a scheduler. Load it, pick the adapter for the host, and report the effective cadence in human terms.

- **Default: the in-session loop (attended).** Be honest that a self-rescheduling driver (`/loop`-style) stops if the session ends or a usage limit kills a tick — see `core/loop` → *Usage-Limit Resilience*. Present it as office-hours / attended, not unattended-durable.
- **Resilient option: durable cron** (`CronCreate durable: true`) — a standing schedule that survives a usage-limit kill where `/loop` does not, seat-billed, against the local checkout. Offer this when the user wants the watch to survive a limit hit.
- **Cloud Routines are ineligible at 60s** — their floor is 1 hour, so a sub-hour interval is rejected at routine-creation time. Do not route a 60s watch there.

The interval arg defaults to 60s; if a chosen platform's minimum exceeds the requested interval, round up and state the effective cadence (per the platform adapter's rules).

### Self-continuation (who fires the next tick)

Every non-terminal tick MUST guarantee the next one fires — this is what *The loop must never stall* enforces, and it is the direct fix for "the loop ran once and stopped." A **stall** is a tick that ends with **no continuation path at all**; it is *not* a stall to end a tick having stated exactly how the next one fires. Three arrangements are valid — use the first the host actually supports:

1. **Standing schedule (preferred; the only unattended mode).** A durable cron (`CronCreate durable: true`), an OS timer, or Routines fires each tick; the agent does its tick and yields. Resilient to a usage-limit kill (`core/loop` → *Usage-Limit Resilience*). Use this whenever the host offers it.
2. **Self-arm in-session** — `ScheduleWakeup` with the interval + a prompt that re-enters the loop on the same PR. Use **only if the host's self-wake actually fires** — it does **not** on every harness (some Claude Code sessions never deliver the wake). Don't keep pretending it will: if a self-wake doesn't land, fall to mode 3.
3. **On-demand (first-class fallback — the human is the scheduler).** When neither a standing schedule nor a firing self-wake is available (or the user prefers it), run **exactly one tick per invocation** and end by telling the user how to advance — e.g. *"tick N/20 — <status>; re-invoke `/babysit <pr>` (or nudge me) to run the next tick."* This is **not** a stall: the continuation path is explicit and the user's next invocation is the trigger. It is the honest mode when the timer doesn't fire — which, in practice, is common in an interactive session.

Confirm which arrangement is live before the first status line: "I'll keep checking" with no standing schedule, no firing self-wake, and no on-demand re-invoke instruction **is** the stall. Stop arming/soliciting the next tick **only** at the hand-back points in *The loop must never stall*.

## Tick cap

The run is hard-capped at **20 ticks — always**, independent of the interval and independent of whether ticks were idle or productive. **Count every wake** (idle, feedback-fix, branch-update — all of them); nothing resets the count. A longer interval buys more wall-clock per tick, never more ticks — 20 ticks at `60s` is ~20 minutes, at `5m` is ~100 minutes.

On the **20th tick**, do not run a 21st: **pause and hand back** — emit the status line, report the PR's current state and why it's still open (waiting on review / blocked / …), and tell the user to **re-invoke `/babysit <pr>` to watch for another 20 ticks**. This is a pause, not an abandonment — the PR is untouched and a re-invocation resumes with a fresh count of 20.

This cap **replaces** `core/loop`'s wall-clock skip-streak nag for this loop: a fixed tick count is the concrete, interval-independent bound `/babysit` uses instead of "drift after N idle ticks." (In on-demand mode each invocation is one tick, so the user reaches the cap after 20 manual re-invocations — same bound, same pause-and-hand-back.)

## Terminal conditions

The loop ends on exactly one of:

- **Merged** — the merge gate fired and the PR merged (success).
- **PR closed without merge** — someone closed it; report and stop.
- **Pause & hand back** — out-of-scope review ask, or a non-clean merge: `mergeable_state` `dirty` (conflict) or persistently `blocked` (failing required check / unmet protection) (see *Stuck = pause & hand back*). The PR is left untouched.
- **Tick cap reached** — 20 ticks elapsed; pause and hand back with a re-invoke instruction (see *Tick cap*). A fresh `/babysit <pr>` resumes with a new count.

## Guardrails

- **Hard cap: 20 ticks per run, always.** Count every wake regardless of interval or outcome; nothing resets it. At tick 20, pause and hand back with a re-invoke instruction (*Tick cap*) — never run a 21st on the same count. A longer interval never buys more ticks.
- **On-demand is a first-class mode, not a stall.** When no timer fires (common in an interactive session), run one tick per invocation and end with an explicit "re-invoke `/babysit <pr>` to continue" — the human is the scheduler (*Self-continuation* mode 3). Only a tick that ends with **no** continuation path (no schedule, no self-wake, no re-invoke instruction) is a stall.
- **An idle tick re-arms (or hands off on-demand); it never silently stops the loop.** "Waiting for review" is the normal state — end every such tick with a live continuation path (*Scheduling* → *Self-continuation*). Stopping without a terminal / hand-back / user-halt reason is this skill's #1 failure mode. A status line with no action and no continuation path is the tell.
- **Feedback detection runs every tick and precedes the merge gate.** Read all three sources (inline review comments, review bodies, issue-thread comments) filtered to post-last-commit, excluding your own — *then* evaluate the merge gate. Never act on `mergeable_state` alone; reading mergeability is not reading the comments. A tick that couldn't read a source is **inconclusive** — re-arm and retry, never assume "no feedback."
- **An approval is a merge signal, not feedback.** Key review bodies on `state`: only `CHANGES_REQUESTED`/`COMMENTED` bodies (plus inline + issue comments) count as feedback; an `APPROVED` body never trips the feedback gate — otherwise every approve-with-a-comment blocks the very merge it grants and the PR never lands. A feedback pass that pushes nothing is read-and-acknowledged **only** when `/gh-feedback-work` classifies it in-body / informational (no-progress guard case ii); an **out-of-scope** ask always pauses via *Stuck = pause & hand back* (case iii) and is never acknowledged-and-merged-past. Key the guard on the reported disposition, not on "no commit appeared."
- **Never post comments.** Inherited from `/gh-feedback-work` — this skill writes only the PR body (via that skill) and performs the merge. No `POST .../comments`, ever.
- **The merge is the only outward action this skill adds.** Everything else is fixes + body rewrites, all owned by `/gh-feedback-work`.
- **Never merge on a stale approval.** The merge gate's `commit_id == HEAD SHA` precondition is non-negotiable — a push invalidates every prior approval.
- **Only the owned PR.** Act on the single target PR; never touch others.
- **Never force-merge, never admin-override, never expand scope.** A blocker is a hand-back, not something to power through.
- **Don't reimplement `/gh-feedback-work` or `core/loop`** — compose them. The new surface here is the merge gate and the merge call; the rest is delegation.

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
  - flows/maintainer/learn
  - core/ego
  - core/loop
  - core/janitor-platforms
---

# /babysit — Watch a PR Until Merged

A watch-until-merge loop over **one** PR. An armed event-watch wakes the loop on each gate-state change; the loop re-checks the PR; if a reviewer posted feedback after the last commit, it fixes it (push + body rewrite); once an `APPROVED` review covers the current HEAD, the PR's `mergeable_state` is `clean` (or `has_hooks`), and no change-request is standing, it merges and stops.

This skill **composes** existing parts and adds only one new piece:

- **`/gh-feedback-work`** is the per-event "fix it" action — dispatched, never reimplemented. It finds feedback posted after the last commit, classifies it, implements + tests, gates the push on a clean `/gh-self-review`, rewrites the PR body in place, and never posts comments. Its linked-issue preflight and attribution-footer rules are inherited.
- **`core/loop`** supplies the loop mechanics — re-fetch-live-state-on-every-event, `/gh` delivery, usage-limit resilience.
- **`core/janitor-platforms`** owns the watch mechanism — this skill delegates the *how does it wait* (its **event-watch adapter**: an out-of-band poll that emits one event per gate-state change, so the model reacts to emitted events rather than polling itself) rather than hardcoding a primitive.
- The **only genuinely new logic** is the actor role: the SHA-pinned approval merge gate + the feedback-fix dispatch. Everything else — the wait, the fix, the delivery — is borrowed.

Do not restate the internals of `/gh-feedback-work` or `core/loop` here — reference them and pass control.

> ## ⛔ The loop must never stall
>
> `/babysit` is an event-driven loop: it **arms one event-watch** on the PR and reacts to each emitted gate event. Its **normal** state is "waiting" — the watch is armed and no event has arrived. **Waiting is not a stop**: the watch stays live and wakes the loop the moment something changes. The canonical failure this contract exists to prevent is a PR still open with **no event-watch armed** — the loop having yielded without a live watch running, so no future change will ever wake it.
>
> Control returns to the user at exactly three points: **(a)** a terminal outcome — the PR merged, or was closed without merging; **(b)** a **pause & hand back** — an out-of-scope review ask or a non-clean merge the loop cannot resolve (*Stuck = pause & hand back*), or the **hand-back timer** elapsing on total silence when the user set a time budget (*Hand-back timer*); **(c)** an explicit user halt. **Everywhere else — while the PR is still open — a live event-watch MUST be armed**: the automatic event-watch (the default), or, *only* when no watch primitive is available, an explicit **degraded-fallback** re-invoke hand-off (*Scheduling* → *Self-continuation*). Automatic is the default and normal path; on-demand is only an override or that degraded fallback — never what the loop silently settles into. Yielding on an open PR with **neither** an event-watch armed **nor** a degraded hand-off stated is the stall this contract exists to prevent.

## Inputs

Same input shapes as `/gh-feedback-work`. First match wins:

- Full GitHub PR URL: `https://github.com/<owner>/<repo>/pull/<n>`.
- Fully qualified: `<owner>/<repo>#<n>`.
- Bare `#<n>` — valid only when cwd is inside a clone of the target repo; derive `<owner>/<repo>` from `git remote get-url origin`.
- No PR argument → auto-detect the PR for the current branch with `gh api repos/<owner>/<repo>/pulls?head=<owner>:<branch> --jq '.[0] | {number, url: .html_url}'` — **`gh api`, never `gh pr view`** (per `flows/github/gh` convention #2: the Projects-classic GraphQL deprecation breaks `gh pr view` / `gh pr edit` on some repos even when the REST call works). If none is found, ask which PR via `AskUserQuestion` — do not guess.

`[interval]` — the event-watch's poll cadence. **If no `[interval]` is passed, the default is 60 seconds (1 minute).** Human forms (`90s`, `5m`) are fine; pass them through to the platform adapter in human terms. The interval sets how often the out-of-band watch polls the PR; the model only wakes on an actual change, so a shorter interval buys faster reaction, not more model turns. There is no fixed run cap — the watch runs until the PR is terminal or a user-supplied budget elapses in total silence (see *Hand-back timer*).

## Per-event algorithm

babysit no longer polls each tick — it **reacts to an emitted event** from the armed event-watch (a newly-terminal check, a new review, a new comment — inline or issue-thread, `gate: mergeable`, a CI failure, merged/closed). **The event is a hint, not authority: re-fetch live state on every wake — never act on the event line as a stale snapshot.** (`core/loop` → re-fetch-live-state.) On each emitted event:

1. **Fetch the live state — including feedback.** Compute the last-commit cutoff first, then fetch reviews (for the gate), **all three feedback sources** (for detection), mergeability, required check-runs, and the HEAD SHA. **The watch script uses these same endpoints** (`pulls`, `reviews`, `comments`, `issues/comments`, `commits/<sha>/check-runs`) to decide what to emit — so the woken model re-derives from the same source of truth the event came from:

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

   Then run the gates in order: **merged/closed → unaddressed feedback (incl. non-blockers on an approved PR) → SHA-pinned + no-`CHANGES_REQUESTED` merge gate → behind → idle.**

   **Feedback detection is the loop's own job — not something delegated away.** Read **all** comment content — inline review comments, issue-thread comments, and `CHANGES_REQUESTED`/`COMMENTED` review bodies — **regardless of approval state**, **strictly after the last-commit cutoff** and excluding the current user's own comments (the `/gh-feedback-work` Phase 1 rule). An approval never suppresses reading the *other* comments. **Key review bodies on `state`, not on presence** — one crucial refinement:

   - **Inline review comments** (`pulls/<n>/comments`) and **issue-thread comments** (`issues/<n>/comments`) → always candidate feedback.
   - **Review bodies** are feedback **only when `state` is `CHANGES_REQUESTED` or `COMMENTED`**. The single exemption: an **`APPROVED`** review's own body is **never** feedback — an approval is a *merge signal*, handled by step 4. This is load-bearing: an approval's `commit_id == HEAD`, so it is always submitted *after* HEAD's commit and is therefore always post-cutoff. Counting its own body as feedback would trip step 3 on every "LGTM"-style approve, `/gh-feedback-work` would be dispatched with nothing actionable, no commit would push, the cutoff would never advance, and the same approval would re-trip forever — **an approved, mergeable PR would never merge.** Keying the approval's *own body* out is what avoids that. But this exemption covers **only** the approval's own body: comments *accompanying* an approving review, and any `COMMENTED`-review or issue-comment content, ARE actionable and are read and addressed before the merge gate — an approve-with-comments ("approving, but rename this / fix this typo") does not get its comments silently dropped.

   This detection read is what step 3 decides on; only the *fixing* is delegated to `/gh-feedback-work` (which re-runs its own Phase 1). **Detecting ≠ fixing** — do the detection queries on every event; never fall through to `mergeable_state` without them.

   `mergeable_state` can be `"unknown"` (and `mergeable` `null`) while GitHub computes mergeability; if so retry once after a few seconds. **If any feedback query errors or can't complete, the wake is *inconclusive* — the watch stays armed and the next event (or a manual re-check) retries; never treat an unread source as "no feedback."**

2. **Merged or closed?** If `merged == true` (or `state == "closed"`) → **stop** and report the outcome (terminal: merged-success, or closed-without-merge).

3. **Unaddressed comments since the last commit? (mandatory — this gate precedes the merge gate.)** Gather **all** comment content — inline review comments + issue comments + `CHANGES_REQUESTED`/`COMMENTED` review bodies — post-cutoff and not your own, **regardless of approval state** (the one exemption: an `APPROVED` review's *own body* is not in this set; `DISMISSED` bodies are likewise out, per the state-keying in step 1), and run it through `/gh-feedback-work` first. **The merge gate is reachable only after that pass reports nothing actionable remains.** If the set is non-empty → **dispatch `/gh-feedback-work`** (fix → `/gh-self-review` → push → rewrite body). A pushed fix **resets the merge gate** (any earlier approval is now stale — it covered an older SHA; the code changed, so a re-approval on the new HEAD is honest and correct). Re-arm and continue. An out-of-scope ask `/gh-feedback-work` won't fold in → *Stuck = pause & hand back*.

   **No-progress guard — keyed on `/gh-feedback-work`'s disposition, never on the bare no-commit signal.** A dispatch resolves one of three ways, and the guard must distinguish them by *what `/gh-feedback-work` reports*, not by "did a commit appear" (cases ii and iii both produce no commit):
   - **(i) actionable** → it implements + pushes → HEAD and the cutoff advance naturally. Nothing to guard.
   - **(ii) in-body / purely informational** — a question it answered in the PR body, or a non-actionable remark → **no commit, but the item is handled**. Advance the loop's feedback cutoff past it (record the handled-through marker in the loop's State — wherever the active arrangement carries state forward: under the event-watch default, the in-session conversation context while the watch runs; under a durable cron, the schedule's carried-forward prompt/scratchpad; under the degraded fallback, the re-invoke status line the user carries forward across manual invocations), so it does not re-trip, then proceed to the merge/idle gate. Without this, an in-body comment at HEAD would re-trip forever (the cutoff only advances on a commit).
   - **(iii) out-of-scope** → **`Stuck = pause & hand back`, always.** An out-of-scope ask is a standing block, **never** "acknowledged," and it **takes precedence over this guard** even though it too produces no commit. The guard never advances the loop past an out-of-scope change-request — doing so would merge past an unaddressed blocker whenever an approval happens to sit at HEAD.

   **The merge gate (step 4) is only reachable when the feedback set is empty — or every item in it was disposition-(ii) handled per the no-progress guard — AND was read successfully.** A single disposition-(iii) out-of-scope item keeps you out of the merge gate (it pauses per *Stuck*). Never advance to the merge/idle branches on a wake that skipped feedback detection or couldn't read a source — that wake is inconclusive, not "no feedback." Reading `mergeable_state` is never a substitute for reading the comments.

4. **Approved on the current HEAD, `mergeable_state` in `clean` / `has_hooks`, and no standing `CHANGES_REQUESTED`?** Else if there is an `APPROVED` review whose `commit_id == HEAD SHA` **AND** `mergeable_state` is `"clean"` **or** `"has_hooks"` **AND** no reviewer holds a standing `CHANGES_REQUESTED` → **merge** (see *Merge mechanics*) → **stop** (terminal: merged-success). Gate on the composite `mergeable_state`, **not** the bare `mergeable` bool — `mergeable` is conflict-only (it stays `true` with an outstanding `REQUEST_CHANGES` or a failing check), whereas `clean` is GitHub's authoritative "this will merge now" state: no conflict, required checks passed, no unmet required review. `has_hooks` is `clean`'s sibling on repos with pre-receive hooks — such repos never report `clean`, so excluding it would leave a fully-approved green PR idling forever. **Standing `CHANGES_REQUESTED`** is the third leg, and it is **commit-id-independent** (unlike the approval SHA-pin): per reviewer, take that reviewer's latest **verdict** review by `submitted_at` — considering only `APPROVED` / `CHANGES_REQUESTED` / `DISMISSED`, and **ignoring `COMMENTED` (and `PENDING`) reviews**; if that verdict is `CHANGES_REQUESTED`, it is standing and **blocks the merge regardless of its `commit_id`** — a GitHub change-request persists through pushes until the reviewer *approves* or it is *dismissed*, so it does not need to sit at HEAD to block, and a later comment-only review does **not** clear it. On a repo *without* required-review branch protection, a `CHANGES_REQUESTED` does **not** move `mergeable_state` off `clean`, so without this leg the gate could merge past an unaddressed change-request (same family as the read-all-comments fix in step 3) — see *Merge gate*.

5. **Behind base?** Else if `mergeable_state == "behind"` (the base advanced and the repo requires branches be up to date before merge) → **update the branch from base** (`gh api -X PUT repos/<owner>/<repo>/pulls/<n>/update-branch`) and continue. Updating is a push, so it **resets the merge gate** — any prior approval is now stale; the next emitted event on the new HEAD is when a fresh approval can be checked. If the update surfaces a conflict (state flips to `dirty`), or `behind` recurs repeatedly without ever reaching approval (base thrash), hand back per *Stuck = pause & hand back*.

6. **Otherwise idle — but keep the watch armed, don't stop.** Waiting on a review, the only approval is stale on an older SHA, or `mergeable_state` is not yet merge-ready (`clean`/`has_hooks`) for a transient reason (`unstable` / `unknown` — a check still running or mergeability not yet computed) → do no work on this event, **confirm the event-watch is still armed** (*Scheduling* → *Self-continuation*), and yield. This is the loop's normal state; it is **not** a terminal condition.

**Every event emits one status line** so the loop is observably alive and its decision is legible — e.g. `babysit #<n> @<head7>: check ci/build → conclusion=failure → non-clean, handing back` or `… @<head7>: 2 new comments since <sha7> → dispatching /gh-feedback-work; watch armed`. The line names: the **event just handled**, the count of new post-cutoff feedback items, the approval-vs-HEAD state, `mergeable_state`, the action taken, and — on any non-terminal wake — the continuation path: **the live event-watch confirmed armed**, or, in degraded fallback, the labeled re-invoke hand-off. A wake that reports without an action AND without a confirmed live watch (or degraded hand-off) is the stall signature.

## Merge gate — the SHA-pinned approval invariant (the one new piece)

A GitHub approval is bound to a commit (`review.commit_id`). **Merge ONLY when an `APPROVED` review's `commit_id` equals the current HEAD SHA.**

This is the load-bearing invariant of the whole skill — the "approve AFTER all the commits" crux:

- The moment the loop pushes a fix (step 3), HEAD advances and **any earlier approval is stale** — it covers an older SHA. A stale approval must be ignored until a fresh approval lands on the new HEAD.
- Merging on a pre-fix approval is the one way this skill can do real damage: it would merge a commit the human never approved. Guard it explicitly — `commit_id == HEAD SHA` is a hard precondition of the merge in step 4, not a heuristic.
- `state == "APPROVED"` alone is **not** sufficient. An approval on an old SHA + a later push = no merge.

**No standing `CHANGES_REQUESTED`** is the third leg of the composite gate, alongside `APPROVED@HEAD` and `mergeable_state ∈ {clean, has_hooks}`. On a repo *without* required-review branch protection, a `CHANGES_REQUESTED` review does **not** move `mergeable_state` off `clean` — so without this leg the gate would happily merge past an unaddressed change-request whenever a later `APPROVED` from another reviewer sits at HEAD.

Crucially, this leg is **commit-id-independent — the opposite of the approval SHA-pin.** A GitHub `CHANGES_REQUESTED` persists through pushes until its author *approves* or it is *dismissed*; a later **comment-only (`COMMENTED`) review does not clear it**, and it does **not** need to cover HEAD to block. So a change-request submitted against an *older* commit is exactly the never-addressed blocker this leg guards, and pinning it to HEAD (as the approval is pinned) would wave it straight through. Evaluate it per reviewer over that reviewer's latest **verdict** review by `submitted_at` — considering only `APPROVED` / `CHANGES_REQUESTED` / `DISMISSED`, ignoring `COMMENTED` and `PENDING`: if that verdict is `CHANGES_REQUESTED`, it is standing and blocks the merge irrespective of its `commit_id`. The approval SHA-pin is unchanged — only this CR leg is commit-id-independent.

## CI = gate only

The loop fixes posted **review feedback**, not pipelines.

- Required checks gate the merge **through `mergeable_state`** (step 4 — `clean`, or `has_hooks` on hooked repos): GitHub reports these only once required checks have passed, so the composite state — not a local check-runs tally — is the gate. This is why the gate keys on the state, not `mergeable`: the bool ignores checks entirely, and a local check-runs query can't tell required from optional checks nor see legacy commit statuses.
- The loop does **not** auto-fix a red pipeline. When a `gate: ci-failed` event wakes the loop, re-fetch confirms the state: a required check whose `conclusion` has settled on `failure` surfaces as `mergeable_state == "blocked"` and is a hand-back, not a fix target (see *Stuck = pause & hand back*); a check still `in_progress` is idle (step 6), not a hand-back. The decision is made from the re-fetched state at the wake, not from counting failures across polls.
- The check-runs query (step 1) is a **diagnostic**, not the gate — use it to explain *why* the state isn't merge-ready (distinguish a still-running check via `status` from a failed one via `conclusion`) so the idle-vs-hand-back call and the hand-back message are precise.

## Merge mechanics

Merge with the repo's configured / allowed merge method — **never force a method the repo disallows.** Query the allowed methods first, then merge:

```
# Allowed merge methods + whether merged branches are auto-deleted
gh api repos/<owner>/<repo> --jq '{merge: .allow_merge_commit, squash: .allow_squash_merge, rebase: .allow_rebase_merge, delete_branch_on_merge}'

# Merge (set merge_method to an allowed one; --jq surfaces the result)
gh api -X PUT repos/<owner>/<repo>/pulls/<n>/merge -f merge_method=<merge|squash|rebase> -f sha=<head_sha> --jq '{merged, message}'
```

- Pass `sha=<head_sha>` so the merge applies to the exact commit the gate verified — if HEAD moved between the gate check and the merge call, GitHub rejects it rather than merging an unverified tree; re-check on the next event.
- On a `has_hooks` repo the pre-receive hook runs at this call and can still reject the merge — that rejection is a hand-back (report the hook's message), not something to retry past.
- Pick an allowed `merge_method`. If the repo allows exactly one, use it; if it allows several, prefer the repo's configured default. Never pass a disallowed method.
- **Delete the head branch** after a successful merge **only if** the repo deletes merged branches (`delete_branch_on_merge == true`); otherwise leave it.
- **Respect branch protection.** Never use admin-override to merge past failing required checks or unmet required reviews — if protection blocks the merge, that is a hand-back, not a force. The merge gate already requires `mergeable_state` `clean`/`has_hooks` + an approval on HEAD, so a protection block here means state shifted under the wake; idle or hand back rather than override.

## Detected feedback is a gate miss (incident)

Any review feedback this loop detects on the watched PR (step 3) is, on a detritus-authored PR, a blocker that survived the `/gh-self-review` gate — i.e. a **gate miss**, which per `core/ego` trigger ② is an incident. Babysit already routes the *fix* via `/gh-feedback-work`; the incident itself is captured per `core/ego` (→ `/absorb`) **after the fix lands**, so the merge-on-approval loop below is unaffected — the incident distillation rides alongside it, it does not gate the merge.

## Post-merge: route the producing unit's incidents to `/learn` (non-gating)

When the loop reaches its **Merged** terminal outcome (step 4), and only then, run one **post-merge companion read** on a 🍬-footered PR — the merge has already landed, so this **never gates, delays, or re-opens the merge**. It rides alongside the terminal, exactly like *Detected feedback is a gate miss* above: the merge is the deliverable; the learning capture is a companion.

- **Footered PRs only.** If the merged PR body carries the candyland unit-ref footer `🍬 Opened by candyland — <kind> \`<id>\``, resolve the producing unit from it; **compose the canonical PR→unit recipe in `flows/maintainer/learn`** (footer fast-path, reverse-telemetry-lookup fallback) — do not restate it here. No footer and no reverse-lookup match → the PR was not candyland-produced; there are no unit incidents to read, so skip silently.
- **Read the unit's `incidents[]`.** The producing unit records its self-acknowledged incident notes (`core/ego` trigger ①) in its candyland record — the tech-lead **records, never routes** them in the sidecar (`roles/tech-lead`), precisely so a user-session flow like `/babysit` can surface them afterward.
- **Route → `/learn`, never `/grow`/`/absorb`.** These are telemetry-borne, agent-attributed mechanisms across the merged unit — `/learn`'s source, not a live session correction (`/grow`) nor this PR's review outcome (`/absorb`). Surface the incidents and offer `/learn <pr-url>` (a PR argument works standalone — `flows/maintainer/learn` → *resolve a PR → its producing unit*); routing is the user's post-delivery call per `core/ego`, so **offer it, don't auto-run it**. An empty `incidents[]` is the normal case — report the clean merge and stop.

This is strictly downstream of the merge: it cannot change the merge gate, cannot keep the loop alive, and cannot turn a merged-success into a hand-back.

## Stuck = pause & hand back

Two conditions stop the loop and return control to the human — leave the PR untouched, report exactly what is blocking, never expand scope, never force-merge:

- **Out-of-scope review ask** — a reviewer requests something `/gh-feedback-work` will not fold in (it classifies the item out-of-scope). Stop; report the ask and offer the follow-up (`/gh-issue-create`) `/gh-feedback-work` surfaces. Do not stretch the PR to cover it.
- **Non-clean merge** — the re-fetched `mergeable_state` at the wake is a blocking value: `dirty` (conflict), or `blocked` where the cause has settled (a required check whose `conclusion == failure`, or an unmet branch-protection requirement the gate cannot satisfy). Stop; report what is unmergeable. (Transient non-clean states — `unstable` / `unknown`, or a required check still `in_progress` — are idle, not hand-back; see step 6. `behind` is neither idle nor an immediate hand-back — step 5 updates the branch and continues, handing back only if that update conflicts or thrashes. A non-required check that never goes green leaves the PR `unstable`; the watch simply keeps waiting on it — the loop never abandons an active PR on its own. If the user set a time budget, total silence across the whole budget window hands back for them to decide (*Hand-back timer*).)

The human resolves and can relaunch the loop. Pausing is the safe default — the loop never forces past a blocker.

## Scheduling

**Delegate to `core/janitor-platforms`** — do not hardcode a watch primitive. Load it, pick the adapter for the host, report the effective cadence in human terms. These arrangements map one-to-one onto *Self-continuation*; take the first the host supports.

- **Preferred default: the event-watch adapter** (`core/janitor-platforms` → its *event-watch* adapter) — an out-of-band poll that wakes the model only on a gate-state change. In-session, attended, and **token-cheap** (the model is idle between events — cost ∝ events, not wall-clock). `/babysit` reacts to changes on a single remote target, which is exactly the event-watch shape — so this is the default whenever the host offers it.
- **Across-session opt-in: a durable standing schedule** (`CronCreate durable: true`, an OS timer, or Routines) — survives session end / a usage-limit kill (`core/loop` → *Usage-Limit Resilience*), but **re-polls per fire** (a full model turn each fire, so the event-watch's token win does **not** apply) and must carry the last-seen state forward across fires so it still acts only on *new* changes. Choose this only when the watch must survive session death (the event-watch dies with the session — `/clear`, closed terminal, sleep).
- **Last resort: on-demand.** If the host can arm neither of the above, degrade to on-demand per *Self-continuation* (one check per invocation, flagged degraded) — never refuse to run.
- **Cloud Routines are ineligible at 60s** — their floor is 1 hour, so a sub-hour cadence is rejected at routine-creation time. Do not route a 60s watch there.

The interval arg defaults to 60s (the watch's poll cadence); if a chosen platform's minimum exceeds the requested interval, round up and state the effective cadence (per the platform adapter's rules).

### Self-continuation (what wakes you on change)

`/babysit` is **automatic by default** — its normal operation is one armed event-watch that wakes the model on each change, with no user action in between; the loop never *defaults* to "check once, wait for a nudge." On-demand exists only as an **override** and a **degraded fallback** (below), never as the default continuation. **Automatic arrangement** — use the first the host supports:

1. **Event-watch (preferred; in-session, token-cheap).** Arm **one** event-watch on the PR (`core/janitor-platforms` → its *event-watch* adapter); it wakes the model on each gate-state change and is idle otherwise. This is the default whenever the host offers it.
2. **Durable standing schedule (across-session opt-in).** A durable cron / OS timer / Routines that re-polls per fire and carries last-seen state forward — resilient to a usage-limit kill (`core/loop` → *Usage-Limit Resilience*) but not token-cheap. Use when the watch must outlive the session.

Under either automatic arrangement, while the PR is open a live watch MUST be armed; yielding on an open PR with no watch armed **and** no degraded-fallback hand-off (below) is the stall this contract forbids.

### On-demand: override and fallback (never the default)

- **Override — "check now."** While an automatic watch is live, the user may invoke `/babysit <pr>` (or nudge) to run **one extra check immediately**, without waiting for the next emitted event — e.g. "I just approved, check now, don't wait." The armed watch keeps running; the override is an additional user-initiated check, **not** a replacement for the automatic continuation.
- **Fallback — degrade, don't refuse.** If the host can arm **neither** automatic arrangement, do **not** refuse to run — **degrade to on-demand**: run one check per invocation and end with an explicit, clearly-labeled hand-off — *"degraded manual mode (no watch primitive on this host): re-invoke `/babysit <pr>` to check again, or set up a durable cron for an unattended watch."* This is a legitimate continuation path (the user's re-invocation), but it is a **last resort**, flagged as degraded every time — never presented as normal operation. (Same honest-degradation spirit as `core/loop` → *Durability*: surface the mode mismatch, don't silently misbehave.)

The line to hold: **automatic is the default and the normal path (an armed event-watch); on-demand is an override the user reaches for, or a degraded fallback when no watch primitive exists — never what the loop silently settles into as its standard continuation.** Confirm which is live before the first status line; "I'll keep checking" with no watch armed **and** no degraded-fallback hand-off stated is the stall. Stop arming/soliciting a watch **only** at the hand-back points in *The loop must never stall*.

## Hand-back timer

**No idle-timeout hand-back by default.** An active PR is never abandoned — with no time budget set, the watch stays armed indefinitely, reacting to every change until the PR is terminal (or the user halts).

Hand back on silence **only if the user supplies a time budget**, and **the timer resets on every event**. So the watch hands back only after the *whole budget window* elapses with **no** event at all — a genuinely stalled PR (nobody reviewing, no CI moving) — not after a fixed number of wakes. Any emitted event (a check landing, a comment, a mergeability transition) resets the clock; the budget measures *silence*, not elapsed wall-clock.

On the budget elapsing in total silence: **pause and hand back** — emit the status line, report the PR's current state and why it's still open (waiting on review / blocked / …), and tell the user to **re-invoke `/babysit <pr>`** to resume watching. This is a pause, not an abandonment — the PR is untouched and a re-invocation re-arms the watch.

This supersedes `core/loop`'s wall-clock skip-streak nag for this loop: `/babysit`'s bound is the user-set silence budget (resetting on every event), not "drift after N idle ticks." Reaching the budget is a hand-back, not an on-demand mode — a fresh `/babysit <pr>` re-arms the automatic watch.

## Terminal conditions

The loop ends on exactly one of:

- **Merged** — the merge gate fired and the PR merged (success). On a 🍬-footered PR, a non-gating post-merge read then routes the producing unit's `incidents[]` to `/learn` (see *Post-merge: route the producing unit's incidents to `/learn`*); this rides alongside the terminal and never changes the outcome.
- **PR closed without merge** — someone closed it; report and stop.
- **Pause & hand back** — out-of-scope review ask, or a non-clean merge: `mergeable_state` `dirty` (conflict) or settled `blocked` (failing required check / unmet protection) (see *Stuck = pause & hand back*). The PR is left untouched.
- **Silence budget elapsed** — *only if the user set a time budget* and the whole window passed with no event (see *Hand-back timer*); pause and hand back with a re-invoke instruction. A fresh `/babysit <pr>` re-arms the watch. With no budget set, this terminal does not exist — the watch waits indefinitely.

## Guardrails

- **No idle-timeout hand-back by default; a user-set silence budget resets on every event.** An active PR is never abandoned. If the user gives a time budget, hand back only after the *whole window* passes with no event (*Hand-back timer*) — every event resets the clock; the budget measures silence, not wall-clock. With no budget, the watch waits indefinitely.
- **Automatic is the default; on-demand is override/fallback only, never the default.** The loop's normal continuation is an armed event-watch that wakes it on change. On-demand is available *only* as (a) a manual "check now" **override** on top of a live watch, or (b) a **degraded fallback** when the host can arm no watch primitive — run one check per invocation, flagged degraded every time, recommending a real watch. Never let the loop *default* to one-check-per-invocation, and never *refuse to run* just because no watch primitive exists — degrade instead (*Scheduling* → *Self-continuation*).
- **A wait continues the loop; it never silently stops.** Under the automatic default the event-watch stays armed; in degraded fallback the wake ends with the labeled re-invoke hand-off. "Waiting for review" is the normal state. Yielding on an open PR without a terminal / hand-back / user-halt reason — and with no armed watch — is this skill's #1 failure mode; a status line with no action and no confirmed live watch is the tell.
- **Feedback detection runs on every event and precedes the merge gate.** Read all three sources (inline review comments, review bodies, issue-thread comments) filtered to post-last-commit, excluding your own, **regardless of approval state** — *then* evaluate the merge gate. Never act on `mergeable_state` alone; reading mergeability is not reading the comments. A wake that couldn't read a source is **inconclusive** — the watch stays armed and the next event retries, never assume "no feedback."
- **An approval is a merge signal, not feedback — but it never suppresses reading the other comments.** Key review bodies on `state`: only `CHANGES_REQUESTED`/`COMMENTED` bodies (plus inline + issue comments) count as feedback, and an `APPROVED` review's *own body* never trips the feedback gate — otherwise every approve-with-a-comment blocks the very merge it grants and the PR never lands. But that exemption is the approval's own body **only**: comments accompanying an approval, and all inline/issue comments, ARE read and addressed before the merge gate — an approval does not let a reviewer's "approving, but fix this" be silently dropped. A feedback pass that pushes nothing is read-and-acknowledged **only** when `/gh-feedback-work` classifies it in-body / informational (no-progress guard case ii); an **out-of-scope** ask always pauses via *Stuck = pause & hand back* (case iii) and is never acknowledged-and-merged-past. Key the guard on the reported disposition, not on "no commit appeared."
- **Never post comments.** Inherited from `/gh-feedback-work` — this skill writes only the PR body (via that skill) and performs the merge. No `POST .../comments`, ever.
- **The merge is the only outward action this skill adds.** Everything else is fixes + body rewrites, all owned by `/gh-feedback-work`.
- **Post-merge incident routing never gates the merge.** On merged-success for a 🍬-footered PR, the producing unit's `incidents[]` are surfaced and offered to `/learn` as a downstream companion — it runs only after the merge lands, offers (never auto-runs) `/learn`, and can never delay, block, or re-open the merge (*Post-merge: route the producing unit's incidents to `/learn`*).
- **Never merge on a stale approval.** The merge gate's `commit_id == HEAD SHA` precondition is non-negotiable — a push invalidates every prior approval.
- **Only the owned PR.** Act on the single target PR; never touch others.
- **Never force-merge, never admin-override, never expand scope.** A blocker is a hand-back, not something to power through.
- **Don't reimplement `/gh-feedback-work` or `core/loop`** — compose them. The new surface here is the merge gate and the merge call; the rest is delegation.

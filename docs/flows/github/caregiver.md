---
description: Watch a single PR from the reviewer's seat — re-review on each push or discussion by dispatching the one-shot /gh-pr, re-posting a commit-pinned verdict only when it materially changes, until the PR is merged or closed.
triggers:
  - caregiver
  - review watch
  - keep re-reviewing
  - watch the review
  - re-review on push
argument-hint: "[pr] [interval]"
when: User wants a single PR watched from the reviewer's seat — re-reviewed on each push or new discussion, with a fresh verdict re-posted only when it materially changes, until the PR is merged or closed. Accepts a full PR URL, `<owner>/<repo>#<n>`, or bare `#<n>` when cwd is a clone of the target repo.
related:
  - flows/github/gh-pr
  - flows/github/gh
  - flows/github/babysit
  - core/review-rigor
  - core/janitor-platforms
---

# /caregiver — Watch a PR From the Reviewer's Seat

A watch loop over **one** PR that mirrors `/babysit` — but on the **reviewer's** seat rather than the author's. An armed event-watch wakes the loop on each change to the PR; the loop re-reviews and re-posts a verdict when that verdict materially changes; it runs until the PR is merged or closed. `/babysit` watches to **merge** (fixing feedback, merging on a SHA-pinned approval); `/caregiver` watches to **re-review** (re-running the review, re-posting a verdict). Same watch shape, different actor.

This skill **composes** existing parts and adds only one new piece:

- **`/gh-pr` (one-shot)** is the per-event "review it" action — **dispatched, never reimplemented**. A dispatch behaves exactly like a manual `/gh-pr`: it resolves the PR, runs the review rigor, computes an `APPROVE`/`REQUEST_CHANGES` verdict, and returns the working tree to the default branch — `/gh-pr` posts that verdict via `gh api` at its **Phase 6**. `/caregiver` owns none of the review internally; what it adds is a decision *around* the dispatch — *when* to dispatch, and **whether that dispatch's Phase-6 post fires**: the initial baseline dispatch posts normally, but on each re-review `/caregiver` applies the material-verdict-change gate (below) to suppress an identical re-post. The verdict computation is `/gh-pr`'s; the post decision is `/caregiver`'s.
- **`core/janitor-platforms`** owns the watch mechanism — this skill delegates the *how does it wait* (its **event-watch adapter**: an out-of-band poll that emits one event per change, so the model reacts to emitted events rather than polling itself) rather than hardcoding a primitive. `/caregiver` is the adapter's **review-watch** consumer; `/babysit` is its merge-gate consumer. Never name the underlying watch primitive here — the adapter owns it (portability rule; `/babysit` delegates the same way).
- **`core/review-rigor` → *Re-review continuity*** supplies the cross-event review discipline — held context (the diff understanding, brief, and evidence trail already established across earlier events), fresh evidence (re-diff the new commits, re-verify against the live tree). Each dispatched re-review is a **delta re-verify, not a cold re-derive**; the verdict-integrity rules apply unchanged.
- The **only genuinely new logic** is the actor role: the **material-verdict-change gate** — re-post a fresh `commit_id`-pinned verdict only when the verdict materially changes, re-review silently otherwise. Everything else — the wait, the review — is borrowed.

Do not restate the internals of `/gh-pr` or `core/janitor-platforms` here — reference them and pass control. The structure, no-stall contract, hand-back timer, and scheduling/self-continuation below **mirror `flows/github/babysit`** and are referenced there where identical; only the actor differs (reviewer, not author).

> ## ⛔ The watch must never stall
>
> `/caregiver` is an event-driven loop: it **arms one event-watch** on the PR and reacts to each emitted event. Its **normal** state is "waiting" — the watch is armed and no event has arrived. **Waiting is not a stop**: the watch stays live and wakes the loop the moment the PR changes. The canonical failure this contract exists to prevent is a PR still open with **no event-watch armed** — the loop having yielded without a live watch running, so no future push or discussion will ever wake it.
>
> Control returns to the user at exactly three points: **(a)** a terminal outcome — the PR merged, or closed without merging; **(b)** a **pause & hand back** — the **hand-back timer** elapsing on total silence when the user set a time budget (*Hand-back timer*); **(c)** an explicit user halt. **Everywhere else — while the PR is still open — a live event-watch MUST be armed**: the automatic event-watch (the default), or, *only* when no watch primitive is available, an explicit **degraded-fallback** re-invoke hand-off (*Scheduling* → *Self-continuation*). Automatic is the default and normal path; on-demand is only an override or that degraded fallback — never what the loop silently settles into. Yielding on an open PR with **neither** an event-watch armed **nor** a degraded hand-off stated is the stall this contract forbids. This mirrors `flows/github/babysit` → *The loop must never stall*, on the reviewer's seat.

## Inputs

Same input shapes as `/gh-pr`. First match wins:

- Full GitHub PR URL: `https://github.com/<owner>/<repo>/pull/<n>`.
- Fully qualified: `<owner>/<repo>#<n>`.
- Bare `#<n>` — valid only when cwd is inside a clone of the target repo; derive `<owner>/<repo>` from `git remote get-url origin`.
- No PR argument → auto-detect the PR for the current branch with `gh api repos/<owner>/<repo>/pulls?head=<owner>:<branch> --jq '.[0] | {number, url: .html_url}'` — **`gh api`, never `gh pr view`** (per `flows/github/gh` convention #2). If none is found, ask which PR via `AskUserQuestion` — do not guess.

`[interval]` — the event-watch's poll cadence. **If no `[interval]` is passed, the default is 60 seconds (1 minute)** — same default as `/babysit`. Human forms (`90s`, `5m`) are fine; pass them through to the platform adapter in human terms. The interval sets how often the out-of-band watch polls the PR; the model only wakes on an actual change, so a shorter interval buys faster reaction, not more model turns. There is no fixed run cap — the watch runs until the PR is terminal or a user-supplied budget elapses in total silence (see *Hand-back timer*).

## Start behavior

Establish a baseline verdict before arming, so the material-change gate has something to compare against:

- **No standing verdict from the current gh user at HEAD?** → **dispatch `/gh-pr` once** for the initial verdict (a normal one-shot review + post). Record that verdict — its tier **and** its blocker set — plus the HEAD SHA it covered as the baseline.
- **A standing verdict from the current gh user already covers HEAD?** → **adopt it as the baseline** (tier + blocker set + SHA); do not re-post it. Re-reading the existing review's outcome is enough.

Then **arm the event-watch** (*Scheduling*). Report the effective cadence in human terms (e.g. "re-reviewing on each push; checking every 60s").

## Per-event algorithm

`/caregiver` does not poll each tick — it **reacts to an emitted event** from the armed event-watch (a new push / HEAD move, a new discussion, merged, closed). **The event is a hint, not authority: re-fetch live PR state on every wake — never act on the event line as a stale snapshot** (the same rule `/babysit` follows). On each emitted event, re-fetch the live state (`gh api`, never `gh pr view` — `flows/github/gh` #2), reading the current `head.sha` plus reviews / comments the way `/gh-pr`'s review phase does, then:

1. **Merged or closed?** If `merged == true` (or `state == "closed"`) → **terminal**. Stop the watch and report the outcome (merged-success, or closed-without-merge). Mirrors `/babysit`'s terminal — but this skill has no working-tree checkout of its own to defer (each dispatched `/gh-pr` already returned to the default branch), so there is nothing to unwind here.
2. **HEAD moved (new push) or a new discussion landed since the last review?** → **dispatch `/gh-pr` (one-shot)** against the live PR — *Re-review continuity* applies: held context, fresh evidence, a delta re-verify against the new head SHA. Then apply the material-verdict-change gate (below) to decide whether that dispatch's verdict is posted.
3. **Otherwise idle** — no HEAD move and no new discussion (nothing to re-review) → do no work on this event, **confirm the event-watch is still armed** (*Scheduling* → *Self-continuation*), and yield. This is the loop's normal state; it is **not** a terminal condition.

**Every event emits one status line** so the loop is observably alive and its decision is legible — e.g. `caregiver #<n> @<head7>: new push → re-reviewed → REQUEST_CHANGES→APPROVE, posted; watch armed` or `… @<head7>: new comment → re-reviewed → verdict unchanged, silent; watch armed`. The line names: the **event just handled**, whether HEAD moved / a discussion landed, the recomputed verdict, whether it **posted** or **re-reviewed silently**, and — on any non-terminal wake — the continuation path: **the live event-watch confirmed armed**, or, in degraded fallback, the labeled re-invoke hand-off. A wake that reports without an action AND without a confirmed live watch (or degraded hand-off) is the stall signature.

## The material-verdict-change gate (the one new piece)

This is `/caregiver`'s analog of `/babysit`'s SHA-pinned merge gate — the one piece of logic this skill adds on top of the borrowed wait + review. **Re-post a fresh `commit_id`-pinned verdict ONLY when the verdict materially changes; re-review SILENTLY on an identical outcome.**

The **"verdict" is the whole review outcome — the tier (`APPROVE` / `REQUEST_CHANGES`) *and* its blocker set**, not the tier alone. After a step-2 dispatch computes the new verdict, compare it to the last-posted one and re-post (Phase 6 post template of `/gh-pr`, pinned to the *new* head SHA) whenever **either** changed materially:

- **Tier flip** — e.g. a standing `REQUEST_CHANGES` whose blockers a fixing push now clears → **auto-flip to `APPROVE`** and post.
- **Same tier, different blocker set** — the tier stays `REQUEST_CHANGES` but the blockers changed: a push fixed blocker A and introduced regression B, so the standing review now lists a *resolved* blocker and is silent on a *new* one. This **must re-post** — the outcome the author reads has materially changed. Treating same-tier as "nothing new" is the trap: it leaves a misleading standing review (still citing the now-fixed A) and lets a push-introduced regression (B) go unreported.

Re-review **silently — no post** **only** when the new verdict is *identical* to the last posted one — same tier **and** an unchanged blocker set (a resolved item is a change; a still-open item carried verbatim is not). That is the sole anti-review-spam case: an identical re-review adds nothing and must not re-post.

**Track the last-posted verdict — including its blocker set — plus the SHA it covered**, so the next event compares the full outcome, not just the tier. This tracked state is what the material-change gate reads; carry it forward in the loop's State wherever the active arrangement holds it (under the event-watch default, the in-session conversation context while the watch runs; under a durable cron, the schedule's carried-forward prompt/scratchpad; under the degraded fallback, the re-invoke status line the user carries forward across manual invocations).

## Boundaries

- **NEVER merges.** Merging is `/babysit`'s job — the merge-gate consumer of the same watch. `/caregiver` is the reviewer seat and reaches no merge.
- **NEVER fixes.** Addressing feedback with code changes is `/gh-feedback-work`'s job (and `/babysit`'s dispatch of it). `/caregiver` re-reviews; it does not edit the branch.
- **NEVER comments outside a review, NEVER edits the PR body or branch, NEVER updates-from-base.** The only outward action is a posted **review** (via a dispatched `/gh-pr`). Same notify-only spirit as `core/janitor-platforms`' review-watch adapter rule.
- **Only the watched PR.** Act on the single target PR; never touch others.
- **No tree semantics of its own.** `/caregiver` owns no working-tree checkout — each dispatched `/gh-pr` behaves exactly like a manual one-shot, including its own return-to-default checkout on completion. The watch itself does no `git checkout`; it just decides when to dispatch.

## Scheduling

**Delegate to `core/janitor-platforms`** — do not hardcode a watch primitive. Load it, pick the event-watch adapter for the host, report the effective cadence in human terms. This is the **same adapter `/babysit` uses**; the two consumers share it and differ only in the acted-on emit set — for `/caregiver`'s review-watch the adapter emits on **a new push / HEAD move, a new discussion, merged, and closed** (documented in `core/janitor-platforms`' event-watch adapter section). The arrangements below map one-to-one onto *Self-continuation*; take the first the host supports.

- **Preferred default: the event-watch adapter** — an out-of-band poll that wakes the model only on a change. In-session, attended, and **token-cheap** (the model is idle between events — cost ∝ events, not wall-clock). `/caregiver` reacts to changes on a single remote target, which is exactly the event-watch shape — so this is the default whenever the host offers it.
- **Across-session opt-in: a durable standing schedule** (`CronCreate durable: true`, an OS timer, or Routines) — survives session end / a usage-limit kill, but **re-polls per fire** (a full model turn each fire, so the event-watch's token win does **not** apply) and must carry the last-posted verdict + SHA forward across fires so the material-change gate still compares against the true baseline.
- **Last resort: on-demand.** If the host can arm neither of the above, degrade to on-demand per *Self-continuation* (one re-review per invocation, flagged degraded) — never refuse to run.
- **Cloud Routines are ineligible at 60s** — their floor is 1 hour, so a sub-hour cadence is rejected at routine-creation time. Do not route a 60s watch there.

The interval arg defaults to 60s; if a chosen platform's minimum exceeds the requested interval, round up and state the effective cadence.

### Self-continuation (what wakes you on change)

`/caregiver` is **automatic by default** — its normal operation is one armed event-watch that wakes the model on each change, with no user action in between; the loop never *defaults* to "review once, wait for a nudge." On-demand exists only as an **override** ("re-review now, don't wait for the next push") and a **degraded fallback**, never as the default continuation.

- **Fallback — degrade, don't refuse.** Reach this fallback **only after verifying** neither automatic arrangement can be armed — per `core/janitor-platforms` → *Availability — verify, don't assume*: a watch primitive that is merely *unloaded* (a deferred tool surfaced by name, loadable via `ToolSearch`) is still **available**, so "I don't see it" is not "it isn't there." Only if the host can arm **neither** automatic arrangement, do **not** refuse to run — **degrade to on-demand**: run one re-review per invocation and end with an explicit, clearly-labeled hand-off — *"degraded manual mode (no watch primitive on this host): re-invoke `/caregiver <pr>` to re-review again, or set up a durable cron for an unattended watch."* This is a legitimate continuation path (the user's re-invocation), but it is a **last resort**, flagged as degraded every time.

The line to hold, same as `/babysit`: **automatic is the default and the normal path (an armed event-watch); on-demand is an override the user reaches for, or a degraded fallback when no watch primitive exists — never what the loop silently settles into.** Confirm which is live before the first status line; "I'll keep re-reviewing" with no watch armed **and** no degraded-fallback hand-off stated is the stall.

## Hand-back timer

Same contract as `flows/github/babysit` → *Hand-back timer*, on the reviewer's seat:

- **No idle-timeout hand-back by default.** An open PR is never abandoned — with no time budget set, the watch stays armed indefinitely, re-reviewing every change until the PR is terminal (or the user halts).
- **Hand back on silence only if the user supplies a time budget, and the timer resets on every event.** The watch hands back only after the *whole budget window* elapses with **no** event at all — a genuinely stalled PR (nobody pushing, no discussion). Any emitted event (a push, a comment, a mergeability transition) resets the clock; the budget measures *silence*, not elapsed wall-clock.
- On the budget elapsing in total silence: **pause and hand back** — emit the status line, report the PR's current state and standing verdict, and tell the user to **re-invoke `/caregiver <pr>`** to resume watching. This is a pause, not an abandonment — the PR is untouched and a re-invocation re-arms the watch.

## Terminal conditions

The loop ends on exactly one of:

- **Merged** — the PR merged (by a human or `/babysit`, never by this skill). Report and stop.
- **PR closed without merge** — someone closed it; report and stop.
- **Silence budget elapsed** — *only if the user set a time budget* and the whole window passed with no event (*Hand-back timer*); pause and hand back with a re-invoke instruction. A fresh `/caregiver <pr>` re-arms the watch. With no budget set, this terminal does not exist — the watch waits indefinitely.

## Guardrails

- **The watch is notify-only — it re-reviews and re-posts a REVIEW only.** It NEVER merges (that is `/babysit`), NEVER fixes (that is `/gh-feedback-work`), NEVER comments outside a review, NEVER edits the PR body or branch, NEVER updates-from-base.
- **Re-post on a material verdict change; stay silent only on an identical one.** The verdict is the tier **and** its blocker set. Post a fresh `commit_id`-pinned verdict whenever either changed — a tier flip (blockers cleared → auto-flip to APPROVE) **or** the same tier with a different blocker set (A fixed / B introduced still stays `REQUEST_CHANGES`, but the outcome changed, so it re-posts — otherwise the standing review misleads and a push-introduced regression goes unreported). Re-review **silently** only when the new verdict is *identical* (same tier, same open blockers).
- **The watch must never stall.** While the PR is open, a live event-watch MUST be armed (automatic default) or a labeled degraded-fallback re-invoke stated — never yield on an open PR with no watch and no hand-off. No idle-timeout hand-back by default (an open PR is never abandoned); hand back on total silence only if the user set a time budget, and the budget timer RESETS on every event.
- **Delegate the watch primitive — never name it here.** The watch arms `core/janitor-platforms`' event-watch adapter; this skill never names or hardcodes the underlying watch primitive (portability rule; `/babysit` delegates the same way).
- **Dispatch `/gh-pr`, don't reimplement it.** The per-event review is a full `/gh-pr` one-shot dispatch — the review rigor, the verdict, the `gh api` post, and the return-to-default checkout all live there. The new surface here is the material-change gate and *when* to dispatch; the review itself is delegated.
- **Only the watched PR.** Act on the single target PR; never touch others.
- **Incident hook.** A self-acknowledged mistake/doctrine violation ("you are right, I …", "I didn't follow …", "I ignored /…") or a blocker surfacing on a PR this flow authored is an incident — detect and route per `core/ego` (→ `/grow` / `/absorb`), after finishing the deliverable.

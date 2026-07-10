---
description: Close the learning loop on a shipped PR - resolve its candyland unit, fix outstanding blockers, and distill the review outcome into the KB
triggers:
  - absorb
  - absorb pr
  - learn from pr
  - learn from this pr
  - closed loop
  - close the loop
  - review outcome
  - absorb the feedback
when: User invokes /absorb <pr-url|#N> to run the closed learning loop over a PR — fix any live blockers, then mine the review outcome (and the candyland telemetry that produced it) into audited KB updates
related:
  - flows/github/gh
  - flows/maintainer/learn
  - flows/maintainer/grow
  - flows/github/gh-feedback-work
---

# /absorb — Close the Learning Loop on a Shipped PR

> ## /absorb ORCHESTRATES; IT OWNS NO MACHINERY OF ITS OWN
>
> `/absorb` is the **closed loop**: a PR carried the work out, a reviewer said what was wrong,
> and that outcome must flow back into the KB — while any live blocker on the PR gets fixed
> first. It is **pure composition**. Every step is delegated to an existing flow **by reference**
> (this KB forbids duplicated prose):
> - fixing blockers + rewriting the PR body → **`/gh`** (→ `flows/github/gh-feedback-work`)
> - mining the producing candyland unit's telemetry → **`/learn`**
> - generalizing + shipping the KB delta → **`/grow` Steps 3–6** (via `/learn`)
>
> If you find yourself restating a phase of `/gh`, `/learn`, or `/grow` here, stop — reference it.
> `/absorb` only decides *which* flow runs, *in what order*, and *scoped to what*.

> ## A LIVE BLOCKER IS AN IMMEDIATE INCIDENT, NOT "EVENTUAL" WORK
>
> A standing blocker on a detritus-authored PR is not merely a review outcome to absorb whenever
> the loop gets around to it — it is an **immediate incident** per `core/ego` trigger ②. Every
> detritus PR passes the `/gh-self-review` loop-until-clean gate before it opens, so a surviving
> blocker is by definition a **gate miss**: something the gate should have caught escaped it.
> `/absorb` is that incident's immediate route. Immediacy never costs the deliverable — the fix
> leg (Phase 3a) fixes first, before any learning runs. (See `core/ego` for the trigger taxonomy;
> do not restate it here.)

---

## Scope boundary with /learn and /grow (read first)

- `/grow` source = a **live session correction**. `/learn` source = **candyland telemetry across many units**.
  `/absorb` source = **one PR's review outcome** — the single most concrete signal there is, because a reviewer
  named a concrete defect on real shipped code.
- `/absorb` does not replace `/learn`; it **invokes** it, scoped to the one unit that produced this PR (plus its
  children), instead of mining the whole telemetry corpus. The mining, clustering, addressability filter, ledger,
  and shipping spine all live in `/learn`/`/grow` — `/absorb` adds only the PR-first intake and the fix-before-learn
  routing.

## Companion work: the candyland PR-body footer (out of scope here)

Phase 2 resolves the producing unit fastest from a PR-body footer that candyland stamps when it opens a PR:

```
🍬 Opened by candyland — <kind> `<id>`
```

where `<kind>` is `run` / `quest`. **Emitting that footer is a companion change in the candyland
repo — OUT OF SCOPE for this doc.** Until it ships, and for every PR opened before it shipped, `/absorb` resolves
the unit via the **reverse telemetry lookup** fallback (Phase 2) — that fallback is the migration path, not a
temporary crutch, so `/absorb` works today with no candyland change.

---

## Phase 1: Intake — resolve the PR and fetch FRESH state

Input is a PR URL or bare `#N` (bare only when cwd is inside the target repo). Locate the repo and fetch **live**
PR state — never a stale snapshot — using the resolution helpers in `flows/github/gh` (Phase 0 repo locate,
Phase 1 classification + the `gh api` resolution-helper block). Do NOT restate the `jq`; read it from `/gh`.

From those helpers, establish exactly three facts:

- **open vs merged/closed** (`state`, `merged`).
- **the blocker set at/after head** — each reviewer's *latest decisive* review (the `group_by(.user.login)|map(last …)`
  helper), keeping only `CHANGES_REQUESTED` that stands at or after the current head commit. A stale
  `CHANGES_REQUESTED` already superseded by a newer `APPROVED` from the same reviewer is **not** a live blocker.
- **the review-round history** — the sequence of reviews/comments across head commits, carried into Phase 4 as the
  `/learn` review-round input.

## Phase 2: Unit resolution — find the producing candyland unit

Resolve which candyland unit opened this PR, in order:

1. **Footer ref (fast path).** Read the PR body (`gh api …/pulls/<n> --jq .body`, per the `/gh` reads-via-`gh api`
   convention) and parse the `🍬 Opened by candyland — <kind> \`<id>\`` footer. A match gives `<kind>`+`<id>` directly.
2. **Reverse telemetry lookup (fallback + migration path).** No footer (pre-footer PR, or a hand-opened PR) → glob
   the candyland records for the unit whose `prs[]` contains this PR URL. **The recipe is canonical in
   `flows/maintainer/learn`'s data-access map** (ports/keys, `/runs/*` · `/quests/*` glob reads,
   the `prs[]` field) — compose it by reference; do not restate the endpoints here.
3. **No unit found.** The PR was not candyland-produced. Record "non-candyland" and carry it into Phase 3 routing —
   there is no telemetry leg to run.

## Phase 3: Route by state

Decide the legs to run from Phases 1–2. Fix always precedes learn (never trade the fix for the lesson):

| PR state | Unit | Legs |
|---|---|---|
| **open** with a live blocker (Phase 1) | candyland | **Fix leg** (Phase 3a) → **Learn** (Phase 4) → **Grow** (Phase 5) |
| **merged / closed**, or open + **no** live blocker | candyland | skip fix leg → **Learn** (Phase 4) → **Grow** (Phase 5) |
| **open** with a live blocker | non-candyland | **Fix leg** → **Grow-only** (skip Phase 4 — no telemetry) |
| merged / no blocker | non-candyland | **Nothing to absorb** — honest stop; report it and finish |

### Phase 3a: Fix leg (only when a live blocker stands)

Dispatch **`/gh`** with the resolved PR. Per `flows/github/gh` Phase 1, an open PR carrying an unaddressed
`CHANGES_REQUESTED` routes to `gh-feedback-work`, which fixes the named findings, self-reviews, pushes, and rewrites
the PR body in place. **Do not restate that flow.** Two `/absorb`-specific obligations on the handoff:

- **Carry the authorization forward.** Invoking `/absorb` on a PR with a standing blocker *is* authorization to fix
  and push — propagate that signal so the sub-skills don't re-ask (`flows/github/gh` Phase 2 authorization
  propagation). `/absorb` never opens a *new* PR, so no PR-open gate is in play; it only pushes onto the existing one.
- Capture the fix result (commits, push, rewritten-body URL) for the Phase 6 report.

## Phase 4: Learn — mine the producing unit (candyland PRs only)

Invoke **`/learn` scoped to the resolved unit + its children**, not the whole corpus:

- The unit from Phase 2 is the mining root; a quest includes its runs
  (`/learn`'s data-access map has the child endpoints).
- Pass the **review-round history** from Phase 1 as first-class telemetry — the reviewer's blocker is the strongest
  mechanism evidence in the bundle.
- `/learn` runs its own machinery unchanged: failure-signature extraction, deterministic clustering, the one bounded
  attribution pass, the addressability filter, and the **learnings ledger dedup** (so re-absorbing a unit doesn't
  re-propose an already-adopted/rejected delta). Do NOT restate any of it — `kb_get flows/maintainer/learn`.

## Phase 5: Grow — distill the mechanism and ship ONCE

The blocker names a **mechanism** (per `/learn` Step 1: symptom ≠ mechanism — "test failed" is a symptom; "coder
claimed done without running the failing path" is a mechanism). Only mechanisms get patched.

- **The mechanism to distill may be the gate gap itself, not only the code defect.** The reviewer's blocker gives
  the code-defect mechanism ("coder claimed done without running the failing path"). But because a surviving blocker
  is a `/gh-self-review` **gate miss** (see the immediate-incident note above), Phase 5 also asks: *why did
  `/gh-self-review` miss this blocker before the PR opened?* That gate-gap mechanism is a first-class distillation
  target — patching only the code defect leaves the gate that let it through unchanged.
- **When the code-defect mechanism and the gate-gap mechanism converge on one root cause, ship ONE delta** (the
  "one mechanism, one delta" guardrail). Often the reason the code was wrong *is* the reason the gate didn't catch
  it — a single KB change closes both. Only split when they are genuinely distinct roots.
- **When the fix leg and the learn leg converge on the same mechanism, ship it once.** The reviewer's blocker (fixed
  in Phase 3a) and the telemetry cluster (Phase 4) are two views of one underlying gap — do not open two KB deltas
  for it. One delta, one issue, one PR.
- Ship through **`/grow` Steps 3–6** (generalize past the trigger → prefer editing an existing doc → confirm-or-implement
  → deliver via `/gh` as issue → branch → PR). This is the identical spine `/learn` already composes; `/absorb` reaches
  it *through* `/learn`, and adds nothing to it. `kb_get flows/maintainer/grow`.
- **Non-candyland + blocker** runs Grow-only: the review blocker alone is the signal; distill its mechanism and ship
  via `/grow` Steps 3–6, with no telemetry leg.
- Public `benitogf/detritus` — the `/grow`/`/gh` private-name scrub applies to every issue/PR/branch/commit body.

## Phase 6: Report

Print, in the terminal (nothing extra posted to GitHub beyond what the composed flows already posted):

- **Fixed PR** — the push result + rewritten-body URL from Phase 3a (or "no live blocker — fix leg skipped").
- **KB delta** — the issue URL and PR URL `/grow`/`/gh` opened (or "nothing addressable — logged to the ledger").
- **Ledger changes** — which mechanisms moved to `adopted` / `rejected` / `candidate` this pass (from `/learn`).
- For a non-candyland or blocker-free PR with nothing to absorb, say so plainly — an honest stop is a valid outcome.

## Guardrails

- **Compose, never restate.** `/absorb` owns routing + scoping only. The moment you rewrite a `/gh`, `/learn`, or
  `/grow` step, you've duplicated machinery this KB forbids — reference it instead.
- **Fix before learn, always.** Never defer or drop a live blocker to get to the lesson. The feature PR is the
  primary deliverable; the KB delta is secondary.
- **One mechanism, one delta.** When the blocker and the telemetry cluster are the same gap, ship a single KB change —
  don't open a review-driven delta and a telemetry-driven delta for one root cause.
- **`/absorb` opens no new PR of its own.** It pushes onto the reviewed PR (via the fix leg) and opens a KB issue/PR
  (via `/grow`→`/gh`). It never creates a third PR against the reviewed repo.
- **Honest stop.** A merged, blocker-free, or non-candyland PR with no addressable mechanism absorbs to nothing — report
  it; don't manufacture a lesson to justify the run (`/learn`'s addressability filter is the backstop).

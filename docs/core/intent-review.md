---
description: Shared intent-review method — did we build what was MEANT? A reproducible, per-commitment check of the shipped result against the original intent, complementary to core/review-rigor (which checks code mechanics). Do not invoke directly; composed by the review stage (agent id intent-reviewer).
triggers:
  - intent review
  - intent-review
  - intent satisfaction
  - commitment verdict
  - did we build what was meant
when: Internal. Loaded via kb_get by the role that performs a run or quest's final intent review (the review stage, agent id intent-reviewer) to check the shipped diff/PRs against the immutable original input + Intent Brief — not a standalone workflow.
related:
  - core/review-rigor
  - core/completion
  - core/planning
  - core/dream
  - flows/principles/truthseeker
  - roles/tech-lead
---

# Core — Intent Review: did we build what was meant?

`core/review-rigor` answers *"is this diff correct, safe, and non-fragile?"* — code
mechanics. It does **not** answer *"is this the thing we were asked to build?"* Intent
review is that second, distinct gate: it checks the **shipped result against the original
intent**, commitment by commitment, with cited evidence. A run or quest is done
only when **both** hold — a clean review-rigor pass **and** no unmet intent commitment.

> ## ⛔ Do not invoke directly
> No slash command. Loaded via `kb_get` by the role that performs a run or quest's final intent
> review — the review stage (agent id `intent-reviewer`). Like every doctrine doc, the reviewer **loads this method rather than carrying a
> paraphrased rubric** — a hand-rolled "does it match intent?" prompt silently drops the
> discipline below (see `core/review-rigor` → *the hand-rolled rubric* anti-pattern).

## Why a separate gate

A change can pass review-rigor — well-tested, no bugs, no fragility — and still be the
**wrong thing**: it satisfies an adjacent problem, drops a stated requirement, or quietly
narrows scope mid-build. Task completion ("every generated task is done") is not intent
satisfaction ("the shipped result fulfills what was asked"). Final review compares output
against **intent**, not only against the generated task list (`core/completion` → done is
"the agreed scope met", and the agreed scope is the *intent*, not the plan's restatement of
it). Intent review is the forcing function that keeps the two from drifting.

## Inputs (read these, in this order)

1. **The immutable original input** — the user's verbatim instruction/goal as first
   captured, never the agent's later paraphrase. (This is the stored, immutable
   run/quest input; `core/planning` → the Intent Brief is derived *from* it.)
2. **The Intent Brief** — the restated goal, scope, and commitments (`core/planning` /
   `core/dream`).
3. **The shipped artifacts** — the actual diff, the opened PR(s), the tests, command output.
   Claims in a PR body are **claims, not evidence** (`core/review-rigor` → *verify the
   change's claims*); read the diff/tests, not the description.

## The method — extract commitments, verdict each with evidence

Intent review is **reproducible**: two reviewers running it over the same inputs reach the
same verdicts, because each verdict is anchored to cited evidence — never an overall
impression or a vibe check.

1. **Extract commitments.** Decompose the Intent Brief (and the original input it derives
   from) into **commitments**: each a single, checkable assertion about what the shipped
   result must do or be. One assertion per commitment — split compound statements. A
   commitment the brief implies but never states (a load-bearing assumption) is still a
   commitment; surface it.
2. **Verdict each commitment** against the shipped artifacts, with **cited evidence** (a diff
   hunk, a PR link, a passing test, a command's output):
   - **`satisfied`** — the shipped result meets the commitment; evidence proves it.
   - **`partial`** — partially met: a narrower case works, an edge/scale/secondary aspect is
     unmet, or it's met but with a caveat. Evidence shows both what works and what's missing.
   - **`missed`** — not met, or met in name only (a stub / placeholder / "simplified for
     now" is `missed`, not `partial` — `core/completion` forbids shipping a stub for in-scope
     behavior). Evidence shows the gap.
   No verdict without evidence. "Looks right" is not a verdict.
3. **Emit the verdict set.** A machine-readable record, one entry per commitment:

   ```json
   {"commitment":"export endpoint streams CSV for >1M rows without buffering all in memory",
    "verdict":"partial",
    "evidence":["api/export.go:42-71 streams row-by-row","export_test.go covers 1k rows only — no large-input test proving the no-buffer claim"]}
   ```

## The gate (what each verdict does to delivery)

Intent review runs **before** a run or quest opens its per-repo PR(s) (`core/completion` → the
exit gate; delivery shape in `roles/tech-lead` / `core/build`). The verdicts gate delivery:

- **`missed` → blocks that repo's PR.** A missed commitment is unmet in-scope work; per
  `core/completion`'s disposition 1 it is **handled now** (the loop continues — feed the gap
  back as the next unit), not deferred. The PR does not open while a `missed` stands.
- **`partial` → annotates, does not block.** Record the partial verdict on the PR and route
  it to the **review router** (the human reviewer/area suggestion) so a human sees the caveat,
  but it does not hold the PR. Use `partial` honestly — a `missed` dressed as `partial` to
  open a PR sooner is exactly the silent deferral `core/completion` forbids.
- **`satisfied` → clears.** No annotation needed beyond the record.

A run or quest's final intent review is clean when **every commitment is `satisfied` or
`partial`** (zero `missed`) — and that is necessary, not sufficient: the lead must also
confirm technical done (integration green, review-loop clean per `core/review-rigor`). This is
the dual sign-off: intent review + technical done, both required.

**Passing both gates is still not `done` until delivery lands.** The gates run *before* the
PR opens; the push/PR-open can still fail afterward (branch protection, secret-scanning push
protection, a PR-open error). A post-gate **delivery-mechanic failure** is not a clean
terminal — it re-enters the *same* bounded remediation model as a `missed` commitment: an
agent reads the verbatim git/gh error (usually machine-fixable) and fixes it, then delivery
is re-attempted (`core/completion` → *Honest terminal state* → delivery attempted-but-failed).
For a per-repo-PR deliverable the run or quest is `done` only when **every** impacted repo's PR
actually opened; one repo's delivery failure is isolated but keeps it out of `done`.

## Boundaries

- Intent review **does not** re-do code review — fragility, correctness, and test-mechanics
  are `core/review-rigor`'s job. It asks only "does this fulfill the intent?"
- It judges **shipped artifacts**, not promises — an unverified claim in a PR body is at best
  a `partial` (the commitment is asserted but unproven), never a `satisfied`.
- It never asks the user to clarify during execution — an ambiguous commitment is resolved
  from the immutable input + brief + research, or escalated within the agent hierarchy
  (`core/dream`: the architect owns decisions; no user loop mid-run).

## What this doc is not

- Not a slash command — `core/intent-review` is `kb_get`-only.
- Not `core/review-rigor` (code mechanics) — it is the complementary intent-satisfaction gate;
  both are required.
- Not the planning method (`core/planning` / `core/dream`) that *produces* the Intent Brief —
  it *consumes* the brief to judge the result.
- Not the completion doctrine (`core/completion`) — it reuses the dispositions and exit gate
  defined there to decide what a `missed` verdict means for delivery.

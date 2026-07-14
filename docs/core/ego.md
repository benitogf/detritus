---
description: "Incident triggering: detect self-acknowledged mistakes and PR-blocker gate-misses, route each to the learning loop that ends in a KB PR."
triggers:
  - ego
  - incident
  - self-acknowledged
  - "you are right"
  - "I didn't follow"
  - "I ignored"
  - doctrine violation
  - blocker on PR
  - gate miss
when: Internal. Referenced by flow docs to define when an incident fires and where it routes; loaded via kb_get, no slash command.
related:
  - core/completion
  - flows/maintainer/grow
  - flows/maintainer/absorb
  - flows/maintainer/learn
  - flows/github/gh-self-review
---

# Ego — Incident Triggering

An **incident** is a signal that agent behavior diverged from doctrine, or that a quality gate
missed something. This doc owns **detection and routing** only; the distill→ship machinery lives in
`/grow`, `/learn`, `/absorb` (composed by reference, never restated here). Every route terminates in a
**KB PR** (or an honest ledger no-op when nothing is addressable).

> ## ⛔ Do not invoke directly
> No slash command. Loaded via `kb_get` by the flow docs that need the incident-trigger taxonomy.

## Trigger taxonomy

| ID | Trigger | Detection | Routes to |
|----|---------|-----------|-----------|
| ① | **self-acknowledged error / doctrine violation** | Keys off the *agent's own output* conceding a mistake or a doctrine/flow violation. Admission phrase-class (e.g. "you are right", "sorry, I…", "my mistake", "I was wrong", "I should have…") or violation phrase-class (e.g. "I didn't follow {doctrine}", "I ignored /{flow}", "that was against {rule}", "I skipped {step}"). **Rule: acknowledgment ≡ detection** — the moment the agent concedes, the trigger has fired; no user escalation is required. | → `/grow` |
| ② | **PR blocker despite the self-review gate** | A live blocker on a detritus-authored PR (reviewer `CHANGES_REQUESTED`, a `/gh-pr` finding, or babysit-detected feedback). Every detritus PR ships through the `/gh-self-review` loop-until-clean gate, so a surviving blocker is by definition a **gate miss** — itself an incident. Fires **immediately** (does not wait for merge). | → `/absorb` |
| ③ | **user correction** | An in-session correction from the user. Detection cues live in `flows/maintainer/grow` Step 1. | → `/grow` |
| ④ | **telemetry failure signature** | A failure signature mined from candyland telemetry across units — e.g. a merged candyland PR whose producing unit carries `incidents[]`. Detection cues live in `flows/maintainer/learn` Step 1. | → `/learn` |

Samples above are illustrative `e.g.` only — generalize to the whole phrase-class, never to the
literal string.

Each route's command (the *Routes to* column) is composed **by reference**; this doc owns no
distill/ship machinery — only when an incident fires and where it goes.

## Invariants

- **Deliver / fix-first.** Never trade the primary deliverable for the lesson PR
  (`core/completion`). In-session flows finish delivery, then route.
- **Only ② may interrupt** — and only because `/absorb`'s fix leg fixes the blocker first, so
  "immediate" never means dropping the deliverable. ① never interrupts.
- **Every route ends in a KB PR** (or an honest ledger no-op when nothing is addressable).
- **In-session vs sidecar.** In-session flows route directly. Sidecar (candyland) agents *record*
  each incident by appending to an `incidents[]` collection on their unit record/report; the user's
  session drains that collection and routes every entry post-delivery — lesson capture is
  in-session, never in the sidecar. Appending (never overwriting) means a unit that trips multiple
  incidents surfaces all of them, not just the last.
- **Route directly means autonomously — never a confirmation prompt.** Once a trigger has fired,
  its route is *decided* (this table decided it); post-delivery the agent runs that route without
  asking the user to authorize it. Detection ≡ decision (①: acknowledgment ≡ detection), so
  surfacing a decided route as something for the *user* to trigger is a redundant confirmation of
  what doctrine already mandates — the failure mode `/grow` itself exists to fix. This holds for
  **every route (①–④) and every phrasing**: imperative ("want me to `/grow` that?", "should I
  capture this lesson?"), conditional ("if you `/absorb` #N, that's a clean gate-miss"), and passive
  aside ("that's worth a `/learn`", "you could `/absorb` this") are all the same failure — do not
  match on the literal `/grow`+imperative shape and miss the others. Announce that you're
  routing if useful, then do it; the only thing that legitimately waits for the user is a genuine
  decision the taxonomy doesn't already settle. (The user can always redirect after the fact.)
- **No local clone required.** Detection fires anywhere; shipping does too — the ship-leg flows
  (`/grow` / `/absorb` / `/learn`) resolve or provision a writable KB checkout on demand per
  `core/kb-writeback` (forking when the user lacks write access), so an incident routes to a KB PR
  from any machine, not only inside a detritus clone.
- **Public `benitogf/detritus`:** scrub private names from any resulting issue/PR/branch/commit.

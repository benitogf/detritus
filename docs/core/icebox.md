---
description: The single definition of the `icebox` label — the provenance marker every mid-work deferral issue carries, plus the sole home of the label mechanics. Distinguishes parked out-of-scope findings from primary-ask issues, and enforces the label at the one posting chokepoint. Do not invoke directly.
triggers:
  - icebox
  - out-of-scope finding
  - deferral label
  - parked finding
when: Internal. Loaded via kb_get by gh-issue-create (the posting chokepoint) and referenced by every finding-origination site.
related:
  - flows/github/gh-issue-create
  - core/completion
  - core/review-rigor
  - flows/github/gh
---

# Core — Icebox: the mid-work deferral label

The one authoritative definition of the `icebox` label and the single home of its mechanics.
The posting chokepoint enforces it; every finding-origination site references this doc and never
restates the mechanics.

> ## ⛔ Do not invoke directly
> No slash command. Loaded via `kb_get` by `flows/github/gh-issue-create` and pointed at by every
> finding-origination site. Those docs reference this one; they never restate the label color,
> description, or lifecycle.

## Definition

An **icebox issue** is one an agent creates **mid-work to park an out-of-scope finding** — instead
of silently dropping the finding because it is out of scope for the job at hand. The `icebox` label
marks that provenance.

Origination classes (all labeled `icebox`):

- **RV-F5 excluded / verified findings** — a reviewer routing a finding that is real but outside the
  reviewed diff's scope (`core/review-rigor`, "Excluded ≠ discarded").
- **Disposition-2 feature-splits** — work a builder encounters that is a distinct, named new unit
  rather than part of the current one (`core/completion` disposition 2).
- **Out-of-scope feedback / follow-ups / asks** — surfaced by `/gh-feedback-work`, `/gh-issue-work`,
  or `/babysit` while working something else.

Contrast class — issues that ARE the ask, which are **never** labeled `icebox`:

- User-directed issue creation (the user asked for the issue itself).
- The `/smith` delivery **seed** issue (the issue is the work about to be done).
- `/grow`, `/learn`, `/absorb` KB-delta issues (the maintainer flow's own product).

## Provenance, not state

The label is applied **at creation** and is **never stripped** when the issue is later worked. It
answers "why does this issue exist" — a fact that does not change when someone picks it up. There is
therefore **no lifecycle or removal mechanic** anywhere: no flow adds, checks, or removes `icebox`
as a signal of progress.

## Origin classification happens at the chokepoint

`flows/github/gh-issue-create` classifies origin **itself**, even when the caller passes no signal,
using the rule:

> Was this session / agent **asked** to do this work, or is it **about to do it now**? No to both →
> it is a parked out-of-scope finding → `icebox`.

Caller-supplied origin signals (propagated through `flows/github/gh` Phase 2) are **reinforcement,
not enforcement**. Enforcement lives here, at the single posting site, so a caller that forgets the
signal still yields a labeled issue. When origin is genuinely ambiguous, `gh-issue-create` asks.

## Signal propagation

Callers that route a deferral into `/gh-issue-create` name the handoff an **icebox deferral**,
mirroring how the authorization signal propagates in `flows/github/gh` Phase 2. The name is intent
clarity for the reader; the chokepoint still classifies independently.

## Mechanics (defined ONLY here)

This is the single home of the label mechanics. The color `c2e0f0` appears in no other doc.

```
# ensure the label exists (idempotent — 422 already_exists is fine)
gh api --method POST repos/<owner>/<repo>/labels \
  -f name=icebox -f color=c2e0f0 \
  -f description="Out-of-scope finding parked by an agent mid-work" \
  2>/dev/null || true

# then add to the gh-issue-create Phase 5 POST:
#   -f "labels[]=icebox"
```

`gh-issue-create` Phase 5 **references this ensure-label step** and adds only the
`-f "labels[]=icebox"` param to its existing issue POST — it does NOT restate the color or
description.

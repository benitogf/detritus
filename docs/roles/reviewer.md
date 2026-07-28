---
description: Delivery-loop reviewer role — hard-reviews an integrated diff against the driving intent under the shared review doctrine and emits a single verdict. Review-only; it never implements fixes. Do not invoke directly — loaded by /gh-self-review sub-agents, /gh-pr and /gh-pr-safe, /forge delivery, and the candyland reviewer identity.
triggers:
  - reviewer
  - reviewer role
  - review verdict
  - intent fidelity
when: Internal. Loaded by an agent acting as the reviewer in a delivery loop (in-session review sub-agent or candyland out-of-process reviewer).
related:
  - core/review-rigor
  - flows/principles/truthseeker
  - roles/tech-lead
  - flows/github/gh-self-review
  - flows/github/gh-pr
  - flows/build/forge
---

# Reviewer

The reviewer is the reviewing counterpart of the coder roles — it consumes an integrated diff plus the driving intent, applies the shared review doctrine, and emits one verdict. It owns judgment, not remediation.

## Doctrine — load, never paraphrase

The reviewer loads `core/review-rigor` and `flows/principles/truthseeker` via `kb_get` and applies them end-to-end (the same load-never-paraphrase rule review-rigor states). This role doc adds only what is role-specific: the intent contract, the verdict contract, and the review-only boundary.

## Intent fidelity — the intent is part of the review subject

> The brief that spawns a reviewer carries the **driving intent** — what the user asked for (a `.plan/<slug>.md` contract, an issue body, a feedback spec, or the stated ask). The reviewer verifies the diff **satisfies that intent**: an intent commitment that is missing from the diff, only partially delivered, or contradicted by it is a **blocker** (`intent unmet: <commitment>`). A clean verdict asserts intent fidelity as well as defect absence. A reviewer spawned without the intent notes its absence in the verdict and reviews mechanics only — it does not guess the intent, and the spawning flow treats the missing handover as its own defect to fix.

## Two briefing layers

When the brief carries a root-intent layer above the task intent, apply `core/review-rigor` → Two briefing layers.

## Diff consumption and re-review passes

Pull the diff live from the repo — never from a dump or an inline paste — per `core/review-rigor` → Consume the diff from live git. A re-review after a fix pass continues the prior review context per `core/review-rigor` → Re-review continuity.

## Verdict contract

Exactly one verdict per review pass — `REVIEW_CLEAN` (no blockers), or `REVIEW_FINDINGS ` followed by JSON `{"blockers":[{"file":"path","line":12,"issue":"…"}]}` citing file and line per the doctrine. This is the exact format candyland's parser consumes; in-session flows present the same findings as the `/gh-self-review` triage block instead. A verdict's rationale must prove, not hedge — an unproven CLEAN is bounced by the verdict-integrity gate.

The returned verdict/triage also carries the two stamp lines defined in `core/review-rigor` → *Review stamps* — the provenance line and the coverage ledger — so a wrapping flow that posts (`/gh-pr`, `/gh-pr-safe`) can paste them verbatim above the attribution footer.

## Review-only boundary

The reviewer never edits files, never commits, never fixes. Remediation belongs to the fix identity (candyland) or the owning coder via the tech-lead loopback (`/forge`, per `roles/tech-lead` TL-F5). A reviewer that patches what it reviews voids the independence the loop exists for.

## Model and effort

Model and effort are set per role — the installed `~/.claude/agents/detritus-reviewer.md` definition pins `claude-fable-5` at `high` effort for in-session spawns; candyland's settings default the reviewer roles to the same. Independent review wants a different model at high effort, not the builder's own model reviewing its own blind spots.

**Exception — the PR-review flows run unpinned, by design.** `/gh-pr` and `/gh-pr-safe` spawn a **separate** reviewer definition, `detritus-reviewer-pr` (`~/.claude/agents/detritus-reviewer-pr.md`), identical in role and doctrine but defined with `model: inherit` — so the review runs on the invoking session's model, still at `high` effort. This is deliberate: it conserves the weekly Fable allowance for the flows that spend it on every internal iteration (`/forge` delivery and `/gh-self-review` run many reviews per build), at the cost of **cross-session verdict determinism on posted PR reviews** — two runs against the same head can differ if the session model differs. That determinism is worth paying Fable for on the build-side loops; it is not worth burning the allowance on the (typically one-shot per head) posted PR review. The independence the spawn buys — a fresh reviewer identity that loads the review doctrine and never reviews its own builder blind spots — is retained; only the *model pin* is dropped, and only for the PR-review definition. The shared `detritus-reviewer` definition stays fable-pinned, and `/forge` delivery and `/gh-self-review` keep spawning it unchanged.

**The reviewer's output always states the model it actually ran on and its effort** — not conditionally on a fallback, but on every review. The provenance stamp in `core/review-rigor` → *Review stamps* is the carrier: it self-reports the real executing model (a silent degrade shows there), so the reader always knows whether the verdict came from the pinned fable, the opus fallback, or an inline session. The fallback ladder below is unchanged; the disclosure it once carried alone is now always-on via the stamp.

### In-session model-limit fallback

Claude Code has no native usage-limit fallback — its `--fallback-model` covers only transient overloads (529) and model-unavailability, explicitly *not* rate-limit, billing, or usage-limit errors. So when the pinned `claude-fable-5` reviewer exhausts its weekly Fable allowance mid-session, the spawning flow (`/forge` delivery and `/gh-self-review` — the two flows that keep the fable pin) owns the fallback:

- **Trigger.** The reviewer sub-agent dies or returns carrying a usage-limit banner that *names its pinned model* — e.g. `You've hit your Fable limit · resets <time>` (any model-named limit banner).
- **Action.** The spawning flow re-spawns the reviewer **once** with the `Agent` tool's `model: "opus"` override (`claude-opus-4-8`), rather than failing the review or waiting for the Fable limit to reset.
- **Disclosure.** The review output states it ran on the fallback model, so a reader knows the verdict came from opus, not the pinned fable.
- **Session-stickiness.** Once the fable limit is hit, subsequent reviewer spawns in the same session go straight to `model: "opus"` instead of re-hitting the limit each time.
- **Exclusion.** An account-wide banner — a session, weekly, or usage limit with *no model named* — is **not** fallback-eligible: the whole seat is exhausted, so the flow surfaces it to the user as a capability blocker rather than retrying on another model.

The generated `detritus-reviewer` definition stays fable-pinned; the fallback lives in the *spawning flow*, never in the definition.

---
description: Turn the issue under discussion into a single-page decision PDF for a non-technical stakeholder — premise-only, plain real-world terms, no jargon, no component names or technical mechanisms.
argument-hint: "[issue source: #N, a /todo ref, a .plan/<slug>.md path, or nothing for the live session]"
triggers:
  - consult-simple
  - consult simple
  - explain simply for a decision
  - plain-language decision pdf
  - non-technical decision pdf
when: A reader with zero knowledge of the codebase needs a single-page decision PDF — the issue, its cause→effect genesis, and the multiple-choice options framed in plain real-world terms so they can decide.
related:
  - core/consult
  - flows/consult/consult-tech
---

# /consult-simple — Plain-Language Decision PDF

`/consult-simple` is a **thin driver**: it loads `core/consult` and runs the engine's pipeline with the audience register fixed to **non-technical stakeholder, premise-only**. It restates none of the engine — input resolution, the cause→effect model, the five PDF rules, the flowchart-vs-text gate, and the D2→Typst render pipeline all live in `core/consult` and are inherited here. When the engine tightens, `/consult-simple` inherits it.

## Register — non-technical stakeholder, premise-only

Assume the reader has zero knowledge of the codebase but can understand the premise and make a decision:

- **Strip all jargon** — no component names, no technical mechanisms.
- **Frame everything in plain real-world terms** — the issue is *what's happening*, the genesis cause→effect is *why it happened*, and each option is *what that choice means for them*.
- If a fact only makes sense with technical vocabulary, restate its consequence in plain terms or leave it out — the reader decides on the premise, not the mechanism.

## Run

1. Load `core/consult`.
2. Resolve the issue source per `core/consult` (the argument — `#N`, a `/todo` ref, a `.plan/<slug>.md` path — else the live session).
3. Run the engine's pipeline with this register applied as the detail-level rule above. Everything else — the one-page invariant, multiple-choice options, flowchart-first gate, render to `.consult/<slug>.pdf` — is the engine's job.

`/consult-tech` is the high-level technical sibling register over the same engine.

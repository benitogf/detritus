---
description: Turn the issue under discussion into a single-page decision PDF at a bare-minimum, high-level technical register — name the component and mechanism conceptually, no code or deep internals.
argument-hint: "[issue source: #N, a /todo ref, a .plan/<slug>.md path, or nothing for the live session]"
triggers:
  - consult-tech
  - technical consult
  - consult pdf tech
  - decision pdf technical
  - high-level technical decision pdf
when: A technically-literate reader needs a single-page decision PDF that frames the issue, its cause→effect genesis, and the multiple-choice options at a high level — naming the component/area and mechanism conceptually, without code or internals.
related:
  - core/consult
  - flows/consult/consult-simple
---

# /consult-tech — High-Level Technical Decision PDF

`/consult-tech` is a **thin driver**: it loads `core/consult` and runs the engine's pipeline with the audience register fixed to **bare-minimum, high-level technical**. It restates none of the engine — input resolution, the cause→effect model, the five PDF rules, the flowchart-vs-text gate, and the D2→Typst render pipeline all live in `core/consult` and are inherited here. When the engine tightens, `/consult-tech` inherits it.

## Register — bare-minimum, high-level technical

The reader has some technical context, so keep it conceptual:

- **Name the component/area and the mechanism at a conceptual level** — what part of the system, and how it works in principle.
- **No code, no deep internals** — no snippets, signatures, file paths, or line-by-line detail.
- The issue block, the genesis cause→effect chain, and each option's consequence **may use light technical terms** that a technically-literate reader follows without the codebase open.

## Run

1. Load `core/consult`.
2. Resolve the issue source per `core/consult` (the argument — `#N`, a `/todo` ref, a `.plan/<slug>.md` path — else the live session).
3. Run the engine's pipeline with this register applied as the detail-level rule above. Everything else — the one-page invariant, multiple-choice options, flowchart-first gate, render to `.consult/<slug>.pdf` — is the engine's job.

`/consult-simple` is the non-technical, premise-only sibling register over the same engine.

**Incident hook.** A self-acknowledged mistake/doctrine violation ("you are right, I …", "I didn't follow …", "I ignored /…") or a blocker surfacing on a PR this flow authored is an incident — detect and route per `core/ego` (→ `/grow` / `/absorb`), after finishing the deliverable.

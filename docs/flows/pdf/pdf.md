---
description: Generate a PDF to whatever format and style the user describes — general-purpose, multi-page, diagrams when useful.
argument-hint: "[what to generate + desired format/style, or nothing to describe it in the session]"
triggers:
  - pdf
  - generate a pdf
  - make a pdf
  - pdf document
  - custom pdf
when: The user wants a PDF of arbitrary content in a format/style they describe — a report, a one-pager, a multi-page document — not the constrained single-page decision aid. Diagrams rendered when the content calls for one.
related:
  - core/pdf-render
  - flows/pdf/pdf-management
  - flows/pdf/pdf-tech
---

# /pdf — General-Purpose PDF Generator

`/pdf` is the **freeform** PDF engine: the user describes the content and the desired format/style, and it authors and renders the document to match. It loads **`core/pdf-render`** (the shared render substrate — the Typst/D2 pipeline and the fixed output location) and nothing else. It is **not** the decision engine — it is **not** bound by `core/pdf-decision`'s cause→effect IR, five PDF rules, single-page invariant, or flowchart-vs-text gate. Those constrain the decision PDFs; `/pdf` is unconstrained.

## Register — describe it, get it

The user states **what** to produce and, when they care, **how** it should look — in the invocation argument or in the live session. Author the document freely to match:

- **Multi-page is allowed.** Lay out as many pages as the content needs.
- **Style follows the user.** When they specify a format/style, honor it. When they don't, apply **light typographic defaults** — sensible margins, a readable body size, and a clear heading scale — and do not impose the decision layout.
- **Diagrams when useful.** Render a diagram via D2 (per `core/pdf-render`'s flowchart skeleton) when the content calls for one; otherwise render Typst-only.

## Run

1. Load `core/pdf-render`.
2. Determine the content and desired format/style from the argument, else ask the user to describe it in the session (describe the *document*, never the output location).
3. Author the Typst document — and a D2 diagram when the content warrants — and render per `core/pdf-render`.
4. Output ends at `.pdfs/<slug>.pdf` per `core/pdf-render` (fixed location, never prompt for it, never open or commit). Report the path.

**Incident hook.** A self-acknowledged mistake/doctrine violation ("you are right, I …", "I didn't follow …", "I ignored /…") or a blocker surfacing on a PR this flow authored is an incident — detect and route per `core/ego` (→ `/grow` / `/absorb`), after finishing the deliverable.

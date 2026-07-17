---
description: Shared PDF render substrate — the Typst + D2 pipeline and the single output convention every PDF skill inherits. Owns the bundled-binary check, the .d2→SVG→.typ→compile mechanics, the D2 flowchart skeleton, and the fixed output location. Do not invoke directly; composed by the /pdf, /pdf-management, and /pdf-tech flows and the core/pdf-decision engine.
triggers:
  - pdf core
  - pdf render substrate
  - typst d2 pipeline
  - pdf output location
  - render pipeline
when: Internal. Loaded via kb_get by the general /pdf generator (directly) and by the decision engine (core/pdf-decision, which the register wrappers load). Defines the render mechanics and the one output rule so no skill restates them.
related:
  - core/pdf-decision
  - flows/pdf/pdf
  - flows/pdf/pdf-management
  - flows/pdf/pdf-tech
---

# PDF Core — the shared render substrate

Every detritus PDF skill renders through **one pipeline** (D2 for diagrams, Typst for the page) and writes to **one place**. This core owns that shared machinery: the bundled binaries, the `.d2`→SVG→`.typ`→compile mechanics, the D2 flowchart skeleton, and the fixed output location. Wrappers and engines that build on this core (the general `/pdf` generator, and `core/pdf-decision`) never restate any of it.

> ## ⛔ Do not invoke directly
> No slash command. Loaded via `kb_get` by `/pdf` and by `core/pdf-decision` (which the register wrappers load).

## Output location — FIXED, never ask

> **The output path is not a choice.** Write the finished PDF to **`.pdfs/<slug>.pdf`** — always, for every skill built on this core. `.pdfs/` is gitignored.
>
> - `slug` is kebab-case, ≤ 40 chars, derived from the document's subject.
> - The location is **FIXED**. **Never ask the user where the PDF should go** — not in the invocation, not in the session, not on overflow, not ever.
> - When done, **report the path** (`.pdfs/<slug>.pdf`). Do **not** open it. Do **not** commit it.

Any skill that loads this core inherits this rule verbatim. A wrapper author does not re-decide the output path and no skill ever prompts for it.

## Bundled binaries

The pipeline uses two companion binaries installed **beside the detritus binary by detritus setup**:

- **D2** — flowchart source → SVG.
- **Typst** — the document → PDF.

If either binary is missing, **stop and tell the user to run `detritus --update`** (or `detritus --setup` if already on the latest version) — the setup step re-attempts the companion-binary fetch (Typst + D2). Never attempt a manual install.

## Render pipeline

Work in a temp dir **under the output dir** (`.pdfs/`). Steps:

1. **Author the diagram** (only when the content calls for one) — write `<tmp>/<slug>.d2` using the *D2 flowchart skeleton* below.
2. **Render to SVG** — `d2 <tmp>/<slug>.d2 <tmp>/<slug>.svg`.
3. **Author the document** — write `<tmp>/<slug>.typ`, embedding `image("<slug>.svg")` when a diagram was rendered plus the body content.
4. **Compile** — `typst compile <tmp>/<slug>.typ .pdfs/<slug>.pdf`.
5. **Report** — announce `.pdfs/<slug>.pdf`. Do not open or commit it.

## Multi-page typesetting defaults — keep content off page seams

A multi-page document (the `/pdf` generator's case) must not split an atomic unit across a page boundary — the classic failure is a heading, or a "…returns:" lead-in, stranded at the bottom of one page while the code block / table / figure it introduces flows to the next. Single-page renders (the decision engine) never paginate, so this is moot there; the rules below are inert on one page and safe to include always.

**Baseline preamble — put at the top of every multi-page `.typ`:**

```typst
#show heading: set block(sticky: true)                     // a heading never sits alone at a page bottom — it moves with its content
#show raw.where(block: true): set block(breakable: false)  // code blocks stay whole (when they fit — see the caveat)
#show figure: set block(breakable: false)                  // figures / embedded diagrams stay whole
```

(In current Typst, sticky headings and unbreakable figures are already the defaults — only the `raw` line changes behavior. The preamble states all three explicitly so it stays correct on older Typst and if a default ever changes.)

**Couple an intro line to its block (the default fix, and the one the preamble alone misses).** `sticky` attaches a block to the one *immediately after* it — so a sticky heading follows its lead-in paragraph, but that lead-in does not automatically follow the block *it* introduces. When a short paragraph introduces a code block, table, or figure, **make that paragraph sticky** so the whole unit travels together (4-backtick fence so the inner block's fence doesn't close the example):

````typst
#block(sticky: true)[Gateway-side rejections return:]
```json
{"error": "<reason>"}
```
````

**Caveat — whole *when it fits*, break when it can't.** A unit taller than the printable page area cannot be kept whole; forcing `breakable: false` on it makes Typst overflow the margin. So keep short units atomic (above), but leave genuinely page-spanning content — a long code listing, a large table — **breakable** so it splits at a sensible seam instead of overflowing.

**Escape hatches** for anything the rules above don't settle:

- `#block(breakable: false)[ … ]` — force an arbitrary span (heading + intro + block) to stay on one page as a unit.
- `#pagebreak(weak: true)` — start a section on a fresh page when it would otherwise straddle the seam (weak: collapses if already at a page top).

## D2 flowchart skeleton

Minimal and runnable — a general starting point when a diagram helps. Fill the labels from the content; add, remove, and rewire nodes freely.

```d2
direction: down

a: "<node A>" { shape: rectangle }
b: "<node B>"
c: "<node C>"

a -> b -> c
```

The decision engine (`core/pdf-decision`) specializes this skeleton into its issue→genesis→decision→options shape; the general `/pdf` skill uses it however the content warrants (or renders Typst-only when no diagram is needed).

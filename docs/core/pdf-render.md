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

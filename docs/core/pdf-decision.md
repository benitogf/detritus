---
description: Shared decision-PDF engine — turn the issue under discussion into a single-page decision PDF for a human to act on. Resolves the issue from an explicit source (GitHub issue, /todo item, .plan file) or the live session, builds a cause→effect intermediate model, and renders it flowchart-first to .pdfs/<slug>.pdf. Builds on core/pdf-render for the render substrate. Do not invoke directly; composed by /pdf-tech and /pdf-management, which pass an audience register.
triggers:
  - decision pdf engine
  - decision pdf
  - pdf decision core
  - cause-effect model
  - single-page decision aid
when: Internal. Loaded by /pdf-tech or /pdf-management to define the input resolution, the cause→effect IR, the five PDF rules, the flowchart-vs-text gate, and the one-page decision template. Builds on core/pdf-render for the render pipeline + output location. The wrappers add only their audience register.
related:
  - core/pdf-render
  - flows/pdf/pdf-tech
  - flows/pdf/pdf-management
  - core/todo-import
  - flows/project/todo
  - flows/github/gh-issue-create
---

# Decision-PDF Engine — the single-page decision PDF

`/pdf-tech` and `/pdf-management` are two **audience registers** over **one engine**: take the issue currently under discussion, frame it as a discrete set of choices, and emit a single-page PDF a human reads and decides from. This engine **builds on `core/pdf-render`** (the shared render substrate — the Typst/D2 pipeline and the fixed output location) and adds the decision-specific layer: input resolution, the cause→effect intermediate model, the PDF invariants, the flowchart-vs-text gate, and the one-page template. The wrappers add only their register (see *Audience register*). Neither restates what's below.

v1 is **one-shot**: produce the PDF and stop. There is no decision loop-back (the chosen option feeding back into the session is a deliberate v2 split).

> ## ⛔ Do not invoke directly
> No slash command. Loaded via `kb_get` by `/pdf-tech` or `/pdf-management`.

## 1. Input resolution (dynamic)

Resolve **the issue under discussion** from exactly one source, in this priority order. Stop at the first that applies.

1. **Explicit GitHub issue** — argument is `#N` (or an issue URL). Pull with `gh issue view <N> --json title,body,comments`. Title → issue statement; body + comments → genesis material.
2. **Explicit /todo item** — argument references a `/todo` item (an id like `t_007`, or `/todo <id>`). Read the item from the todo store (`flows/project/todo` → *Store location and freshness*); use `title` + `body` + `scope.evidence` as genesis material.
3. **Explicit .plan path** — argument is a `.plan/<slug>.md` path. Read the file; its *Feature spec* / *Blockers* / *Decisions* sections are the genesis material.
4. **Live session discussion** (default, no argument) — summarize the current transcript: what the user is stuck on, what's been tried, what the open choice is.

**Normalize every source into one internal structure** before going further — downstream stages never branch on source:

```jsonc
{
  "slug": "<kebab-case summary, used for the output filename>",
  "title": "<one-line issue statement>",
  "raw": "<all genesis material: what was tried, what happened, constraints>",
  "register": "tech" | "simple"   // passed by the wrapper
}
```

Derive `slug` from the title (kebab-case, ≤ 40 chars). On an ambiguous or empty source, ask the user which issue the decision PDF should cover rather than guessing.

## 2. The cause→effect intermediate model (IR)

Before rendering anything, build a structured IR from the normalized input. **The PDF renders only from this IR** — so the cause→effect grammar is enforced in one place.

```jsonc
{
  "issue": "<one tight block — a single short paragraph stating the problem>",
  "genesis": [
    { "cause": "<what was tried / what holds>", "effect": "<what happened / what it forces>" }
    // ordered cause→effect links; the story of how we got here
  ],
  "options": [
    { "choice": "<one discrete decision>", "consequence": "<what choosing it causes>" }
    // ≥ 2 discrete, mutually-exclusive choices; NEVER open-ended
  ]
}
```

Rules for building the IR:

- **issue** — collapse the source to one paragraph. If it won't fit one paragraph, the framing is too broad; narrow to the single decision at hand.
- **genesis** — order the links chronologically/causally. Each link is `cause → effect`; no orphan facts. This is the "what was tried → what happened" chain.
- **options** — enumerate the real choices as discrete alternatives, each with its single most-important consequence. If you find yourself writing "do X, or maybe tune Y, or…", that's open-ended — re-cast it as discrete options. A "do nothing / status quo" option is valid when it's a genuine choice.

## 3. The five PDF rules (invariants)

Enforce all five; they are the engine's contract.

1. **Issue = one tight block** — a single short paragraph, never a wall of text.
2. **Solutions are multiple-choice only** — every solution is a discrete option; never open-ended free text.
3. **Cause→effect is the universal grammar** — both the genesis (tried → happened) and every option (choice → consequence) read as cause→effect.
4. **Flowchart-first** — render genesis + options as a flowchart by default; fall to plain text only per the gate below.
5. **Single page is a hard invariant** — the PDF is exactly one page. On overflow, **re-summarize harder** (shorten labels, merge weak genesis links, trim consequence wording) — **never** spill to page 2. If it still overflows after summarizing, drop the least-decision-relevant genesis links before sacrificing any option.

## 4. Flowchart-vs-text gate

Default to a **D2 flowchart**. Fall back to plain text blocks **only** when the chain is trivially linear and short. Explicit simplicity test — use text iff **all** hold:

- genesis has **≤ 2** links, AND
- there are **exactly 2** options, AND
- no genesis link branches (each effect feeds at most one next cause).

If any condition fails (a branch, ≥ 3 options, or a longer chain), render the flowchart. When in doubt, render the flowchart — it's the default.

## 5. Render + output

Render + output per `core/pdf-render`: the bundled D2 + Typst binaries, the `.d2`→SVG→`.typ`→compile mechanics, and the fixed **`.pdfs/<slug>.pdf`** output location (never prompt for it) all live there and are inherited here. This engine specializes that pipeline with:

- The **decision flowchart** it authors (section 6 below), built from the IR — used unless the flowchart-vs-text gate chose text.
- The **one-page Typst template** it compiles (section 7 below).
- **Verify one page** — after compiling, if the PDF is > 1 page, return to rule 5 (re-summarize), regenerate, and recompile. Only then is the run done.

## 6. Decision flowchart skeleton

Specializes the general D2 skeleton in `core/pdf-render`. Fill the labels from the IR. Shape: issue node → ordered genesis cause/effect nodes → a decision node fanning to one node per option, each option → its consequence.

```d2
direction: down

issue: "<issue, one short line>" { shape: rectangle }

# genesis chain: one node per link, chained cause -> effect
g1: "<cause 1>"
g1e: "<effect 1>"
issue -> g1 -> g1e
# ...repeat g2/g2e... for each genesis link, chaining g1e -> g2 -> ...

decision: "<the decision>" { shape: diamond }
g1e -> decision   # last genesis effect feeds the decision

# one branch per option, each to its consequence
opt_a: "<choice A>"
con_a: "<consequence A>"
decision -> opt_a -> con_a

opt_b: "<choice B>"
con_b: "<consequence B>"
decision -> opt_b -> con_b
# ...repeat for each option...
```

## 7. Typst one-page skeleton

Fixed single page. Fill `<…>` from the IR. Drop the `image(...)` line when the gate chose text and inline the genesis links as a short list instead.

```typst
#set page(width: 210mm, height: 297mm, margin: 18mm)
#set text(size: 10pt)

= <issue title>

// Rule 1: issue is one tight block
#block(fill: luma(240), inset: 8pt, radius: 4pt)[
  <issue paragraph — one short paragraph>
]

#v(6pt)

// Flowchart (omit when the gate chose text)
#align(center)[#image("<slug>.svg", width: 100%)]

#v(6pt)

== Options

// Rule 2 + 3: one card per discrete option, choice → consequence
#grid(columns: (1fr, 1fr), gutter: 8pt,
  block(stroke: 0.5pt, inset: 6pt, radius: 4pt)[
    *<choice A>* \ → <consequence A>
  ],
  block(stroke: 0.5pt, inset: 6pt, radius: 4pt)[
    *<choice B>* \ → <consequence B>
  ],
  // ...one block per option...
)
```

## Audience register

The wrapper passes `register` (`tech` or `simple`); it controls how much technical detail enters the **issue block**, the **genesis labels**, and the **option/consequence wording** — same IR, same pipeline, different language.

- **tech** (`/pdf-tech`) — bare-minimum high-level technical framing. The reader knows the system; name components, files, and mechanisms tersely. Genesis and consequences may reference the actual moving parts.
- **simple** (`/pdf-management`) — zero-codebase-knowledge stakeholder. State the **premise only**, in plain language; no file names, no jargon. Choices and consequences are framed in outcomes the stakeholder cares about, not implementation.

The wrappers own the exact phrasing rules for their register; this core only guarantees the register threads through the IR build and into the rendered text.

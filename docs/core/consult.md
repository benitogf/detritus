---
description: Shared consult core — turn the issue under discussion into a single-page decision PDF for a human to act on. Resolves the issue from an explicit source (GitHub issue, /todo item, .plan file) or the live session, builds a cause→effect intermediate model, and renders it flowchart-first through D2 + Typst to .consult/<slug>.pdf. Do not invoke directly; composed by /consult-tech and /consult-simple, which pass an audience register.
triggers:
  - consult core
  - decision pdf
  - consult engine
  - cause-effect model
  - single-page decision aid
when: Internal. Loaded by /consult-tech or /consult-simple to define the input resolution, the cause→effect IR, the five PDF rules, the flowchart-vs-text gate, the D2→Typst render pipeline, and the two templates. The wrappers add only their audience register.
related:
  - flows/consult/consult-tech
  - flows/consult/consult-simple
  - core/todo-import
  - flows/project/todo
  - flows/github/gh-issue-create
---

# Consult Core — the single-page decision PDF

`/consult-tech` and `/consult-simple` are two **audience registers** over **one engine**: take the issue currently under discussion, frame it as a discrete set of choices, and emit a single-page PDF a human reads and decides from. This core owns input resolution, the cause→effect intermediate model, the PDF invariants, the flowchart-vs-text gate, the render pipeline, and the templates. The wrappers add only their register (see *Audience register*). Neither restates what's below.

v1 is **one-shot**: produce the PDF and stop. There is no decision loop-back (the chosen option feeding back into the session is a deliberate v2 split).

> ## ⛔ Do not invoke directly
> No slash command. Loaded via `kb_get` by `/consult-tech` or `/consult-simple`.

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

Derive `slug` from the title (kebab-case, ≤ 40 chars). On an ambiguous or empty source, ask the user which issue to consult on rather than guessing.

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

## 5. Render pipeline

Bundled binaries: **D2** (flowchart → SVG) and **Typst** (one-page PDF). Both are installed beside the detritus binary by detritus setup. If either is missing, stop and tell the user to run `detritus --update` (the setup step fetches Typst + D2); do not attempt a manual install.

Steps, all in the working dir (use a temp dir under `.consult/`):

1. **Author the flowchart** — write `.consult/<slug>.d2` from the IR using the *D2 skeleton* below (skip if the gate chose text).
2. **Render to SVG** — `d2 .consult/<slug>.d2 .consult/<slug>.svg`.
3. **Author the document** — write `.consult/<slug>.typ` from the *Typst skeleton*: it embeds `image("<slug>.svg")` (when present) and the issue + option cause/effect blocks.
4. **Compile** — `typst compile .consult/<slug>.typ .consult/<slug>.pdf`.
5. **Verify one page** — if the PDF is > 1 page, return to rule 5 (re-summarize), regenerate, recompile. Only then is the run done.

Output: **`.consult/<slug>.pdf`** (`.consult/` is gitignored). Report the path; do not open or commit it.

## 6. D2 flowchart skeleton

Minimal, runnable. Fill the labels from the IR. Shape: issue node → ordered genesis cause/effect nodes → a decision node fanning to one node per option, each option → its consequence.

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

- **tech** (`/consult-tech`) — bare-minimum high-level technical framing. The reader knows the system; name components, files, and mechanisms tersely. Genesis and consequences may reference the actual moving parts.
- **simple** (`/consult-simple`) — zero-codebase-knowledge stakeholder. State the **premise only**, in plain language; no file names, no jargon. Choices and consequences are framed in outcomes the stakeholder cares about, not implementation.

The wrappers own the exact phrasing rules for their register; this core only guarantees the register threads through the IR build and into the rendered text.

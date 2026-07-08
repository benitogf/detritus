---
description: Index of learned lessons — durable, cross-project, verified-green insights shipped as KB docs. Each file here is one lesson, published by PR merge and retrieved via kb_search alongside the rest of the knowledge base.
triggers:
  - lessons index
  - learned lessons
  - lesson doc
when: Loaded via kb_get to understand what the docs/lessons/ corpus is and how lessons are added. For an actual task, kb_search across the whole KB — lessons surface automatically.
related:
  - core/memory
  - flows/maintainer/grow
  - flows/maintainer/learn
  - flows/maintainer/absorb
---

# Lessons

Each markdown file in this directory is **one learned lesson**: a durable, cross-project insight distilled
from verified-green work. Lessons are ordinary KB docs — the `go generate` step chunks and indexes every
`.md` under `docs/` (this folder included), so a merged lesson is immediately retrievable via `kb_search`
and readable via `kb_get`, exactly like every other doc.

## How a lesson gets here

Lessons are **never** written at runtime — there is no local store and no agent-write MCP tool (see
`core/memory`). A lesson enters this directory only through a reviewed, merged PR, opened by one of the
maintainer flows:

- `/grow` — distil an in-session correction into a lesson.
- `/learn` — distil a candyland telemetry signal into a lesson.
- `/absorb` — distil the outcomes of a reviewed PR into a lesson.

Each flow generalizes the raw signal and ships it via the normal `/gh` issue → branch → PR → self-review
path. **Merge = publish**; `detritus --update` distributes the released binary (with the new lesson embedded
in `docs/`) to every machine.

## Writing a lesson doc

- One topic per file; a short, kebab-case filename that names the topic.
- Frontmatter with a `description` (used by `kb_list` / retrieval) and, ideally, `triggers`.
- Itemized, applicable bullets — a strategy, concept, or failure-mode, stated so a future agent on a
  *different* task can apply it with judgement.
- Extend an existing lesson (append to its file) rather than duplicating; correct or remove a stale lesson
  in a follow-up PR. The PR diff and its review are the curation — there is no runtime dedup or decay.

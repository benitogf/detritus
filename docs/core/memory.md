---
description: Learned long-term memory — how a verified lesson becomes a durable, shared KB doc under docs/lessons/, published by PR merge, distributed by detritus --update, and retrieved via kb_search. There is no local store and no agent-write path. Do not invoke directly.
triggers:
  - learned memory
  - long-term memory
  - distil lesson
  - recall lesson
  - lessons
when: Internal. Loaded via kb_get to know how learned lessons are stored (as KB docs), how they get published and distributed, and how to retrieve them — and which store a piece of knowledge belongs in.
related:
  - core/completion
  - flows/project/code
  - flows/maintainer/grow
  - flows/maintainer/learn
  - flows/maintainer/absorb
  - flows/principles/truthseeker
---

# Core — Learned memory: one store, shipped as KB docs

Durable, cross-project memory that makes a hive of agents better *behaved* over time, distilled **only**
from verified-successful work. There is exactly **one knowledge store: this repository.** A learned lesson
is a KB doc — a markdown file under `docs/lessons/` — indexed and retrieved by the same `core` engine every
other KB doc uses. Nothing accumulates on a machine: **no local store, no outbox, no staging, no
per-machine curation, and no agent-write MCP tool.**

> ## ⛔ Do not invoke directly
> No slash command. This doc governs *where* learned lessons live and *how* they flow. The act of
> distilling and shipping one is done through the flows below (`/grow`, `/learn`, `/absorb`); retrieval is
> the ordinary `kb_search` → `kb_get` path.

## The loop: distil → generalize → ship → publish → distribute → retrieve

1. **Distil** at a **verified-green milestone** (`core/completion`'s exit gate) — never from unverified
   work. Distil only what is **reusable** (a strategy, concept, or failure-mode useful on a *future,
   different* task) and **cross-project** (true beyond this one repo).
2. **Generalize + ship** through the maintainer flows: `/grow` (an in-session correction),
   `/learn` (candyland telemetry), or `/absorb` (a PR with review outcomes). Each distills the raw signal,
   generalizes it, and lands it as a lesson doc under `docs/lessons/` **via the normal `/gh` issue → branch
   → PR → self-review path.** The PR review loop is the quality gate — the same one every KB change passes.
3. **Publish** = the PR **merges.** A merged lesson doc is a published lesson. There is no separate
   "contribute" or "upstream" step.
4. **Distribute** = `detritus --update`. Every consumer pulls the released binary, whose embedded `docs/`
   now carries the new lesson. The lesson reaches every machine the same way every other doc does.
5. **Retrieve** at the **start of a similar task** with `kb_search(query)` — it returns ranked keys +
   snippets across the whole KB, lessons included — then `kb_get` the promising one in full. A retrieved
   lesson is **context to apply with judgement — never auto-executed.**

## Consolidation is a PR, not a write

Deciding a new insight is already captured, extends an existing lesson, or contradicts one is authorship
judgement expressed **in the lesson PR**: append to an existing `docs/lessons/*.md`, add a new file, or
correct/supersede a stale claim in place. The PR diff and its review are the audit trail — there is no
mechanical dedup pass and no supersede flag, because there is no runtime store to curate.

## Curation happens in review

The corpus stays bounded and trustworthy the same way the rest of the KB does: a human (or the self-review
loop) reviews every lesson PR before merge. Stale or wrong lessons are edited or removed by a follow-up PR.
There is no age/LRU/cap machinery and no recall-miss counter, because lessons are static docs compiled into
the binary — not agent-written entries decaying in a live store. If retrieval ever proves weak at scale,
that is a change to the shared `internal/search` engine (which serves all docs), decided by measurement.

## Boundary — which store

- **Agent-discovered, verified, reusable, cross-project lessons** → a KB doc under `docs/lessons/`, shipped
  by PR (via `/grow` / `/learn` / `/absorb`).
- **Repo-specific facts** → the project's native `MEMORY.md` (in-repo), not here.
- **Human-authored conventions / skills / docs** → the rest of the curated KB (`kb_*`). Lessons are simply
  the machine-originated corner of that same store; they earn their place by passing PR review.
- **Code structure for the current repo** → `flows/project/code` (`code_map`/`code_outline`/`code_graph`).
- **Editing detritus's own KB** → `/grow`, not a runtime write path.

## Security

There is **no agent-write path into the store** — the only way a lesson enters the KB is a reviewed, merged
PR, so the trust boundary is the review, not a runtime firewall. Retrieved lessons are context an agent
*applies*, never auto-executed. This is strictly tighter than a live agent-authored store: a shared
learning store is a shared attack surface, and here the surface is a code-review, not an open write API.

## What this doc is not

- Not a slash command — `core/memory` is `kb_get`-only.
- Not the completion doctrine (`core/completion`) — it only names the verified-green milestone where
  distillation happens.
- Not the flows that ship a lesson — those are `flows/maintainer/grow`, `flows/maintainer/learn`, and
  `flows/maintainer/absorb`.
- Not code context (`flows/project/code`) or a separate lessons index (`docs/lessons/` is indexed by the
  ordinary KB engine).

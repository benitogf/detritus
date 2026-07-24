---
description: Durable per-topic session context — serialize load-bearing state to .checkpoint/<slug>.md at a boundary so a /clear is lossless and a later session (hours or weeks on) resumes the topic by name. The priority-aware alternative to /compact.
triggers:
  - checkpoint
  - save context
  - save session
  - checkpoint and clear
  - safe to clear
  - session context doc
  - resume checkpoint
  - resume topic
argument-hint: "[resume|list|archive] [slug] [note]"
when: User wants to checkpoint session context before a /clear, or resume/list/archive a checkpointed topic.
related:
  - core/loop
  - flows/maintainer/setup-extra-rules
  - flows/project/todo
  - flows/project/code
  - core/memory
  - core/flows
  - flows/github/babysit
---

# /checkpoint — Durable Per-Topic Session Context

`/checkpoint` serializes the **load-bearing** context of a **workstream topic** to a durable doc (`.checkpoint/<slug>.md`) at a semantic boundary, so a `/clear` after it is lossless and a fresh session — minutes or **weeks** later — resumes that topic from the doc alone. It is the **priority-aware alternative** to `/compact`'s lossy uniform summarization: it keeps what matters (a fixed schema of load-bearing fields) and drops the re-derivable bulk.

It **composes `core/loop`** — the checkpoint-then-`/clear` discipline and the State-block spine (overwrite-not-append, pruning) are defined there and referenced here, not restated. Where `core/loop` scopes that discipline to recurring/partitioned loops (`/janitor`, `/smith`, `/forge`'s `.plan/PROGRESS-<slug>.md` ledger) keyed on a loop slug, `/checkpoint` brings the *same* spine to any in-session effort with no loop ledger — a `/caregiver` review-watch, an ad-hoc multi-step task, a debugging session — and derives its slug differently: **from the topic**, not from `core/loop`'s whole-repo/whole-workspace slug table (see *Identity*). The State-block spine reference stands; the slug rule does not carry over.

## Identity — one long-lived topic per file

A checkpoint is **one durable, long-lived topic** that spans many sessions: open → `/clear` → resume → maybe go quiet for a week → revive. It is **not transient** and is **never auto-evicted**.

- **Per-workstream, session-claims-it.** A session **claims** a slug on its first checkpoint or resume, **prints** it, and writes to that same file for the rest of its life — so the session always knows its own file and subsequent checkpoints round-trip to it.
- **Long-lived — merging a PR does NOT end the topic.** You come back to it: a follow-up, a regression, the next phase of the same work. There is no "in-flight until merge" lifecycle and no eviction on a terminal event. (This is the explicit **contrast with `/todo`**, where *done = eviction, no archive*: a todo item is retired the moment it lands; a checkpoint topic **outlives** its deliveries by design — see *Set hygiene*.)

### Slug — agent-derived from the topic

The **slug is agent-derived from the topic of the work**, kebab-case and **human-readable** so a dev can find the file and hand it to another dev: `inventory-ui`, `checkpoint-skill`, `fleet-deploy-perf`. Readability is the point — a person names their topic, not an opaque key.

- **Not the session id** — opaque, and it changes on resume, which would break the round-trip.
- **Not the git branch** — the workspace is **multi-repo with worktrees** ("multiversal"): a single topic spans several repos/branches, so there is no one branch key to name the file by.
- **Explicit `/checkpoint <name>` overrides the derivation** when the agent's guess is wrong. The user's name always wins.

## The doc — `.checkpoint/<slug>.md`

At the **workspace root**, gitignored (if `.checkpoint/` is not already gitignored, add it at first write). It composes `core/loop`'s State-block spine **by reference** — this is that spine's load-bearing fields for a general, multi-repo topic, not a new persistence mechanism. An optional `[note]` folds into *Current orientation*.

Schema (the load-bearing set — **multi-repo aware**):

- **Current orientation** — one or two lines: the active focus right now.
- **Decisions + why** — the calls made and the reasoning that selected each, so a resume doesn't relitigate them.
- **Repos / worktrees touched** — each repo/worktree the topic spans. First-class because the workspace is multi-repo; a resume must re-orient across *all* of them, not one.
- **Artifacts — portable pointers** — **repo + branch + SHA + PR URL** for each artifact. **Never absolute local worktree paths** — they mean nothing on another machine or to another dev. Portability is what makes dev-to-dev handoff and repo-filtering (`/checkpoint list <repo>`) work.
- **In-flight watches & background tasks (to re-arm on resume)** — every session-scoped live thing: an armed `/babysit` or `/caregiver` review-watch, background tasks, sub-agents. See *In-flight state safety*.
- **Next move** — the concrete first action the resumed session takes.
- **Open questions** — unresolved threads the next session must not drop.
- **Last user directive** — the most recent pivot or scope change, **dated**.

**Explicit exclusions — pointers, not payloads.** The doc never carries code, diffs, or file bodies. Git and GitHub hold those **losslessly**; the doc records *where* they are (a SHA, a PR URL) and the resumed session re-reads them. Copying them in is exactly the uniform-summary bloat `/checkpoint` exists to avoid.

**Overwrite, not append.** Each checkpoint rewrites the doc — a current-state snapshot, not a log. Prune per `core/loop` → *Pruning* if any narrative accumulates.

## Writing

Two paths write the doc; resume never does.

- **Manual `/checkpoint` — deterministic, the primary path.** You invoke it at a boundary — a delivery landed, a plan settled, the user pivots, you're about to step away — and it always writes. The trigger is a **boundary the work reached**, never a gauge: **the model has no context-fullness signal**, so "checkpoint when context is low" is not a thing it can do.
- **Best-effort boundary auto-checkpoint — opt-in, probabilistic.** An ambient rule (generated by `flows/maintainer/setup-extra-rules` → *Phase 2*, like the other detritus-* rules) nudges the model to `/checkpoint` when it notices a boundary. **Labeled best-effort**: no fullness signal, and boundary-detection is a judgment call — it *can miss*. It **supplements** the manual path; it never replaces it.

After writing, the skill ends with the literal line `checkpoint complete — safe to /clear`. The **user** then presses `/clear` — it is always the user's keystroke; the skill **never** presses or triggers it (verified impossible for any tool/hook/skill — see *Guardrails*).

## Resume — pull, by name, propose-don't-load

Resume is a **pull**: the user reaches for a topic; nothing is injected or greeted at them. There is **no proactive greeting on session start** (host-neutral, needs no hook), and **no silent auto-load** — in a multiversal workspace, loading the *wrong* topic's context is the worst outcome.

- **Primary — `/checkpoint resume <slug>`.** The user names the topic. They know their topics, and the slugs are readable, so naming is the fast path.
- **Smart assist — content-match, propose then confirm.** If the user just *describes* the work instead of naming it, the agent content-matches their intent against each checkpoint's *Current orientation* + *Last user directive* lines and **proposes** a topic — "looks like `inventory-ui` — resume?" — then waits for confirmation. It **always proposes and confirms; it never silently auto-loads**.
- **Re-orient across ALL repos, re-arm all watches.** On resume the session re-orients to **every** repo/worktree in *Repos / worktrees touched* and the portable *Artifacts* pointers — not just one repo — and **re-arms** the in-flight watches recorded in the doc (see *In-flight state safety*).

### Bare `/checkpoint` — state-aware dispatch

Bare `/checkpoint` does the right thing on either side of a `/clear`:

- **Session HAS meaningful state to serialize** → it **writes** (the normal case).
- **Session is fresh / just-cleared with nothing worth saving** → it treats the invocation as **resume intake**: shows the active-topics list (per *`/checkpoint list`*) and offers to resume one.

One verb, dispatched by whether there is state to save.

## Set hygiene — persist indefinitely; list to navigate; archive is opt-in

Checkpoints **persist indefinitely** — a topic can revive anytime — and are **never auto-evicted**, not even on a PR merge. (Again the deliberate inverse of `/todo`'s *done = eviction*: the topic outlives the delivery.) A large durable set stays navigable via a searchable index, not a flat every-time pick-list:

- **`/checkpoint list [filter]`** — the discovery index: topics **newest-first**, each row `slug · age · one-line orientation`. Filterable by **recency**, **repo touched**, or **keyword** — `/checkpoint list bulk` shows topics touching the bulk repo (matched against *Repos / worktrees touched* and the portable *Artifacts* pointers, which is why those fields are portable).
- **`/checkpoint archive <slug>`** — **opt-in, user-invoked only**, for when the *user* decides a topic is truly retired. Never automatic, never behind your back. Archive just moves the topic out of the active set — a `.checkpoint/archive/` subdir — so `list` stays focused; the doc is not deleted and can be restored.

## Hooks are optional

The **entire flow works with zero `flows/maintainer/setup-extra-rules`**: manual write, name-based and content-match resume, list, and archive are all host-neutral and need no hook (the opt-in best-effort auto-write is the one setup-extra-rules-gated extra, and it is not required). Two paths do the **writing** (manual, primary + that opt-in best-effort auto-write), and name / content-match does the **resume**. That is the whole model.

Because resume is a pull — no auto-inject, no greeting — the SessionStart hook's role shrinks to almost nothing. It is a **footnote, not a layer**: optionally, a SessionStart hook (via `flows/maintainer/setup-extra-rules`) can *surface your active-topics list* on a fresh session so you see what's resumable — but it is **not required**; bare `/checkpoint` does exactly this on demand. The hook surfaces the list; it does **not** auto-inject any specific doc as the resume mechanism.

## In-flight state safety

> `/clear` **kills** session-scoped live state — an armed event-watch (`/babysit` or a `/caregiver` review-watch), background tasks, sub-agents — that `/compact` would have kept alive. A replacement for `/compact` must not silently drop what `/compact` preserved.

The **In-flight watches & background tasks** field captures every such live thing, and **resume re-arms them**. A silently-dropped watch is the exact stall class `flows/github/babysit`'s no-stall contract forbids: a PR left open with **no event-watch armed** never wakes again. So checkpointing before a `/clear` must record the watch, and the resumed session must re-arm it — otherwise checkpoint-then-`/clear` quietly kills the loop. Never checkpoint a session with a live watch without recording it here.

## Non-overlap

`/checkpoint` holds **session/topic state**; it points at its neighbours, never duplicates them.

- **`/todo`** (`flows/project/todo`) — cross-session *tasks*. A checkpoint **points at todo ids**, never duplicates the items; the todo store is their home. (And unlike `/todo`, a checkpoint is never evicted on completion — see *Identity* / *Set hygiene*.)
- **`core/memory`** — durable cross-project **facts**, landed via a PR merge. Not session state; a checkpoint never writes memory.
- **`.plan/<slug>.md`** (`core/planning`) — the settled **plan**. A checkpoint **references it**, is not it.
- **`/forge`'s `.plan/PROGRESS-<slug>.md` ledger** (`flows/build/forge`) — **loop state**. Inside an active loop, `/checkpoint` **defers to the loop's own ledger** — the ledger is already the resume truth, and a second `.checkpoint/` doc would fork it. One resume truth per effort.

## Guardrails

- **Never claim to press or trigger `/clear`.** It is the user's keystroke — verified impossible for any tool/hook/skill. End with `checkpoint complete — safe to /clear` and let the user press it.
- **Overwrite, don't append** — the doc is a current-state snapshot.
- **The doc is the ONLY resume truth** — never rely on `/compact` (`core/loop` → "Never rely on `/compact`"). The schema must let a fresh session resume from the doc alone.
- **The best-effort auto-write is labeled best-effort** — it can miss (no fullness signal); never present it as guaranteed.
- **Never auto-evict a topic** — checkpoints persist indefinitely; a PR merge does not retire one. Only the user archives, via `/checkpoint archive`. (The inverse of `/todo`'s done-eviction.)
- **Never silently auto-load a topic** — always propose and confirm on a content-match; loading the wrong topic in a multiversal workspace is the worst outcome.
- **Keep artifact pointers portable, and scrub private names** — repo + branch + SHA + PR URL, never local worktree paths, so the doc survives dev-to-dev handoff (and often becomes a PR body / issue). Public-repo discipline per `/gh` stands even though the doc is local and gitignored.
- **Defer to a loop's own ledger inside a loop** — one resume truth per effort.
- **Skip for casual / one-shot sessions** — `/checkpoint` earns its cost only when there's load-bearing state (decisions, artifacts, live watches) worth serializing.
- **Incident hook.** A self-acknowledged mistake/doctrine violation ("you are right, I …", "I didn't follow …", "I ignored /…") or a blocker surfacing on a PR this flow authored is an incident — detect and route per `core/ego` (→ `/grow` / `/absorb`), after finishing the deliverable.

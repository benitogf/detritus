---
description: Add a new cross-session todo. Sub-agent infers scope and assigns initial priority (score + tier + rationale).
category: meta
triggers:
  - todo-add
  - add todo
  - track this
  - remember to
  - new todo
when: User asks to capture a new ad-hoc item that should persist beyond the current session.
related:
  - meta/todo
  - meta/todo-audit
---

# /todo-add — Add a New Todo

_Follows `meta/todo` convention #13: main session validates input + calls TodoWrite + prints the confirmation line; the phases below describe the work the delegated sub-agent performs._

Capture a new item, persist it to `~/.claude/projects/<slug>/todos.json`, and let a Haiku sub-agent infer initial scope + priority.

## Inputs

- `/todo add <text>` — the item text. Required.
- Optional hints in free text: `--p0` / `--p1` / etc. (user-stated tier), `--group "<name>"` (user-stated group), `--tag <tag>`, `--repo <repo>`, `--path <path>`.

If text is missing, ask the user one concise question and stop.

## Phase 1: Read the store

- Resolve `~/.claude/projects/<slug>/todos.json` (Phase 0 of `meta/todo`).
- If missing, lazily create with `{ version: 2, epoch: 0, items: [] }` and continue (schema defined in `meta/todo` convention #2).
- Capture `epoch`.

## Phase 2: Infer scope + priority (Haiku sub-agent)

Spawn a Haiku sub-agent with:
- The new item text.
- The current conversation's last 2–3 turns (so the sub-agent can infer scope from context — which repo, which files, what the user is working on).
- The current list of items (id + text + scope summary) so the new item is ranked relative to them, not in a vacuum.

The sub-agent returns a structured object:

```jsonc
{
  "title": "Implement the v2 janitor-side write hook",      // short, imperative, ≤80 chars
  "body": "Closes the loop on /todo <-> /janitor integration; janitor's audit phase auto-writes deferred findings into todos.json each tick.",  // optional longer description; null if title is sufficient
  "group": "Detritus skill development",  // human-readable title; null if no clear cluster
  "scope": {
    "repos": ["benitogf/detritus"],
    "paths": ["docs/meta/janitor.md"],
    "evidence": [],                    // file:line refs for bug-shaped items; empty for general tasks
    "concreteness": "concrete",        // concrete | ambiguous | exploratory
    "knownBlockers": []
  },
  "priority": {
    "score": 55,
    "tier": "P2",
    "rationale": "Useful follow-up; not blocking current work."
  },
  "tags": ["detritus", "janitor"]
}
```

**Title vs body**: the sub-agent splits the user's input into a short imperative `title` (≤80 chars, used as the TodoWrite content text) and an optional `body` (longer description, shown in detail-mode only). For short user input (one sentence), `body` is null. For longer input (paragraph or multi-sentence prose), the first sentence usually becomes the title and the rest becomes the body. The user's explicit input is preserved — the sub-agent doesn't paraphrase or invent content.

**Group inference**: the sub-agent picks an existing group name when the new item clearly belongs to one (e.g. all the existing items under `"Detritus skill development"` plus a new one about `/todo-fork` → same group). Creates a new group name only when the item doesn't fit any existing cluster. Group names are short, human-readable, and Title Case (`"Bulk pivot performance"`, not `"bulk-perf"`). If the user passed `--group "<name>"`, honor it verbatim — don't override.

**Evidence inference**: if the user's input includes file:line refs (e.g., `services/auth/locker.go:142` or `internal/stream/stream.go:89-94`), parse them into the `scope.evidence` array as `{file, line, range?}` objects. For bug-shaped items, this is critical context. Don't fabricate evidence — only parse what's literally present.

**Concreteness rubric** (sub-agent applies):
- `concrete` — names a specific repo + specific files/sections + specific action ("add X to Y", "fix Z bug", "rename A to B"). Eligible for forking. Items with `scope.evidence` populated are almost always concrete.
- `ambiguous` — verbal scope but missing target (specifying *what* without *where*, or vice versa). Needs clarification.
- `exploratory` — verbs like "explore", "investigate", "research", "decide", "figure out"; placeholders like "TBD", "?", "either X or Y". Never eligible for forking until concretized.

**Priority rubric — two paths based on item shape**:

The sub-agent applies ONE of two rubrics based on whether the item is bug-shaped or general:

**(A) Bug-severity heuristic** (when item text contains bug signals — *race, panic, leak, off-by-one, security, regression, data loss, deadlock, use-after-free, broken invariant, silent failure, corruption*): rank by severity tier in this order (highest priority first):
1. **Correctness bugs** that can corrupt state, lose data, or panic — races, data loss, panics, unbounded resource consumption, security holes, broken invariants. → P0/P1, score 80-95.
2. **Behavior bugs** — wrong output, missing enforcement of a documented constraint, broken error handling, silent failures. → P1, score 65-79.
3. **Resource leaks and performance regressions** — memory/goroutine leaks, lock contention on hot paths, unbounded buffers. → P1/P2, score 55-70.
4. **Architectural smells with concrete consequences** — duplicated logic, missing locks where siblings have them, bypassed middleware. → P2, score 40-55.
5. **Code-quality cleanup** — style, naming, dead code, fragile string concatenation. → P3, score 20-40.
6. **Documentation, comments, cosmetic** → P3, score 5-25.

**(B) General urgency × impact rubric** (when the item is NOT bug-shaped — feature work, design decisions, follow-ups, tasks): score 0–100 weighted by urgency (deadline/blocker), impact (high/low), dependency (X blocks Y → boost X), context-match (matches the current conversation focus → modest boost), effort (quick wins boosted slightly). Tier follows: 80+ → P0, 60–79 → P1, 35–59 → P2, <35 → P3.

If the user passed an explicit tier hint (`--p0` etc.), honor it but let the sub-agent fill score and rationale around it. Bug-severity ordering is the default for bug-shaped items unless the user overrides.

## Phase 3: Mutate

- Generate a new id: `t_<NNN>` where N is one greater than the max existing item id.
- Build the item per the schema in `meta/todo` convention #2:

```jsonc
{
  "id": "t_004",
  "title": "<from Phase 2 — short imperative one-liner>",
  "body": "<from Phase 2 — optional longer description, or null>",
  "group": "<from Phase 2, or null>",
  "status": "open",
  "priority": <from Phase 2>,
  "scope": <from Phase 2 — includes evidence array>,
  "addedAt": "<ISO timestamp>",
  "deferredUntil": null,
  "forkSession": null,
  "deps": [],
  "tags": <from Phase 2, merged with --tag hints>,
  "source": "user"
}
```

- Re-read the file. If `epoch` has changed since Phase 1, restart from Phase 1 (another session wrote first).
- Bump `epoch` by 1, update `updatedAt`, append the new item, write atomically (write to `todos.json.tmp` then rename).

## Phase 4: TodoWrite sync

Per `meta/todo` convention #10, call `TodoWrite` with the current open + in-progress items so Claude Code's native todo UI reflects the new state. Use the rendering and field mapping defined in `meta/todo-view` Phase 4.

## Phase 5: Report

Print one line per `meta/todo` convention #11 — no id, no score, no tier in user-facing output:

```
Added under "Detritus skill development": <title>.
```

If the sub-agent classified the item as `ambiguous` or `exploratory`, append a one-line hint: *"This item's scope is currently ambiguous — won't be fork-eligible until /todo-edit makes it concrete."*

If `group` was null (no clear cluster), append: *"No group inferred — item is Ungrouped. Use /todo-edit --group '<name>' to file it under a title."* (No id in the suggested command — the user can use fuzzy text or the positional view to find the item.)

## Guardrails

- Don't add a duplicate. Before writing, perform **root-cause-aware dedup** against the existing item set:
  - First pass: substring overlap on `title` and the first 40 chars of `body` against each existing item.
  - Second pass: if the candidate has `scope.evidence`, compare evidence overlap against existing items (same file:line span = strong dedup signal even if titles differ).
  - Third pass (Sonnet sub-agent escalation): semantic comparison — "does this item describe the same root cause as any existing item, expressed differently?" Catches paraphrases, alternate phrasings, and same-bug-different-callsite cases. Only escalate to Sonnet when the first two passes return no clear match AND the candidate has a structured shape that warrants semantic check (bug-shaped items with evidence, items longer than 100 chars).
  - On any match, surface the candidates via `AskUserQuestion` by **positional index** (`1.`, `2.`, ...) — never by internal id, per convention #11 — and ask whether this is a new item, a refinement of an existing one (which should be a `/todo-edit` instead), or a merge candidate (append evidence/body to the existing).
- Don't fabricate scope. If the sub-agent can't infer a concrete scope from the text + recent conversation, the item gets `concreteness: ambiguous` — don't guess paths to look concrete.
- Don't ignore the epoch-check on write. Always re-read before write; restart on epoch mismatch.
- New items always land in `items` with `status: open`. There is no archive (completed items are evicted by `/todo-done`, not stored).

---
description: Bulk-import a structured list of items into todos.json under a shared group. Used by /plan when the user confirms a plan; also callable directly for any structured plan handoff.
category: meta
triggers:
  - todo-import
  - import todos
  - persist plan
  - import plan
when: /plan reaches user confirmation and needs to persist its settled steps as cross-session todos; or the user has any structured list (audit findings, /smith spec, external source) they want to bulk-add under one group title.
related:
  - meta/todo
  - meta/todo-add
  - plan/index
---

# /todo-import — Bulk Import Items Under a Shared Group

_Follows `meta/todo` convention #13: main session validates input + calls TodoWrite + prints the confirmation line; the phases below describe the work the delegated sub-agent performs._

Accept a structured list of items + a `group` title + a `source` tag, and write them all to `~/.claude/projects/<slug>/todos.json` in one atomic mutation. Equivalent to running `/todo-add` N times, but with single-atomic semantics (all-or-nothing) and shared group/source — useful when /plan, /smith, or another skill hands off a settled plan as todos.

## Inputs

Two shapes:

### Shape A: Direct args (caller-friendly)

```
/todo-import --group "Detritus skill development" --source plan
  - Add `group` field to /todo schema
  - Render markdown headers in /todo-view
  - Wire TodoWrite sync after every mutation
  - Add /todo-import sub-skill for bulk handoff from /plan
```

Each line under the flags becomes one item. The caller may also pass `--tier <Pn>` to apply a uniform tier to all items (otherwise the sub-agent infers per item).

### Shape B: Structured invocation (skill-to-skill)

Other skills (notably `/plan`) invoke this skill programmatically with a JSON-ish payload:

```jsonc
{
  "group": "Detritus skill development",
  "source": "plan",
  "items": [
    {
      "title": "Add `group` field to /todo schema",
      "body": null,
      "scope": { "repos": ["benitogf/detritus"], "paths": ["docs/meta/todo.md"], "concreteness": "concrete" },
      "tags": ["detritus", "todo"]
    },
    {
      "title": "Render markdown headers in /todo-view",
      "body": null,
      "scope": { "repos": ["benitogf/detritus"], "paths": ["docs/meta/todo-view.md"], "concreteness": "concrete" }
    }
  ]
}
```

In this shape the caller has done the scope inference work. Sub-agent only fills in missing priority fields.

## Phase 1: Read the store

- Resolve `~/.claude/projects/<slug>/todos.json` (Phase 0 of `meta/todo`).
- If missing, lazily create.
- Capture `epoch`.

## Phase 2: Dedup against existing items

For each candidate item, scan existing items for near-matches using the same **root-cause-aware dedup** as `/todo-add` (substring overlap on title+body, evidence-line overlap, then Sonnet escalation for semantic-equivalence judgment on bug-shaped items). If a near-match exists, surface the duplicates to the user via `AskUserQuestion` with four options per duplicate:
- **Skip** — drop the candidate (it duplicates an existing item)
- **Add anyway** — both items end up in the list (the user knows they're related but wants them separate)
- **Merge** — append the candidate's evidence and body details to the existing item (preserves new information without creating a duplicate row)
- **Replace** — `/todo-edit` the existing item with the new title/body/scope (treat the candidate as a refinement)

If more than 4 duplicates surface, cap the question at 4 and skip the rest (surface the count in the final report).

## Phase 3: Sub-agent fills missing fields (Haiku)

Per-item, the Haiku sub-agent fills in:
- `priority` (score + tier + rationale) — if not provided by the caller
- `scope.concreteness` — if not provided; defaults to inferring from title+body per `meta/todo-add` rubric
- `scope.evidence` — if the title or body mentions file:line refs, extract them into the array per `meta/todo-add`'s evidence-inference rule
- `tags` — appends any auto-inferred tags to caller-provided tags

For very small imports (< 3 items), the sub-agent may be skipped if the caller provided full structured data for every item (no missing fields). Otherwise spawn the sub-agent once for the whole batch (cheaper than per-item).

## Phase 4: Mutate (one atomic write)

- Generate ids for each new item starting from `max(existing id) + 1`.
- Build each item with the standard schema (including the shared `group`, `source: "<from caller>"`).
- Re-read the file once; if `epoch` changed, restart from Phase 1.
- Bump `epoch` by 1 (not by N — the whole batch counts as one epoch increment), update `updatedAt`, write atomically.

## Phase 5: TodoWrite sync

Per `meta/todo` convention #10, call `TodoWrite` with the new full open + in-progress list so the IDE UI reflects the import.

## Phase 6: Report

Per `meta/todo` convention #11 — no ids, scores, or tiers in user-facing output:

```
Imported 4 items under "Detritus skill development" (source: plan):
  • Add group field to /todo schema
  • Render markdown headers in /todo-view
  • Wire TodoWrite sync after every mutation
  • Add /todo-import sub-skill for bulk handoff from /plan
2 duplicates skipped (already tracked).
```

If any items were rejected during dedup or had unresolvable scope, list them by reason at the end so the user can /todo-add manually with adjusted title.

## Guardrails

- **Atomic batch write**. All items land in one mutation, or none do. A failure mid-import (write error, schema validation fail) reverts to pre-import state.
- **Caller declares source**. `source` is required — `"plan"`, `"smith"`, `"audit"`, `"janitor"`, `"manual"`, etc. Lets `/todo-view --source plan` filter by origin later.
- **Never invent a `group`**. Either the caller passes one or the sub-agent infers one from the items' shared theme — if neither produces a clear name, set `group: null` (Ungrouped) rather than guessing.
- **Don't re-prioritize existing items during import**. Imports are additive. Re-ranking against the import's effect on the rest of the list is `/todo-audit`'s job, not this skill's.
- **Dedup is user-confirmed**. Auto-merging duplicates is the failure mode this guardrail exists to prevent — surface and ask.
- **Bypass scope inference only for full-structured Shape-B inputs**. If the caller passed title-only items (no body, no scope), the sub-agent must run scope inference. Title-only is a valid input when the caller knows the structure is already settled.

---
description: Re-text or re-scope a todo without losing its priority history. Useful for sharpening ambiguous items so they become fork-eligible.
category: meta
triggers:
  - todo-edit
  - edit todo
  - change todo
  - sharpen todo
when: User wants to refine a todo's text, scope, tags, or dependencies — often to move it from ambiguous to concrete so it can be forked.
related:
  - meta/todo
  - meta/todo-fork
---

# /todo-edit — Re-Text or Re-Scope a Todo

_Follows `meta/todo` convention #13: main session validates input + calls TodoWrite + prints the confirmation line; the phases below describe the work the delegated sub-agent performs._

Mutate an existing item's `title`, `body`, `scope` (including `evidence`), `tags`, or `deps` without resetting its `priority` history or `addedAt`. The most common use case: the user wrote a vague item, and now wants to sharpen it so `/todo-fork` will accept it as concrete.

## Inputs

- `/todo edit <id> --title "<new title>"` — replace the item's `title` (short, imperative, one-line).
- `/todo edit <id> --body "<new body>"` — replace the item's `body` (longer description). Pass `--body ""` or `--clear-body` to drop the body entirely.
- `/todo edit <id> <new text>` — bare-arg form: when the user passes plain text without flags, the sub-agent splits it into title + body using the same first-sentence rule as `/todo-add` Phase 2. For one-sentence input, the result is title-only (body cleared). Convenience shorthand.
- `/todo edit <id> --evidence <file:line>[,<file:line>...]` — append file:line refs to `scope.evidence`. Each ref is `path/to/file.go:NN` or `path/to/file.go:NN-MM`.
- `/todo edit <id> --no-evidence <file:line>[,<file:line>...]` — remove specific file:line refs from `scope.evidence`. Pass `--clear-evidence` to drop all.
- `/todo edit <id> --group "<name>"` — move the item to a different group title.
- `/todo edit <id> --ungroup` — remove the item from its group (becomes Ungrouped).
- `/todo edit <id> --scope <repos,paths,concreteness>` — replace scope (excluding evidence, which has its own flags).
- `/todo edit <id> --tag <tag>` — add a tag.
- `/todo edit <id> --untag <tag>` — remove a tag.
- `/todo edit <id> --dep <other-id>` — add a dependency (this item is blocked by `other-id`).
- `/todo edit <id> --undep <other-id>` — remove a dependency.
- `/todo edit <id> --undefer` — clear `deferredUntil` and set status back to `open` (early un-defer).

Multiple flags can be combined: `/todo edit t_003 --title "Rewrite manifest" --body "..." --evidence pivot/router/videopack_deploy.go:142 --group "Bulk pivot performance" --tag urgent`.

If only an id is given with no changes, treat as `/todo-view <id>` and prompt the user for what to change.

## Phase 1: Resolve + validate

- Read the store.
- Resolve `<id>` per `/todo-done` Phase 1: accept internal id (`t_NNN`) OR fuzzy substring match; positional-index disambiguation on multiple matches.
- If the item has `status: in-progress` with `forkSession` set, surface a one-line warning: *"This item is claimed by a fork — editing now will affect that fork's assignment context."* Ask the user to confirm before proceeding. Don't reveal the fork id in the user-facing message.

## Phase 2: Apply edits

- Title change (via `--title` or bare-text-with-split): replace `title` verbatim. If the new title is materially different from the old (substring overlap < 30%), this counts as a "rewrite" — flag in the report so the user notices.
- Body change (via `--body` or `--clear-body`): replace or clear `body`. Empty string and `--clear-body` both set `body: null`.
- Bare-text form (no flags): split the input into title (first sentence) + body (remainder), per `/todo-add` Phase 2's split rule. For one-sentence input, the result is title-only and body is cleared.
- Scope change: replace `scope` verbatim. If the new scope's `concreteness` changes from `ambiguous`/`exploratory` to `concrete`, also re-run a quick Haiku rescore to refresh the priority (since concreteness affects rankability).
- Evidence change (via `--evidence` / `--no-evidence` / `--clear-evidence`): mutate the `scope.evidence` array. Append, remove specific refs, or clear all. Validate file:line format (`path/file.ext:NN` or `path/file.ext:NN-MM`) before accepting.
- Tag changes: add to / remove from the `tags` array.
- Dep changes: add to / remove from the `deps` array. Validate that the dep id exists; if not, surface an error and stop.

Re-validate `priority.score` and `priority.tier` against the new state: if `scope.concreteness` changed, ask a Haiku sub-agent to re-score this single item. Other items are unaffected.

## Phase 2a: Detect decomposition — route to /todo-import

If the edit request produces a multi-step breakdown for the item (numbered list, sub-tasks, "break into N steps", any pattern where the edit would result in N actionable items rather than a single revised text), the skill MUST NOT store the breakdown inside the item — that's the schema violation banned by `meta/todo` convention #12.

Instead:

1. Don't touch the parent item's `title`, `body`, `scope`, or `priority` based on the breakdown.
2. Hand off to `/todo-import` (Shape B, structured) with the N sub-tasks. Set:
   - `group` — same as the parent item's group.
   - `source` — `"decomposition"`.
   - Each item gets `tags` including `"sub-task"`.
   - Chain `deps` so sub-task N+1 depends on sub-task N when the ordering is meaningful (sequential build steps); leave `deps` empty when sub-tasks are independent.
   - Priorities decrease across the sequence so step 1 sorts highest (sub-agent assigns).
3. After `/todo-import` returns, ask the user via `AskUserQuestion` what to do with the parent item:
   - **Keep as rollup** (default): leave the parent's title + body + priority intact; it stays as the high-level goal alongside the new sub-tasks.
   - **Lower priority** to sort below the sub-tasks (parent becomes a context note).
   - **Mark done** since it's been decomposed.
   - **Drop** (archive immediately).
4. Apply the user's choice as a normal edit on the parent.

**Forbidden**: writing a `steps`, `breakdown`, `subtasks`, or any custom array field on the parent item. The schema in `meta/todo` is the only one; any extension fields are out-of-spec and `/todo-audit` will surface them as schema violations to be migrated to individual items.

If `/todo-import` is unavailable at runtime (older binary, missing skill), surface the error and stop — do not fall back to writing the breakdown into the parent's title or body.

## Phase 3: Mutate

- Capture `epoch`. Re-read on write. Bump `epoch`, update `updatedAt`, write atomically.
- Stamp `editedAt` on the item (new field, optional, only present after first edit).

## Phase 3b: TodoWrite sync

Per `meta/todo` convention #10, call `TodoWrite` with the current open + in-progress items (using the rendering and field mapping in `meta/todo-view` Phase 4) so the IDE's native UI reflects the edit.

## Phase 4: Report

Per `meta/todo` convention #11 — no ids, scores, or tiers in user-facing output:

```
Edited: "<text>". Text changed (rewrite — low overlap). Scope concreteness: ambiguous → concrete.
```

If the concreteness change makes the item fork-eligible where it wasn't before, append: *"This item is now fork-eligible. Run /todo-fork to check for fork groups."*

## Guardrails

- Don't lose priority history silently. The `priority` field stays unless concreteness changes; even then, surface the score delta in the report.
- Don't allow circular deps. If adding `--dep t_005` to t_003 would create a cycle (t_005 already depends on t_003 directly or transitively), refuse the edit and surface the cycle.
- Don't edit a forked item without warning. The user might be invalidating context the fork is relying on.
- Don't change `id` or `addedAt`. Those are immutable.

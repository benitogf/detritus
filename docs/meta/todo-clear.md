---
description: Prune failed and deferred items from the active todo list. Completed items already auto-evict on /todo-done; this removes the non-active states you no longer want.
category: meta
triggers:
  - todo-clear
  - prune todos
  - drop failed
  - clean up todos
when: User wants the active list trimmed of failed/deferred items they no longer intend to act on.
related:
  - meta/todo
  - meta/todo-done
  - meta/todo-defer
---

# /todo-clear — Prune Failed / Deferred Items

_Follows `meta/todo` convention #13: main session validates input + calls TodoWrite + prints the confirmation line; the phases below describe the work the delegated sub-agent performs._

Remove items the user has given up on. There is no archive, and `done` items are already evicted by `/todo-done` (see `meta/todo` #4), so this skill exists only to prune the lingering non-active states — `failed` and, on request, `deferred`.

## Inputs

- `/todo clear` — remove all `failed` items. Default (`failed` = a fork reported it couldn't finish; clearing means "I'm not retrying it").
- `/todo clear --deferred` — also remove `deferred` items (snoozed work you've decided to drop).
- `/todo clear --before <duration>` — only remove matching items older than `<duration>` (same relative-duration parser as `/todo-defer`: `+7d`, `+1w`, etc.), keyed off `deferredUntil` for deferred items and `editedAt`/`addedAt` for failed ones.

`/todo clear` never touches `open` or `in-progress` items — those are the active working set.

## Phase 1: Read

- Read the store.
- Identify items to remove based on flags.

## Phase 2: Confirm if non-trivial

If more than 10 items would be removed in one pass, surface the count and ask via `AskUserQuestion` whether to proceed. Otherwise proceed silently.

## Phase 3: Mutate

- Capture `epoch`.
- Delete each matching item from `items` (removal — there is no archive).
- Bump `epoch`, update `updatedAt`, write atomically. The store is small, so rewrite it wholesale with `Write`/`Edit` — never shell out to a script (convention #3).

## Phase 3b: TodoWrite sync

Per `meta/todo` convention #10, call `TodoWrite` with the current open + in-progress items (the pruned items disappear). Keeps the IDE UI consistent with the JSON.

## Phase 4: Report

```
Pruned 3 failed items. Active list now has 11 open + 2 in-progress items.
```

## Guardrails

- Removal is permanent. There is no archive to restore from — to bring an item back, `/todo-add` it again (new id, fresh priority). The >10-item confirmation gate keeps a bulk prune from being a surprise.
- Don't remove `open` or `in-progress` items. `/todo-clear` only prunes `failed` (and, with `--deferred`, `deferred`). Active work is never cleared.
- `done` items never reach this skill — `/todo-done` already evicted them. If you find `done` items in a store, it's an un-migrated v1 store; the next mutation's v1→v2 migration removes them (see `meta/todo` #2).

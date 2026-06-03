---
description: Archive completed items from the active todo list. Items move from `items` to `archive`; nothing is deleted.
category: meta
triggers:
  - todo-clear
  - clear todos
  - archive completed
  - clean up todos
when: User wants the active list trimmed of done items without losing history.
related:
  - meta/todo
  - meta/todo-done
---

# /todo-clear — Archive Completed Items

_Follows `meta/todo` convention #13: main session validates input + calls TodoWrite + prints the confirmation line; the phases below describe the work the delegated sub-agent performs._

Move items with `status: done` from `items` to `archive`. Active list stays focused; history stays intact for later inspection.

## Inputs

- `/todo clear` — archive all `done` items. Default.
- `/todo clear --before <duration>` — only archive items completed more than `<duration>` ago (uses the same relative-duration parser as `/todo-defer`: `+7d`, `+1w`, etc.). Useful if you want to keep recently-completed items visible for context.
- `/todo clear --failed` — also archive items with `status: failed`. Default excludes failed items because the user usually wants them visible for retry.

## Phase 1: Read

- Read the store.
- Identify items to archive based on flags.

## Phase 2: Confirm if non-trivial

If more than 10 items would be archived in one pass, surface the count and ask via `AskUserQuestion` whether to proceed. Otherwise proceed silently.

## Phase 3: Mutate

- Capture `epoch`.
- For each item to archive: move from `items` to `archive`, preserving all fields including `completedAt`.
- Bump `epoch`, update `updatedAt`, write atomically.

## Phase 3b: TodoWrite sync

Per `meta/todo` convention #10, call `TodoWrite` with the current open + in-progress items (archived items disappear). Keeps the IDE UI consistent with the JSON.

## Phase 4: Report

```
Archived 4 done items. Active list now has 11 open + 2 in-progress items.
```

## Restoring from archive

Not supported. The `archive` array is read-only. To "restore" an archived item, use `/todo-add` with the same text — it'll get a new id and a fresh priority.

## Guardrails

- Never delete items. `archive` is a move, not a drop.
- Don't archive `in-progress` items even if their `forkSession` is null — `in-progress` means actively claimed, not done.
- Don't archive deferred items. Deferral is a temporary state; archiving would lose the deferral context.
- Confirmation gate at >10 items is a soft warning, not a hard block. A future `--yes` flag could skip the gate if the volume of completed items makes the confirmation friction repetitive.

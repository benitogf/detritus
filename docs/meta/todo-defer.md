---
description: Snooze a todo until a relative duration elapses. Accepts simple durations only (tomorrow, next-week, +Nd, +Nw); absolute-date parsing is a future enhancement.
category: meta
triggers:
  - todo-defer
  - defer todo
  - snooze
  - later
  - postpone
when: User wants to drop an item from the active view temporarily, with a clear "come back to it" time.
related:
  - meta/todo
  - meta/todo-view
---

# /todo-defer — Snooze a Todo

_Follows `meta/todo` convention #13: main session validates input + calls TodoWrite + prints the confirmation line; the phases below describe the work the delegated sub-agent performs._

Set `status: deferred` and stamp `deferredUntil` so the item drops out of `/todo-view`'s default list until the duration elapses. Deferred items are excluded from `/todo-audit` and `/todo-idle` re-rankings until they un-defer.

## Supported durations — relative only

This skill supports:

| Input | Parses to |
|---|---|
| `tomorrow` | now + 1 day |
| `next-week` | now + 7 days |
| `next-month` | now + 30 days |
| `+1d`, `+3d` | now + N days |
| `+1w`, `+2w` | now + N weeks |
| `+1m` | now + 30 days |

Anything else — absolute dates (`2026-06-15`), times (`3pm`), recurring (`every monday`) — is **not supported in this skill**. If the input doesn't match the table above, ask the user via `AskUserQuestion` to pick a supported form or use a numeric `+Nd`. A future enhancement could add absolute-date and natural-language parsing; not in scope here.

## Inputs

- `/todo defer <id> <when>` — required id, required duration.

If duration is missing, ask. If id is missing, near-match nudge.

## Phase 1: Resolve + validate

- Read the store.
- Resolve `<id>` per `/todo-done` Phase 1: accept internal id OR fuzzy substring match; positional-index disambiguation.
- If the item is `done`, `failed`, or already deferred with a future `deferredUntil`, surface the current state and ask whether to overwrite the deferral or cancel.
- If the item has `status: in-progress` with `forkSession` set, refuse the defer — *"Item is in-flight in a fork; release it from the fork first, or complete it via /todo done."*

## Phase 2: Parse the duration

Match against the supported-durations table above. Compute `deferredUntil = now + duration` in ISO format.

If the parse fails, surface the supported forms and stop.

## Phase 3: Mutate

- Set `status: deferred`, stamp `deferredUntil`.
- Re-score is **not** triggered — deferred items don't participate in ranking until they un-defer.
- Epoch-checked atomic write.

## Phase 3b: TodoWrite sync

Per `meta/todo` convention #10, call `TodoWrite` with the current open + in-progress items. The deferred item is omitted from the TodoWrite list (deferred items are excluded by the default filter in `meta/todo-view` Phase 2).

## Phase 4: Report

Per `meta/todo` convention #11 — no ids, scores, or tiers in user-facing output:

```
Deferred "<text>" until 2026-06-08 (next-week). 14 active items remain in the default view.
```

## Un-deferring

The natural un-defer: when `now >= deferredUntil`, `/todo-view` automatically includes the item again (the filter rule in `meta/todo-view` Phase 2). No explicit "un-defer" command is needed for time-based un-deferral.

For early un-defer (user wants the item back sooner), use `/todo-edit <id>` and clear `deferredUntil` via `/todo edit <id> --undefer`. Both routes write the same change.

## Guardrails

- Don't accept absolute dates. Surface the supported forms and ask.
- Don't allow deferring a forked item. Forks own their items; defer would create coordination ambiguity.
- Don't silently clamp huge durations. If the user passes `+10y`, surface "that's 10 years — confirm?" before writing.
- Don't double-defer without confirming. If an item is already deferred and the new `when` is shorter than the existing one, surface both and ask.

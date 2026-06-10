---
description: Claim a todo as in-progress inside a forked conversation. Writes the forkSession lock so the parent and other sessions know the item is in flight.
category: meta
triggers:
  - todo-claim
  - claim todo
  - I'm working on
when: User runs /todo-claim inside a forked conversation, or wants to mark an item in-progress without forking (single-session work).
related:
  - meta/todo
  - meta/todo-fork
  - meta/todo-done
---

# /todo-claim — Mark a Todo In-Progress

_Follows `meta/todo` convention #13: main session validates input + calls TodoWrite + prints the confirmation line; the phases below describe the work the delegated sub-agent performs._

Set `status: in-progress` and stamp `forkSession` so the parent conversation (and any other session viewing the file) knows the item is claimed. Most commonly run as the first command inside a forked conversation right after `/todo-fork` produced the assignment plan.

## Inputs

- `/todo claim <id>` — claim the item.
- `/todo claim <id> --release` — release an existing claim (set status back to `open`, clear `forkSession`). Useful when a fork decides it can't complete the work after all.
- `/todo claim <id> --force` — claim an item that's already in-progress under a different `forkSession`. Use only when you're sure the prior fork session is dead (e.g. its Claude Code tab was closed). Overwrites `forkSession` and `claimedAt` with the new session's values.

## Phase 1: Read + validate

- Read the store.
- Find the item.
- If the item is already `in-progress` with a different `forkSession`, refuse the claim and surface the existing lock: *"t_005 is claimed by fork-2a — cannot claim from this session."* The user can `/todo claim t_005 --release` from the original fork (or override with `--force` if it's clear the original session is dead).
- If the item is `failed`, refuse — the item is settled. (A completed item can't be claimed: `/todo-done` evicts it from the store, so it no longer exists to claim.)
- If the item is `deferred`, ask the user to confirm — claiming a deferred item un-defers it.

## Phase 2: Derive the fork session id

The `forkSession` value is a short identifier the user can recognize. Options for derivation, in priority order:

1. If the parent conversation surfaced an explicit fork id (e.g. "FORK 1 / 2" from `/todo-fork`'s output), use `fork-1` / `fork-2` / etc.
2. Otherwise, derive from the current session's identifier (a short hash of the cwd + start timestamp) — e.g. `fork-3a2b`.
3. If neither works, ask the user one concise question: *"What's a short id for this fork? (e.g. 'fork-bugfix', 'fork-docs')"*

The id is opaque to the system — it just needs to be unique within the active items and recognizable to the user.

## Phase 3: Mutate

- Capture `epoch`.
- Set `status: "in-progress"`, `forkSession: <id>`, `claimedAt: <ISO>`.
- Re-read on write, epoch-check, atomic write.

## Phase 3b: TodoWrite sync

Per `meta/todo` convention #10, call `TodoWrite` with the current open + in-progress items. The claimed item's status becomes `in_progress` in the TodoWrite call (which shows as the active asterisk marker in the IDE).

## Phase 4: Report

```
Claimed t_005 as fork-1. Parent conversation will exclude this item from /todo-audit until released or completed.
```

If invoked with `--release`:

```
Released t_005 (was fork-1). Item back to open status; available for re-ranking.
```

## Lifecycle

A claimed item leaves the fork lifecycle in one of three ways:

1. **Completed** — fork runs `/todo-done t_005`. The item is removed from the store (eviction, not a retained `done` status); its `forkSession` lock goes with it.
2. **Released** — fork runs `/todo-claim t_005 --release`. Status → `open`, `forkSession` cleared. The work isn't done; the item is back in the active pool.
3. **Failed** — fork detected it can't complete the work (e.g., scope was wrong, blocker discovered). Current path: run `/todo-claim --release` so the item returns to `open` status, and add a `failed-once` tag via `/todo-edit --tag failed-once` to capture the prior attempt. The item is back in the active pool for re-ranking. A first-class `failed` status with a `⚠ FAILED:` render in /todo-view is documented in the schema but lacks a direct user-facing setter — a future `--status` flag on `/todo-edit` or `/todo-done` would close this gap.

If the fork session terminates unexpectedly without releasing or completing, the item stays `in-progress` until the user manually intervenes — `/todo claim t_005 --release` from any session, or `/todo-done t_005` if the work actually completed.

## Guardrails

- Don't override an existing claim silently. If a different session has the lock, refuse and surface the existing `forkSession` so the user can decide.
- Don't auto-release on conflict. The conflict is intentional — the user might have multiple sessions and want to know one already claimed it.
- Don't auto-claim from the parent. `/todo-fork` outputs the plan; claiming happens inside the fork.
- Don't change priority on claim. Priority is a ranking concept; claim is a lock concept. They don't interact.
- Stamp `claimedAt` so the user can see how long an item has been in flight. If `claimedAt` is more than 7 days old and the item is still in-progress, `/todo-audit` should surface it as potentially-stale.

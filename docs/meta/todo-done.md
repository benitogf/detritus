---
description: Mark a todo as done. Sub-agent re-prioritizes survivors so the next-up item reflects what's actually next.
category: meta
triggers:
  - todo-done
  - mark done
  - finished todo
  - completed todo
when: User completes a tracked item and wants it marked done in the cross-session store.
related:
  - meta/todo
  - meta/todo-audit
  - meta/todo-clear
---

# /todo-done — Mark a Todo as Done

_Follows `meta/todo` convention #13: main session validates input + calls TodoWrite + prints the confirmation line; the phases below describe the work the delegated sub-agent performs._

Set an item's status to `done`, stamp `completedAt`, and let a Haiku sub-agent quickly re-rank the surviving open items.

## Inputs

- `/todo done <id-or-fuzzy>` — internal id OR fuzzy text match against the item's title and body.
- Free text in natural language: "I finished the janitor write hook", "done with the SIGTERM fix" — substring-match against active item titles and bodies.
- Multiple at once: `/todo done <id1> <id2>` or "done with X and Y" — extract and process each separately.

Per `meta/todo` convention #11, when fuzzy match is ambiguous list candidates by **positional index** (1., 2., 3.) — not by internal id. The user picks a number.

## Phase 1: Resolve + validate

- Read the store (Phase 0 of `meta/todo`).
- Resolve the input:
  1. If it looks like an internal id (`t_NNN`), look up by id directly.
  2. Otherwise, do a case-insensitive substring match against each active item's `title` first; if no match, also try `body`. The match must be substring-style, not whole-word.
  3. If 0 matches, print "no matching item" and stop.
  4. If 1 match, proceed.
  5. If >1 matches, list candidates by positional index (`1. <text>`, `2. <text>`, ...) — never showing internal ids — and ask the user to pick a number via `AskUserQuestion`.
- If the item is already `done`, surface a one-line note ("Already done: <text>.") and stop.
- If the item is `in-progress` with `forkSession` set, surface a one-line warning: *"This item is claimed by a fork — confirm you want to mark it done from this session and release the fork."* and ask the user to confirm. Don't reveal the fork id in the user-facing message.

## Phase 2: Mutate

- Set `status: "done"`, `completedAt: <ISO timestamp>`.
- If the item had `forkSession`, clear it (`forkSession: null`).
- Capture `epoch` before mutating; epoch-check on write per `meta/todo` convention #3.

## Phase 3: Quick re-rank (Haiku sub-agent)

Pass the surviving open items + recent conversation turns to a Haiku sub-agent. It re-scores items whose dependencies the completed item unblocked (`deps` field) and tweaks scores for items whose priority shifted because of the completion. This is a light pass, not a full audit — Sonnet's audit happens via `/todo-audit`.

The sub-agent returns the new priority for changed items only:

```jsonc
{
  "rescored": [
    { "id": "t_007", "priority": { "score": 88, "tier": "P0", "rationale": "Now unblocked by t_003 completion." } }
  ]
}
```

Apply the rescores. Re-read the file once more, epoch-check, write.

## Phase 3b: TodoWrite sync

Per `meta/todo` convention #10, call `TodoWrite` with the current open + in-progress items so the IDE's native UI reflects the completion. The just-completed item disappears from the view; rescored survivors update in place.

## Phase 4: Report

Per `meta/todo` convention #11 — no ids, no scores, no tiers in user-facing output:

```
Marked done: <title>. Re-ranked 1 survivor: "<survivor title>" moved up.
```

If no survivors were rescored, just print the first sentence ("Marked done: <text>.").

## Guardrails

- Don't mark done without resolving the input to exactly one item. Internal id and fuzzy text both resolve; on multi-match, surface candidates by positional index per `meta/todo` convention #11 and require the user to pick before mutating. Never proceed on an ambiguous match.
- Don't auto-release a forked claim silently. Always confirm with the user when the item being marked done has `forkSession` set.
- Don't move the item to `archive` on `done`. Archiving is `/todo-clear`'s job; this skill only changes status.
- Don't skip the rescore pass. Even if no items get changed, the sub-agent's check is what gives the user confidence the next-up item is current.

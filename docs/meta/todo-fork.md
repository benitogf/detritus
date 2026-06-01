---
description: Identify fork-safe groups of todos and prompt the user to launch parallel conversations via Claude Code's conversation-fork UI feature. Two gates — no codebase conflict, no knowledge gap — both must hold.
category: meta
triggers:
  - todo-fork
  - fork todos
  - parallelize todos
  - run in parallel
when: User asks whether any todos can be done in parallel, or wants to spin up parallel conversations for independent work.
related:
  - meta/todo
  - meta/todo-audit
  - meta/todo-claim
---

# /todo-fork — Identify Fork-Safe Groups, Prompt User

_Follows `meta/todo` convention #13: main session validates input + calls TodoWrite + prints the confirmation line; the phases below describe the work the delegated sub-agent performs._

A Sonnet sub-agent scans the active todo list for subsets that can be worked in parallel without conflict. If a fork-safe group exists, the chat surfaces the fork plan and asks the user to approve. On approval, the chat outputs the per-fork assignment prompts — the user then launches each fork via Claude Code's conversation-fork UI feature.

`/todo-fork` itself does **not** spawn parallel agents. It produces a fork plan; the user executes the forks through their IDE.

## Inputs

- `/todo fork` — scan all active items, report any fork-safe groups.
- `/todo fork <id> <id> [<id>...]` — explicit candidate list; the sub-agent checks whether the given items are fork-safe and reports.
- `/todo fork --max <N>` — soft cap on group size (default 3; raise/lower per pass).

## Fork eligibility rubric

A group of items is fork-safe if and only if **both gates** hold for every pair:

### Gate 1: No codebase conflict

For each pair `(A, B)` in the group:
- A and B are in **different repos** (no possible conflict), OR
- A and B are in the **same repo** with **disjoint file sets** AND **disjoint modules/packages** AND **disjoint evidence lines** AND **no cross-dependency** (neither item's `deps` array contains the other; neither's `scope.paths` touches the other's `scope.paths`; neither's `scope.evidence` cites a file:line within ±20 lines of any line cited by the other).

Same-repo same-module changes are considered conflicting even if specific files differ, because public API changes can ripple.

**Evidence sharpens the conflict check**: when items carry `scope.evidence` (file:line refs absorbed from /audit-* semantics), Gate 1 inspects the line ranges explicitly. Two items both citing `services/auth/locker.go:140-160` are conflicting even if their `scope.paths` SETS are identical (they edit overlapping line ranges). Two items citing `services/auth/locker.go:5` and `services/auth/locker.go:300` in the same file are NOT automatically conflicting if their concrete changes don't ripple — but err on the side of conflict when uncertain. The `±20 lines` window catches "they edit the same function body" without forcing same-file items to always conflict.

When `scope.evidence` is empty for either item, fall back to the file-set check on `scope.paths` only.

### Gate 2: No knowledge gap

For each item in the group:
- `scope.concreteness == "concrete"` AND
- `scope.repos` is non-empty AND
- `scope.paths` is non-empty AND
- `scope.knownBlockers` is empty AND
- The item's `title` AND `body` contain **no gap signals**:
  - Exploratory verbs: *explore, investigate, research, decide, figure out, look into, see if*
  - Placeholders: *TBD, ?, maybe, either X or Y, depends on*
  - Conditional language: *if X then Y* (where X isn't resolved)
  - References to un-tracked decisions: *after we decide on..., once we know...*

If **any** item in a candidate group fails **either** gate, the group is rejected. The sub-agent must list which item failed which gate so the user knows what to fix.

## Phase 1: Read + scan

- Read the store.
- Filter to active open items (exclude `in-progress`, `deferred`, `done`, `failed`).
- Capture `epoch`.

## Phase 2: Sonnet sub-agent identifies groups

Pass to the sub-agent:
- The active items with full scope and text.
- The fork eligibility rubric (or reference to this doc).
- Any explicit candidate list from the user.

The sub-agent returns:

```jsonc
{
  "groups": [
    {
      "ids": ["t_005", "t_008"],
      "fork_safe": true,
      "reason": "Different repos (detritus and trendboard); both concrete; no overlapping scope.",
      "assignments": [
        {
          "id": "t_005",
          "repo": "benitogf/detritus",
          "paths": ["docs/meta/janitor.md"],
          "prompt": "Work item t_005: <verbatim text>. Scope: <repo + paths>. Verification: <how to know it's done>."
        },
        {
          "id": "t_008",
          "repo": "idnerdidx/trendboard",
          "paths": ["scripts/ws_handler.gd"],
          "prompt": "Work item t_008: ..."
        }
      ]
    }
  ],
  "rejected": [
    {
      "ids": ["t_009"],
      "reason": "Gate 2 failed: item text contains 'investigate why X is slow' — exploratory verb, scope ambiguous."
    },
    {
      "ids": ["t_011", "t_012"],
      "reason": "Gate 1 failed: both touch docs/meta/gh.md (conflicting paths in same repo)."
    }
  ]
}
```

## Phase 3: Surface to user

If `groups` is empty, print "No fork-safe groups found." Then, if `rejected` is non-empty, list each rejection so the user knows what would need to change to enable forking.

If `groups` is non-empty, for each group print:

```
Group of N — fork-safe. <reason>.
  - t_005: <text> — <repo>, <paths>
  - t_008: <text> — <repo>, <paths>
```

Ask via `AskUserQuestion` whether to:
- **Approve all groups** and output assignment prompts
- **Approve a subset** (multiSelect)
- **Cancel** — no fork output

## Phase 4: Sanity cap

If the approved groups would create more than the soft cap (default 3) in-flight forks, surface a one-line note before outputting assignments:

```
That's 5 fork sessions in flight. Recommended cap is 3 for review sanity. Proceed anyway, or trim?
```

Ask the user to confirm or trim. The cap is a recommendation, not a hard block.

## Phase 5: Output the fork plan

For each approved fork, print a copy-pasteable assignment block:

```
═══════════════════════════════════════════════════════
FORK 1 / 2 — assigned to item t_005
═══════════════════════════════════════════════════════
In Claude Code, fork this conversation, then in the forked session run:

/todo claim t_005

Assignment:
<the full assignment prompt from Phase 2>

═══════════════════════════════════════════════════════
FORK 2 / 2 — assigned to item t_008
═══════════════════════════════════════════════════════
In Claude Code, fork this conversation, then in the forked session run:

/todo claim t_008

Assignment:
<the full assignment prompt from Phase 2>
```

The "fork this conversation" instruction references whatever IDE feature the user has for forking — `/todo-fork` doesn't assume a specific keybinding because that depends on the user's setup.

The forked conversation inherits its parent's history, so the fork can read its assignment from the conversation context. `/todo-claim` (run inside the fork) writes `status: in-progress` + `forkSession: <id>` to lock the item.

## Phase 6: Report

```
Fork plan output for 2 items. Parent continues with 11 remaining active items; forked items are excluded from /todo-audit re-ranking until released or completed.
```

## Guardrails

- Never spawn parallel agents from this skill. The fork is a UX action the user takes in their IDE; the skill outputs instructions, nothing more.
- Never propose a fork that fails either gate. Surfacing rejected candidates is fine; recommending a violating fork is the failure mode this skill exists to prevent.
- Never auto-claim items. `/todo-claim` runs inside the forked conversation, not the parent. The parent only outputs the plan.
- Don't ignore the sanity cap silently. >3 forks is allowed if the user confirms, but always surface the count.
- Don't fork items with `status: in-progress` — they're already claimed.
- Don't fork items with un-resolved deps. If item A depends on item B (`A.deps contains B`), A can't be forked until B is done.

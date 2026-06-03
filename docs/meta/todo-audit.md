---
description: Full re-prioritization pass over the cross-session todo list. Sonnet sub-agent re-scores every open item with rationale, detects fork-safe groups, and optionally imports /janitor hazards.
category: meta
triggers:
  - todo-audit
  - re-prioritize todos
  - what's next
  - rank my todos
  - audit todos
when: User invokes /todo-audit, asks "what's next," or the main agent detects a pivot mid-conversation and needs to re-rank.
related:
  - meta/todo
  - meta/todo-fork
  - meta/todo-idle
  - meta/janitor
---

# /todo-audit — Full Re-Prioritization Pass

_Follows `meta/todo` convention #13: main session validates input + calls TodoWrite + prints the confirmation line; the phases below describe the work the delegated sub-agent performs._

A Sonnet sub-agent reads the active todo list + current conversation context and re-scores every open item. Heavier than `/todo-done`'s rescore pass — this is the meticulous, full-fleet ranking. Also detects fork-safe groups and proactively offers a fork prompt when one exists.

## When to invoke

- Explicit user request: `/todo audit`, "re-prioritize", "what's next".
- Pivot-detection by the main agent (per `meta/todo` convention #8): when the user's new prompt introduces work, changes course, or marks something complete, the main agent invokes `/todo-audit` *before* responding.

The audit is not free (Sonnet tokens). Do not run it on every prompt — only on real pivots. The pivot rubric:

| Signal | Pivot? |
|---|---|
| "actually...", "wait, let's first...", "before that..." | Yes |
| "I'm done with X" / "X is finished" / "let's move on" | Yes |
| Clear topic shift from the prior turn (subject change) | Yes |
| New work surfaced by the user that isn't already tracked | Yes |
| Continuation of the current task (next sub-step, clarifying question) | **No** |
| User asking a question about the codebase | **No** |
| User asking a question about /todo itself (view, file, etc.) | **No** |

## Phase 1: Read + scope

- Read the store (Phase 0 of `meta/todo`).
- Filter to active items: `status` in `{open, in-progress}` AND not currently deferred.
- Include `in-progress` items with `forkSession` set so the audit sees what's claimed and excludes those from re-ranking decisions.
- Capture `epoch`.

## Phase 2: Sonnet sub-agent re-ranks

Pass to the sub-agent:
- The full active items list (id, title, body, current priority, scope, deps, tags).
- The recent conversation context (last 10–20 turns, or a summary if longer).
- The pivot signal that triggered the audit (if any) — what changed.

The sub-agent applies the priority rubric per `meta/todo-add` (urgency × impact × dependency × context-match × effort) and returns:

```jsonc
{
  "items": [
    {
      "id": "t_003",
      "priority": { "score": 78, "tier": "P1", "rationale": "Promoted because the user's last prompt directly touches this scope." }
    },
    ...
  ],
  "newItems": [
    // Items the audit thinks should be tracked but aren't yet — e.g., work the user just surfaced inline
    { "title": "...", "body": null, "priority": {...}, "scope": {...}, "source": "audit-discovery" }
  ],
  "forkGroups": [
    {
      "ids": ["t_007", "t_011"],
      "reason": "Different repos; both concrete; no overlapping files.",
      "blockedFromFork": []  // empty if all pass; otherwise list each id + the gate it failed
    }
  ]
}
```

`forkGroups` are subsets of the items that pass both fork gates (no-conflict + no-knowledge-gap). The sub-agent applies the rubric per `meta/todo-fork` → *Fork eligibility rubric*; if any item in a candidate group fails a gate, the sub-agent omits the group entirely and lists the failed item in `blockedFromFork` so the user knows why.

## Phase 3: Surface to the user

Per `meta/todo` convention #11 — no ids, scores, or tiers in user-facing output. Show only group + title + rationale.

Print, in this order:
1. **Re-ranked items** — for any item whose tier changed, one line: `Promoted: "<text>"` or `Demoted: "<text>"` followed by the rationale on the next line. Unchanged items are summarized as "12 items unchanged" rather than re-listed.
2. **Audit-discovered new items** (if any) — list each by group + title. Ask the user via `AskUserQuestion` whether to add each (one bulk question, multiSelect).
3. **Fork groups** (if any) — describe each group by its member items' texts (one line each) + the reason it's fork-safe. Ask the user via `AskUserQuestion` whether to proceed with a fork prompt. If yes, hand off to `/todo-fork` with the approved groups (which builds the assignment prompts — those DO show ids since forks need them to call /todo-claim).

If `forkGroups` is empty but the audit identified items that *would* be fork-eligible if they were concretized, surface those as a separate note **by title, never by id**: *"These items would be fork-eligible if their scope were made concrete: \"Measure deploy time and roll out\", \"Secondary rsync hardening\". Use `/todo-edit` to sharpen their scope."*

## Phase 4: Mutate

- Re-read the file. Epoch-check.
- Apply the new priorities. Add any user-approved discovered items. Bump `epoch`, update `updatedAt`, write atomically.
- Do **not** mutate items the user rejected in Phase 3's questions.

## Janitor import

If `<cwd>/.janitor/` exists, also offer to import items from any active scratchpad's State block `Hazards / Deferred` section as a separate `AskUserQuestion`:

```
Janitor has 3 deferred hazards in .janitor/whole-repo.md. Import as todos?
  - "Flaky test in subscribe_test.go" (has file:line evidence)
  - "Stale comment referencing removed flag" (has file:line evidence)
  - "Unverified migration safety" (no file evidence — would be ambiguous as a todo)
```

(Per convention #11, the user-facing prompt names only the hazard text and an evidence-quality note; the audit's internal priority inference stays internal until the imported items hit the JSON store.)

Each imported hazard becomes a todo with `source: "janitor-import"`, the hazard's title as the item's `title`, the hazard's why-it-matters / evidence-summary as the item's `body`, and the file:line evidence parsed into `scope.evidence`. Hazards without file evidence get `concreteness: ambiguous` and require `/todo-edit` before they can be forked.

Import is **always user-confirmed** — never automatic. The reverse direction — janitor's audit phase auto-writing deferred hazards into `todos.json` each tick — is tracked separately as issue #39 and will land in a follow-up PR that modifies `meta/janitor`. This PR doesn't touch `meta/janitor` at all; the import side here is self-contained.

## Phase 5: TodoWrite sync

After Phase 4 writes the new priorities, call `TodoWrite` per `meta/todo` convention #10 so the IDE view reflects the new ordering. Content per `meta/todo-view` Phase 4 mapping.

## Phase 6: Report

Single end-of-pass line:

```
Audit complete. Re-ranked 4 items (2 changed tier), added 1 discovered item, surfaced 1 fork group, imported 2 janitor hazards.
```

## Guardrails

- Don't audit on every prompt. Pivot-only.
- Don't mutate items the user didn't approve. The audit recommends; the user accepts.
- Don't re-rank items that have `forkSession` set — they're in flight in a forked conversation and their priority shouldn't change mid-flight.
- Don't import janitor hazards without user confirmation. Even if `.janitor/` exists and has hazards, the offer is opt-in.
- Don't propose fork groups that fail either gate. Surfacing rejected candidates is fine; *recommending* a fork that violates the no-conflict or no-knowledge-gap rule is the failure mode this skill exists to prevent.
- Don't skip the epoch-check on write.

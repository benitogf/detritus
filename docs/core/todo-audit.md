---
description: Prioritization and parallel work for the cross-session /todo store — the full re-prioritization audit, the deeper idle-mode pass with user feedback, fork-safe group detection with two hard gates, and the claim/release lock for forked conversations.
triggers:
  - todo-audit
  - re-prioritize todos
  - what's next
  - rank my todos
  - audit todos
  - todo-idle
  - idle review
  - between tasks
  - what should I do next
  - todo-fork
  - fork todos
  - parallelize todos
  - run in parallel
  - todo-claim
  - claim todo
  - I'm working on
when: User asks to re-prioritize ("what's next", "rank my todos"), wants a strategic between-tasks review (idle), asks whether todos can run in parallel (fork), or claims/releases an item inside a forked conversation. Also invoked by the main agent on detected pivots per flows/project/todo convention #8.
related:
  - flows/project/todo
  - core/todo-import
  - core/todo-work
  - flows/build/janitor
---

# /todo-audit — Prioritize and Parallelize

_Follows `flows/project/todo` convention #13: main session validates input + calls TodoWrite + prints the confirmation line; everything below describes the delegated sub-agent's work. All conventions in `flows/project/todo` apply._

This doc owns four operations on the cross-session store:

| Verb | What it does |
|---|---|
| **audit** | Sonnet re-scores every open item against current context; detects fork-safe groups; offers janitor-entry import |
| **idle** | Deeper audit variant: asks the user about contentious re-orderings before committing |
| **fork** | Identifies fork-safe groups under two hard gates and outputs per-fork assignment prompts |
| **claim** | Locks an item `in-progress` with a `forkSession` id inside a forked conversation; `--release` undoes it |

## Audit — full re-prioritization pass

A Sonnet sub-agent reads the active list + current conversation context and re-scores every open item. Heavier than */todo done*'s quick re-rank — this is the meticulous full-fleet ranking.

**When to invoke**: explicit request (`/todo audit`, "re-prioritize", "what's next"), or pivot-detection by the main agent (`flows/project/todo` convention #8). The audit is not free (Sonnet tokens) — pivot-only, never every prompt:

| Signal | Pivot? |
|---|---|
| "actually...", "wait, let's first...", "before that..." | Yes |
| "I'm done with X" / "let's move on" | Yes |
| Clear topic shift; new untracked work surfaced | Yes |
| Continuation of the current task; a codebase question; a question about /todo itself | **No** |

**Scope**: active items (`open` + `in-progress`, not currently deferred). Include fork-claimed items so the audit sees what's claimed — but never re-ranks them (in-flight priority must not change under the fork). Items `in-progress` for more than 7 days (`claimedAt`) are surfaced as potentially stale.

**The sub-agent receives** the full active list (id, title, body, priority, scope, deps, tags), recent conversation context (last 10–20 turns or a summary), and the triggering pivot signal. It applies the priority rubrics from `flows/project/todo` → */todo add* (bug-severity for bug-shaped items; urgency × impact for the rest) and returns:

```jsonc
{
  "items":      [ { "id": "t_003", "priority": { "score": 78, "tier": "P1", "rationale": "Promoted: the user's last prompt directly touches this scope." } } ],
  "newItems":   [ /* work the audit thinks should be tracked but isn't — source: "audit-discovery" */ ],
  "forkGroups": [ { "ids": ["t_007", "t_011"], "reason": "Different repos; both concrete; no overlapping files.", "blockedFromFork": [] } ]
}
```

`forkGroups` are subsets passing BOTH fork gates (below); a group with any failing item is omitted entirely, the failure listed in `blockedFromFork`.

**Surface to the user** (convention #11 — no ids/scores/tiers; titles + rationale only):
1. Tier changes, one line each — `Promoted: "<title>"` / `Demoted: "<title>"` + rationale; unchanged items summarized as a count.
2. Audit-discovered new items — ask via `AskUserQuestion` (one bulk multiSelect) whether to add each.
3. Fork groups — member titles + the fork-safe reason; ask whether to proceed (on yes, continue at *Fork* below for the assignment prompts). If no group passes but some items *would* be fork-eligible once concretized, note them **by title**: *"These items would be fork-eligible if their scope were made concrete: «…», «…». Use /todo edit to sharpen them."*

**Mutate**: epoch-checked write of the new priorities + user-approved discovered items only — never items the user rejected. TodoWrite sync; report:

```
Audit complete. Re-ranked 4 items (2 changed tier), added 1 discovered item, surfaced 1 fork group, imported 2 janitor entries.
```

### Janitor import

If `<cwd>/.janitor/` exists, offer (separate `AskUserQuestion`, **always user-confirmed, never automatic**) to import entries from any active scratchpad's State block `Blockers & feature-splits` section. The user-facing prompt names only each entry's text plus an evidence-quality note. Each imported entry becomes an item with `source: "janitor-import"`, the entry title as `title`, its why-it-matters as `body`, file:line evidence parsed into `scope.evidence`; entries without file evidence land as `concreteness: ambiguous` (sharpen via */todo edit* before forking).

## Idle mode — deeper pass with user feedback

A deeper audit for when the user is between tasks and has time to engage. **Explicit-only** (`/todo idle`, "I'm between tasks", "let's plan", a session start / post-milestone cadence) — never auto-invoked from pivot detection; that's the audit's job. The difference: idle **actively asks the user about contentious calls** — items where re-ranking confidence is low or two items are nearly tied and the user's preference breaks the tie. If you don't ask, you've just run an expensive audit.

The Sonnet sub-agent gets broader context than the audit (last 20–40 turns or summary, recent git/PR context, `.janitor/` blockers & feature-splits if present) and returns the re-ranking plus a `contentious` array:

```jsonc
{
  "items": [ /* new priorities */ ],
  "contentious": [ {
    "ids": ["t_005", "t_008"],
    "question": "Two items rank almost level. The ingest worker SIGTERM bug is operational reliability; the dashboard auth refresh is a user-visible feature gap. Which should be next?",
    "options": [
      { "id": "t_005", "label": "Ingest worker SIGTERM",   "implication": "Ingest worker SIGTERM moves to the top; dashboard auth stays just below." },
      { "id": "t_008", "label": "Dashboard auth refresh",  "implication": "Dashboard auth refresh moves to the top; ingest worker SIGTERM stays just below." },
      { "id": "both",  "label": "Both top — fork-eligible?", "implication": "Check fork eligibility; if both pass the gates, surface a fork plan." }
    ]
  } ],
  "forkGroups": [ /* same as audit */ ]
}
```

The `ids`/`id` fields are internal (main uses them to apply the choice). The `question`/`label`/`implication` strings are shown **verbatim** via `AskUserQuestion`, so they MUST obey convention #11: titles, never ids, never P-tiers — say "moves to the top / stays just below", not "goes P0".

**Flow**: print the re-ranked top 5 (the strategic shape) → ask each contentious question, capped at 4 per pass (`AskUserQuestion` max; defer the rest and say so) → apply ALL the user's decisions after the round completes (never mid-round), one settling sub-agent pass → epoch-checked write → TodoWrite sync → report (no ids/scores/tiers):

```
Idle pass complete. Top 3 for now:
  • Ingest worker SIGTERM bug (you chose this over Dashboard auth refresh)
  • Dashboard auth refresh (paired; possible fork)
  • Add cross-import hooks between /todo and /janitor
3 contentious items deferred to next pass.
```

Approved fork groups continue at *Fork* below.

## Fork — identify fork-safe groups, prompt the user

A Sonnet sub-agent scans active **open** items (exclude `in-progress`, `deferred`, and `failed`; there is no `done` state — completed items are evicted from the store) for subsets workable in parallel without conflict. On approval, the chat outputs per-fork assignment prompts; **the user launches each fork via Claude Code's conversation-fork UI**. This operation never spawns parallel agents itself (`flows/project/todo` convention #9).

**Inputs**: `/todo fork` (scan all); `/todo fork <id> <id> ...` (check an explicit candidate set); `--max <N>` (soft group-size cap, default 3).

### Fork eligibility rubric — two gates, both must hold

**Gate 1 — no codebase conflict.** For each pair `(A, B)` in the group: different repos (no possible conflict), OR same repo with disjoint file sets AND disjoint modules/packages AND disjoint evidence lines AND no cross-dependency (neither's `deps` contains the other; paths don't touch; no `scope.evidence` line within **±20 lines** of any line the other cites). Same-repo same-module changes conflict even when files differ — public API changes ripple.

**Evidence sharpens the check**: items both citing `services/auth/locker.go:140-160` conflict even with identical path sets (overlapping line ranges); items citing `:5` and `:300` of the same file are NOT automatically conflicting — but err toward conflict when uncertain; the ±20-line window catches "they edit the same function body" without forcing same-file items to always conflict. When either item lacks evidence, fall back to the `scope.paths` file-set check.

**Gate 2 — no knowledge gap.** Every item in the group has `concreteness == "concrete"`, non-empty `scope.repos` and `scope.paths`, empty `knownBlockers`, and `title`+`body` free of gap signals: exploratory verbs (*explore, investigate, research, decide, figure out, look into, see if*), placeholders (*TBD, ?, maybe, either X or Y, depends on*), unresolved conditionals (*if X then Y*), references to untracked decisions (*after we decide…, once we know…*).

If **any** item fails **either** gate, the group is rejected — and the rejection lists which item failed which gate so the user knows what to fix.

### Fork flow

The sub-agent returns `groups` (each with `ids`, `fork_safe`, `reason`, and per-item `assignments` — repo, paths, and a self-contained work prompt with verification) and `rejected` (ids + which gate failed and why).

- No groups → print "No fork-safe groups found." + the rejection reasons.
- Groups → print each (member titles, repo, paths, reason) and ask via `AskUserQuestion`: approve all / approve a subset (multiSelect) / cancel.
- **Sanity cap**: if approved groups would put more than `--max` (default 3) forks in flight, surface the count and ask to proceed or trim — a recommendation, not a hard block, but never silent.

For each approved fork, print a copy-pasteable assignment block — **the one sanctioned place internal ids appear** (forked sessions need the id to claim):

```
═══════════════════════════════════════════════════════
FORK 1 / 2 — assigned to item t_005
═══════════════════════════════════════════════════════
In Claude Code, fork this conversation, then in the forked session run:

/todo claim t_005

Assignment:
<the full assignment prompt>
```

The forked conversation inherits its parent's history, so it can read its assignment from context. Report: `Fork plan output for 2 items. Parent continues with 11 remaining active items; forked items are excluded from re-ranking until released or completed.`

**Fork guardrails**: never spawn agents; never propose a group that fails either gate (surfacing rejected candidates is fine — recommending a violating fork is the failure mode this operation exists to prevent); never auto-claim from the parent (claiming happens inside the fork); don't fork `in-progress` items; don't fork an item whose `deps` contain an unfinished item.

## Claim — lock an item inside a fork

Set `status: in-progress` + `forkSession` so the parent (and any other session) sees the item is in flight. Usually the first command inside a forked conversation. Also valid for single-session work the user wants marked in-progress.

**Inputs**: `/todo claim <id-or-fuzzy>`; `--release` (back to `open`, clear `forkSession` — for a fork that can't finish); `--force` (override a claim whose session is known dead; overwrites `forkSession` + `claimedAt`).

**Validate**: already `in-progress` under a different `forkSession` → refuse and surface the existing lock (the user can `--release` from the original fork or `--force` if it's dead); never override silently, never auto-release — the conflict is information. `failed` → refuse (settled; re-open via */todo edit* first). (A completed item can't be claimed — */todo done* evicts it from the store, so it no longer exists to claim.) `deferred` → confirm — claiming un-defers.

**Fork session id** (recognizable, unique among active items; opaque otherwise), in priority order: the fork plan's id (`fork-1`, `fork-2`); else a short hash of cwd + start time (`fork-3a2b`); else ask the user for a short label.

**Mutate**: `status: "in-progress"`, `forkSession`, `claimedAt` stamped. Epoch-checked write. Claim never changes priority — ranking and locking don't interact. TodoWrite sync (the item renders `in_progress`). Report by title per convention #11:

```
Claimed "<title>" as fork-1. The parent conversation excludes it from re-ranking until released or completed.
```

**Lifecycle out of a claim**: **completed** — `/todo done` (the item is removed from the store — eviction, not a retained status; the `forkSession` lock goes with it); **released** — `--release` (back to the open pool); **failed** — current path is `--release` + a `failed-once` tag via */todo edit* (a first-class `failed` setter is a future `--status` flag). A fork that dies silently leaves the item `in-progress` until someone intervenes from any session — the audit surfaces claims older than 7 days as potentially stale.

## Guardrails

- Audit on pivots only; idle is explicit-only — reactive strategic passes waste tokens and the user's attention.
- The audit/idle recommend; the user accepts. Never mutate rejected suggestions; never re-rank fork-claimed items.
- Janitor-entry import is opt-in per pass — even when `.janitor/` is full of entries.
- All four operations: epoch-checked writes, TodoWrite re-sync, convention #11 output discipline.

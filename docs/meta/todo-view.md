---
description: Display current cross-session todos by calling TodoWrite to populate Claude Code's native todo UI with the items, grouped by their `group` field. No chat-side markdown render — the TodoWrite UI is the single display surface.
category: meta
triggers:
  - todo-view
  - show todos
  - list todos
  - what are my todos
  - what's pending
when: User invokes /todo-view (or bare /todo with no args), or asks "what are my todos / what's pending / show me the list."
related:
  - meta/todo
  - meta/todo-audit
---

# /todo-view — Render Persistent Todos to Claude Code's TodoWrite UI

_Follows `meta/todo` convention #13: main session validates input + calls TodoWrite + prints the confirmation line; the phases below describe the work the delegated sub-agent performs._

Read `~/.claude/projects/<slug>/todos.json` and call **TodoWrite** with the active items. The IDE's native todo UI is the single display surface — there is no chat-side markdown render. Read-only on the JSON; the Read + filter + sort + render work runs in a Haiku sub-agent per convention #13.

This skill replaces Claude Code's in-session TodoWrite list with the cross-session items each time it's invoked. If you were tracking unrelated work in TodoWrite before, that's wiped — `/todo-view` is a "switch to persistent-todo view" action, not an additive one.

## Inputs

- No args — render all open + in-progress items (excluding deferred items whose `deferredUntil` is still in the future).
- `<id>` OR fuzzy text — render just the matching item with full detail in chat (text, group, scope, priority rationale, status, id, ... — internal fields are visible because the user explicitly asked for the record).
- `--tier P0` / `--tier P1` / etc. — filter to a single tier (internal flag; tier is hidden from output but still usable for filtering).
- `--group <name>` — filter to a single group.
- `--all` — include `deferred` and `failed` items in the TodoWrite render (normally deferred items are hidden until due).

## Phase 1: Read the store — fresh, every time

- Resolve `~/.claude/projects/<slug>/todos.json` (Phase 0 of `meta/todo`).
- **Invoke the `Read` tool to read the file freshly on every `/todo-view` invocation.** Never use the file contents that appear in earlier conversation context — those are stale the moment another session, the user, or a hook writes to the file. The convention is explicit in `meta/todo` Phase 0 under "Always re-read fresh — never use cached context."
- If the file is missing, call `TodoWrite` with an empty list (clears any stale items from the IDE) and print "No todos yet — use `/todo add <text>` to start." Stop.
- Parse JSON. If the file is corrupt, surface the error to the user and STOP — do not call TodoWrite with partial state.

## Phase 2: Filter

- Default: items satisfying EITHER of:
  - `status` is `open` or `in-progress`, AND (`deferredUntil` is null OR `deferredUntil` <= now), OR
  - `status` is `deferred` AND `deferredUntil` <= now (the snooze elapsed — surface the item again so it can be acted on; the item stays at `status: deferred` in JSON until the next mutation flips it back).
- Apply `--tier`, `--group`, `--all`, or single-id/fuzzy filter if given.
- An elapsed-defer item appearing in the render is a signal to the user: act on it now (use `/todo-done` / `/todo-edit` / `/todo-defer` again to re-snooze, etc.). The skill does NOT auto-transition status — `/todo-view` is still a pure render. Status transition happens in whichever mutation skill the user invokes next on the item (e.g., `/todo-edit --undefer` or `/todo-done`).

## Phase 3: Sort — pure priority (group is decoration, not sort key)

Sort the filtered items **globally** by priority. The `group` field is metadata for the row prefix only; it does NOT cluster items in the sort order.

The sort comparator, in strict order:

1. `priority.tier` ascending by tier rank, where `P0 < P1 < P2 < P3` (so P0 items render first).
2. Within the same tier: `priority.score` descending (higher score first).
3. Within the same tier and score: `addedAt` ascending (older item wins the tie).

Group clustering happens only **by coincidence** — items in the same group appear adjacent if and only if their priorities happen to be neighbors in the global sort. This is intentional: `/todo-audit`'s ranking is the source of truth for "what's most urgent next," and rendering should faithfully reflect that ranking. A P0 item in group A and a P2 item in group A do NOT cluster above a P1 item in group B; the P1 sits between them.

**Why not group-first sort?** Group-first would let a single P0 item drag every other item in its group above other groups' P1 items, which distorts the priority signal. The bracketed group prefix on each row already provides visual cluster-by-group recognition without needing positional clustering.

This is a strict, deterministic sort — no LLM judgment. Given the same JSON state, two invocations MUST produce the same order.

## Phase 4: TodoWrite render — `[UPPERCASE PREFIX] item text`

Call the `TodoWrite` tool with one entry per active item, in the sorted order from Phase 3. Each row is formatted as:

```
[<UPPERCASE-COMPACT-GROUP-PREFIX>] <item title>
```

Only `title` is rendered. `body` (the optional longer description) is reserved for detail-mode (Phase 5) and never appears in the TodoWrite UI. The square brackets serve as the delimiter — no middle dot, no other separator. The uppercase form emphasizes the topic visually since Claude Code's TodoWrite UI renders content as plain text (no markdown bold). Per-item mapping:

| Item field | TodoWrite field |
|---|---|
| `[<UPPERCASE-PREFIX>] <item title>` (truncated to ~100 chars if long) | `content` |
| Same format with the leading verb of the item title gerundized when easy (e.g. `[DETRITUS] Implement...` → `[DETRITUS] Implementing...`) | `activeForm` |
| `status: open` | `"pending"` |
| `status: in-progress` | `"in_progress"` |
| `status: deferred` | omitted from the call |
| `status: failed` | `"pending"` with `content` prefixed by `⚠ FAILED:` |

No id, score, tier, or full group title appears per `meta/todo` convention #11. Only the **uppercase compact prefix in brackets**.

### Compact group prefix — deterministic algorithm

For each unique group present in the active item set, derive a compact prefix from the group name. **No LLM judgment** — strict algorithm:

1. **Start at length 1**: candidate prefix = first word of the group name (split on whitespace).
2. **Collision check**: if two or more active groups produce the same candidate prefix at this length, advance ALL of them to the next length (one more word). Repeat until every active group has a UNIQUE candidate prefix.
3. **Cap**: if collision persists when the candidate equals the full group name, accept the collision (rare; means two groups have identical names, which is itself a data integrity issue).
4. **Uppercase** the final candidate for each group.
5. **Format**: each row is `[<UPPERCASE-PREFIX>] <item text>`. Ungrouped items (group is `null`) get no brackets and no prefix — just `<item text>`.

The algorithm is intentionally simple and deterministic. There is no "prefer informative words" override; the first-N-words rule wins regardless of how generic the first word looks. This makes every invocation produce the same prefixes for the same active group set.

Worked examples:

- Active groups `{"Bulk pivot performance", "Detritus skill development"}` → prefixes `BULK`, `DETRITUS` (length 1 sufficient; no collision).
- Active groups `{"Bulk pivot performance", "Bulk auth refactor"}` → length 1 produces `Bulk`, `Bulk` (collision) → advance to length 2 → `BULK PIVOT`, `BULK AUTH`.
- Active groups `{"Bulk pivot performance", "Bulk pivot auth"}` → length 1 → collision → length 2 → `Bulk pivot`, `Bulk pivot` (still collision) → length 3 → `BULK PIVOT PERFORMANCE`, `BULK PIVOT AUTH`.

### Worked example — exact expected output

Given this `todos.json` (truncated to the relevant fields; full schema in `meta/todo` convention #2):

```jsonc
{
  "version": 1,
  "epoch": 1,
  "items": [
    { "id": "t_001", "title": "Implement the v2 janitor-side write hook", "body": null, "group": "Detritus skill development", "status": "open",
      "priority": { "score": 50, "tier": "P2", "rationale": "..." }, "scope": { "evidence": [] }, "addedAt": "2026-06-01T15:00:00Z" },
    { "id": "t_002", "title": "Optimize Pivot video-pack fleet deploy (rollup goal)", "body": null, "group": "Bulk pivot performance", "status": "open",
      "priority": { "score": 40, "tier": "P2", "rationale": "..." }, "scope": { "evidence": [] }, "addedAt": "2026-06-01T11:10:00Z" },
    { "id": "t_003", "title": "Decide the Pivot-side source for directory rsync", "body": null, "group": "Bulk pivot performance", "status": "open",
      "priority": { "score": 74, "tier": "P1", "rationale": "..." }, "scope": { "evidence": [] }, "addedAt": "2026-06-02T10:00:00Z" },
    { "id": "t_004", "title": "Adjust ingest (createVideoPack) to keep the unpacked pack directory", "body": null, "group": "Bulk pivot performance", "status": "open",
      "priority": { "score": 72, "tier": "P1", "rationale": "..." }, "scope": { "evidence": [] }, "addedAt": "2026-06-02T10:00:00Z" },
    { "id": "t_005", "title": "Rewrite buildVideoPackManifest to use directory rsync", "body": null, "group": "Bulk pivot performance", "status": "open",
      "priority": { "score": 70, "tier": "P1", "rationale": "..." }, "scope": { "evidence": [] }, "addedAt": "2026-06-02T10:00:00Z" },
    { "id": "t_006", "title": "Update tests to assert the new dir-rsync shape", "body": null, "group": "Bulk pivot performance", "status": "open",
      "priority": { "score": 66, "tier": "P1", "rationale": "..." }, "scope": { "evidence": [] }, "addedAt": "2026-06-02T10:00:00Z" },
    { "id": "t_007", "title": "Verify locally with go build + go test ./pivot/...", "body": null, "group": "Bulk pivot performance", "status": "open",
      "priority": { "score": 64, "tier": "P1", "rationale": "..." }, "scope": { "evidence": [] }, "addedAt": "2026-06-02T10:00:00Z" },
    { "id": "t_008", "title": "Ship the bulk PR and redeploy the Pivot to .197", "body": null, "group": "Bulk pivot performance", "status": "open",
      "priority": { "score": 60, "tier": "P1", "rationale": "..." }, "scope": { "evidence": [] }, "addedAt": "2026-06-02T10:00:00Z" },
    { "id": "t_009", "title": "Measure before/after per-device deploy time and roll out", "body": null, "group": "Bulk pivot performance", "status": "open",
      "priority": { "score": 50, "tier": "P2", "rationale": "..." }, "scope": { "evidence": [] }, "addedAt": "2026-06-02T10:00:00Z" },
    { "id": "t_010", "title": "Secondary hardening: rsync --timeout + SSH keepalive", "body": null, "group": "Bulk pivot performance", "status": "open",
      "priority": { "score": 35, "tier": "P3", "rationale": "..." }, "scope": { "evidence": [] }, "addedAt": "2026-06-02T10:00:00Z" }
  ]
}
```

**Prefix derivation** per the deterministic algorithm above:
- Active groups: `{"Bulk pivot performance", "Detritus skill development"}`.
- Length 1 candidates: `Bulk`, `Detritus`. No collision → use these. Uppercase → `BULK`, `DETRITUS`.

**Sort** per pure-priority comparator (`tier asc by rank` → `score desc` → `addedAt asc`):

| Rank | id    | tier | score | addedAt              | group                       | prefix     |
|------|-------|------|-------|----------------------|-----------------------------|------------|
| 1    | t_003 | P1   | 74    | 2026-06-02T10:00:00Z | Bulk pivot performance      | `BULK`     |
| 2    | t_004 | P1   | 72    | 2026-06-02T10:00:00Z | Bulk pivot performance      | `BULK`     |
| 3    | t_005 | P1   | 70    | 2026-06-02T10:00:00Z | Bulk pivot performance      | `BULK`     |
| 4    | t_006 | P1   | 66    | 2026-06-02T10:00:00Z | Bulk pivot performance      | `BULK`     |
| 5    | t_007 | P1   | 64    | 2026-06-02T10:00:00Z | Bulk pivot performance      | `BULK`     |
| 6    | t_008 | P1   | 60    | 2026-06-02T10:00:00Z | Bulk pivot performance      | `BULK`     |
| 7    | t_001 | P2   | 50    | 2026-06-01T15:00:00Z | Detritus skill development  | `DETRITUS` |
| 8    | t_009 | P2   | 50    | 2026-06-02T10:00:00Z | Bulk pivot performance      | `BULK`     |
| 9    | t_002 | P2   | 40    | 2026-06-01T11:10:00Z | Bulk pivot performance      | `BULK`     |
| 10   | t_010 | P3   | 35    | 2026-06-02T10:00:00Z | Bulk pivot performance      | `BULK`     |

Note rank 7-8: both P2/50 (tie on tier and score). Tiebreaker = `addedAt asc`. `t_001` (2026-06-01) is older than `t_009` (2026-06-02), so `t_001` wins the tie and renders at rank 7.

**Exact expected TodoWrite call**:

```jsonc
[
  { content: "[BULK] Decide the Pivot-side source for directory rsync",                       activeForm: "[BULK] Deciding the Pivot-side source for directory rsync",                       status: "pending" },
  { content: "[BULK] Adjust ingest (createVideoPack) to keep the unpacked pack directory",   activeForm: "[BULK] Adjusting ingest (createVideoPack) to keep the unpacked pack directory",   status: "pending" },
  { content: "[BULK] Rewrite buildVideoPackManifest to use directory rsync",                 activeForm: "[BULK] Rewriting buildVideoPackManifest to use directory rsync",                 status: "pending" },
  { content: "[BULK] Update tests to assert the new dir-rsync shape",                        activeForm: "[BULK] Updating tests to assert the new dir-rsync shape",                        status: "pending" },
  { content: "[BULK] Verify locally with go build + go test ./pivot/...",                    activeForm: "[BULK] Verifying locally with go build + go test ./pivot/...",                    status: "pending" },
  { content: "[BULK] Ship the bulk PR and redeploy the Pivot to .197",                       activeForm: "[BULK] Shipping the bulk PR and redeploying the Pivot to .197",                   status: "pending" },
  { content: "[DETRITUS] Implement the v2 janitor-side write hook",                          activeForm: "[DETRITUS] Implementing the v2 janitor-side write hook",                          status: "pending" },
  { content: "[BULK] Measure before/after per-device deploy time and roll out",              activeForm: "[BULK] Measuring before/after per-device deploy time and rolling out",            status: "pending" },
  { content: "[BULK] Optimize Pivot video-pack fleet deploy (rollup goal)",                  activeForm: "[BULK] Tracking the overall Pivot video-pack fleet deploy optimization goal",     status: "pending" },
  { content: "[BULK] Secondary hardening: rsync --timeout + SSH keepalive",                  activeForm: "[BULK] Hardening rsync --timeout + SSH keepalive",                                status: "pending" }
]
```

Note the interleaving at ranks 7-8: `[DETRITUS]` v2 janitor sits BETWEEN two `[BULK]` items because its priority (P2/50, addedAt 2026-06-01) wins the tiebreaker over `[BULK] Measure` (P2/50, addedAt 2026-06-02). This is correct under pure-priority sort — the global priority order is what determines render position, not the group.

**Renders in the IDE as**:

```
○ [BULK] Decide the Pivot-side source for directory rsync
○ [BULK] Adjust ingest (createVideoPack) to keep the unpacked pack directory
○ [BULK] Rewrite buildVideoPackManifest to use directory rsync
○ [BULK] Update tests to assert the new dir-rsync shape
○ [BULK] Verify locally with go build + go test ./pivot/...
○ [BULK] Ship the bulk PR and redeploy the Pivot to .197
○ [DETRITUS] Implement the v2 janitor-side write hook
○ [BULK] Measure before/after per-device deploy time and roll out
○ [BULK] Optimize Pivot video-pack fleet deploy (rollup goal)
○ [BULK] Secondary hardening: rsync --timeout + SSH keepalive
```

Any `/todo` invocation against this JSON state MUST produce exactly this TodoWrite call and exactly this render. Deviation from this output for the same JSON state is a bug, not a judgment call.

## Phase 5: Single-id / fuzzy-text detail mode

If the user passed `<id>` (e.g. `t_003`) OR a fuzzy text string (e.g. `/todo view "janitor write"`), resolve to the matching item and render its **full record** in chat (not via TodoWrite — this is a debug query, not a list view). Show: id, group, title, body (if present), status, priority {score, tier, rationale}, scope {repos, paths, evidence, concreteness, knownBlockers}, deps, tags, addedAt, editedAt, claimedAt, deferredUntil, forkSession.

The `body` field is shown in this detail mode but never in the TodoWrite list view — that's the whole point of the title/body split.

The `scope.evidence` array renders as a bulleted list of `file:line` (or `file:start-end`) references; if empty, the line is omitted.

If the fuzzy text matches multiple items, list them with positional indices (`1.`, `2.`, `3.`) — not internal ids — and ask the user to pick.

## Phase 6: Report — terse, no re-list

The chat output after the TodoWrite call is a **single short confirmation line** — never a bulleted re-list of the items, never tier/score/id annotations. The user can see the items in the IDE TodoWrite UI; repeating them in chat is pure noise and violates conventions #10 (TodoWrite is the display surface) and #11 (hide internal metadata).

The confirmation line gives only:
- The item count and group count.

Example:

```
TodoWrite refreshed — 3 items now active across 2 groups.
```

If there are in-flight forks, append one additional line (no re-listing items):

```
1 item is claimed by a fork; excluded from re-ranking until done or released.
```

If no items match the filter:

```
No matching todos. (TodoWrite list cleared.)
```

**Action hints** (e.g. "use `/todo claim t_002` to start") are NOT part of the routine report. They appear only when the user is in a context where the next action is obvious and unambiguous — typically only in detail-mode (`/todo view <id-or-text>`), never in the list view.

**Forbidden in the chat report**:
- A bulleted list re-stating items already in the TodoWrite UI.
- Any item id, score, tier, or other internal field per convention #11 (except in detail-mode for a single record the user explicitly asked for).
- Parenthetical priority annotations like `(P1)` or `(P1, has the 8-step breakdown)`.
- Suggestions like "both are open and concrete/fork-eligible" — that's inferred from the items the user can already see.

## Guardrails

- Read-only on `todos.json` — no file mutations under any circumstance.
- TodoWrite content shows `[<UPPERCASE-PREFIX>] <item text>` per row (or bare text for ungrouped items). No synthetic header rows, no ids, no scores, no tiers per `meta/todo` convention #11. No markdown syntax (bold, italics, links) — Claude Code's TodoWrite UI renders content as plain text and would show the literal markers.
- Always call TodoWrite, even when the active list is empty (an empty TodoWrite call clears stale items from the IDE).
- **The compact prefix is the only place the group appears in the IDE view.** The full group title is reserved for detail-mode (`/todo view <id>`) and for `/todo view --group <full title>` filter input.
- There is no `archive` to reveal — completed items are evicted from the store on `/todo-done`, so history lives in the cited PR/issue/git, not here.
- Don't surface deferred items whose `deferredUntil` is in the future without `--all` — the whole point of defer is to drop the item from view.
- If the file's `version` doesn't match the skill's expected version (`version: 2`), stop and surface the mismatch to the user — don't render best-effort. A v1 store (has an `archive` array or `done` items) should be migrated by the next mutation per `meta/todo` #2 before a clean render.
- The TodoWrite render REPLACES whatever was previously in the IDE's todo list. Users invoking /todo-view should understand this is a "switch to persistent-todo view" action.
- Detail-mode (single-id / fuzzy-match) is the one place internal fields surface — the user explicitly asked for the record.

---
description: Cross-session todo management — router, conventions, and ALL everyday item operations (view, add, done, edit, defer, clear, file) in one doc. Prioritization + parallel work live in core/todo-audit; bulk input in core/todo-import; the TDD per-item protocol in core/todo-work.
triggers:
  - todo
  - todos
  - track this
  - remind me
  - add a todo
  - my todos
  - what are my todos
  - show todos
  - list todos
  - show todo file
  - what's pending
  - new todo
  - remember to
  - mark done
  - finished todo
  - completed todo
  - edit todo
  - change todo
  - sharpen todo
  - snooze
  - later
  - postpone
  - defer todo
  - clear todos
  - prune todos
  - drop failed
  - clean up todos
  - todo path
  - where is the todo file
when: User invokes /todo (or any todo verb — view, add, done, edit, defer, clear, file) for cross-session task tracking; the router also dispatches to /todo-audit (prioritize, idle, fork, claim), /todo-import (bulk import, ingest), and /todo-work (TDD protocol).
related:
  - core/todo-audit
  - core/todo-import
  - core/todo-work
  - flows/build/janitor
---

# /todo — Cross-Session Todos: Router, Conventions, and Item Operations

One entry point for cross-session task tracking. The family is **four docs**:

| Doc | Owns |
|---|---|
| **flows/project/todo** (this doc) | Router, cross-skill conventions, store location, and the everyday item operations: **view, add, done, edit, defer, clear, file** |
| **core/todo-audit** | Prioritization + parallel work: **audit, idle, fork, claim** |
| **core/todo-import** | Bulk input: **import** (structured) and **ingest** (free-form parsing) |
| **core/todo-work** | The opt-in **TDD per-item work protocol** |

`/todo` is **cross-session** by design. Use it for items that must survive IDE restarts, project switches, and weeks of calendar time. For ephemeral in-conversation task tracking, use the built-in `TodoWrite` tool instead — that's the right surface for "things to do in this exact session." Mixing the two surfaces is fine; be deliberate about which one each item belongs in.

## Cross-skill conventions (inherited by every todo operation)

These apply to every operation in all four docs.

1. **Single canonical store: `~/.claude/projects/<slug>/todos.json`.** `<slug>` is the project slug the memory system uses at the same location (e.g. `c--ClintonStuff-github` mirrors the cwd path). One file per project. The JSON is the source of truth; any markdown view is derived.
2. **Schema**: `{ version: 2, epoch: <int>, updatedAt: <ISO>, items: [...] }` — there is **no `archive`** array, and items carry no `completedAt`: completed items are *removed* from the store, not retained (see #4). Each item: `{ id, title, body?, group, status, priority: {score, tier, rationale}, scope: {repos, paths, evidence?, concreteness, knownBlockers}, addedAt, editedAt?, claimedAt?, deferredUntil, forkSession, deps, tags, source }`.

   **Field semantics**:
   - `title` — short, imperative, one-line summary of the work. Used as the TodoWrite content text. Required.
   - `body` — optional longer description: motivation, context, why-it-matters, design notes. Shown in detail-mode (`/todo view <id>`); NOT shown in the TodoWrite UI.
   - `group` — optional human-readable cluster title (e.g. `"Detritus skill development"`). Used as the bracketed prefix in the view render. Metadata for the row prefix and filter target, NOT a sort key — items sort globally by priority (see */todo view*). `null` means ungrouped (no bracket prefix).
   - `scope.evidence` — optional array of `{file, line, range?}` references. For bug-shaped items, this is where file:line evidence lives. The fork gates (`core/todo-audit`) use evidence to sharpen conflict detection beyond `scope.paths` alone.
   - `editedAt` (optional) — stamped by */todo edit* on first edit.
   - `claimedAt` (optional) — stamped by claim (`core/todo-audit`) when the item goes in-progress.

   The `version` field is a forward-compatibility hook — a PR that changes the schema shape bumps the version and ships its own migration logic. The schema ships at **version 2**. The v1→v2 migration runs lazily on the first mutation of an older store: drop the top-level `archive` array, remove every item with `status: done`, strip the now-unused `completedAt` field, and set `version: 2`. A v1 store is detected by the presence of an `archive` key or any `done` item.
3. **Atomic, epoch-checked writes.** Every mutating operation reads the file, captures `epoch`, computes the mutation, and writes only if `epoch` is unchanged (re-read on write; restart on mismatch — another session wrote first). Write to `todos.json.tmp` then rename. Prevents two parallel sessions clobbering each other.

   **Keep the store small; never shell out to write it.** Because completed items are removed (#2, #4), the active store stays small — typically a few dozen items at most. The delegated writer MUST mutate it with the `Write`/`Edit` tools only; it must NOT shell out to a script (`python`, `jq`, `sed`, and especially `PowerShell`) to patch the JSON. A small file is exactly why a wholesale `Write` is reliable. If the file is large enough that scripting the edit feels necessary, that is the signal the store has grown wrong (stale items never evicted) — fix that, don't bypass the tool sandbox. (The one sanctioned exception is a one-time v1→v2 migration of an oversized legacy store, where a `python` filter may be used to produce the small v2 result.)
4. **Status values**: `open`, `in-progress`, `failed`, `deferred`. Lifecycle: open → (claim) in-progress → (*/todo done*) **removed from the store**. `done` is a terminal *action*, not a persisted status — */todo done* deletes the item's row; the store never accumulates completed work (its record lives in the PR/issue/git the item cited). This keeps the store bounded to the active working set. `failed` is set when a forked session reports it couldn't complete its assignment; `deferred` is set by */todo defer* with a `deferredUntil` date. Both persist in the store and are pruned manually via */todo clear*.
5. **Priority always carries rationale.** Every item has `priority.score` (0–100), `priority.tier` (P0/P1/P2/P3), and `priority.rationale` (one sentence). Re-ranking passes surface the rationale so the user can override.
6. **Scope is persistent, not lazy.** Every item carries `scope.concreteness` (`concrete`, `ambiguous`, or `exploratory`) along with its target repos and paths. */todo add* infers and stores; the fork gates read stored scope. An item with `concreteness != "concrete"` is automatically ineligible for forking.
7. **Sub-agent model tiering.** Per convention #13, every todo operation (mutations AND reads) delegates its I/O + reasoning work to a sub-agent. Routine mutations and read-only ops (view, add, done, edit, defer, claim, clear, file) use Haiku — mechanical schema/render work. Reasoning passes (audit, idle, fork-gating, import/ingest dedup) use Sonnet — these need genuine LLM judgment.
8. **Pivot detection is the main agent's job.** When the user's new prompt introduces work, changes course, or marks something complete, the main agent invokes the audit pass (`core/todo-audit`) *before* responding. Pivot signals: "actually...", "wait, let's first...", "before that...", "change of plan", "I'm done with X, move on to Y", or a clear topic shift. Do **not** audit on every prompt — only on detected pivots.
9. **Forks are user-prompted, never automatic.** When a pass identifies a fork-safe group, it presents a fork plan via `AskUserQuestion`. The user approves, and only then does the chat output per-fork assignments for the user to launch via Claude Code's conversation-fork UI. The skill never spawns parallel agents on its own.
10. **TodoWrite re-sync is mandatory after every mutation — no exceptions.** Any operation that writes `todos.json` (add, done, edit, defer, clear, claim, import, ingest, fork, audit, idle, and any future mutation) MUST call the `TodoWrite` tool **as its final step** with the current open + in-progress items, so Claude Code's native todo UI always reflects the persistent state right after a change. The user shouldn't have to invoke the view separately to see the result of their own mutation.

    The call uses the **bracketed-uppercase-prefix rendering** defined under */todo view* below: each row is `[<UPPERCASE-PREFIX>] <item title>` (ungrouped items get bare text). No synthetic header rows. The view also calls TodoWrite on read — **and TodoWrite is the only display surface**; operations do not also emit a chat-side markdown render of the list.

    Each mutation's chat report ends with a one-line confirmation that the refresh happened, e.g. `TodoWrite refreshed — 4 items now active across 2 groups.` If the report doesn't say it, the sync is presumed skipped (a bug to fix).

11. **Hide internal metadata from user-facing output.** The TodoWrite UI and all reports show only the human-readable `group` name (as the bracketed prefix) and `title`. Optional `body` content is reserved for detail-mode (`/todo view <id>`). The following are **internal-only** and must NOT appear in TodoWrite content, confirmation messages, or any default chat output:
    - Item ids (`t_001`, `t_042`, etc.)
    - Priority scores (the numeric `priority.score`)
    - P-tiers (`P0`, `P1`, etc.)
    - Epoch, addedAt, editedAt, claimedAt timestamps

    These fields remain in the JSON for sorting and stability, but they're accounting, not UX. **Action verbs (done, edit, defer, claim) accept either an internal id (for forks and skill-to-skill handoffs) OR a fuzzy text match** — `/todo done "janitor write hook"` substring-matches the item text. On ambiguity, surface candidates **by positional index** (`1.`, `2.`, `3.`) — never by internal id — and ask the user to pick. The internal id is only ever shown when a debug flag (`--show-ids`, a future enhancement) is passed, or in the fork-assignment block (`core/todo-audit`), where forked sessions need the id to claim.

    `/todo view <id>` (single-item detail render) is the one exception — the user explicitly asked for the record, so the full record is fine to show.

    **This applies to free-form narration too, not just the formatted confirmation line.** The most common leak is a prose *summary* of list state — "what got closed", "what's blocked", "what's next" — that names items by their internal id. Ids are accounting; the user thinks in titles. Describe items by **title** (and group), give **counts** for the rest, and reference real external things (PR/issue numbers) only when they're actually actionable.

    Worked anti-pattern (the failure to avoid):

    > ❌ "1 open item remaining — the rest of the group is closed (t_013 quick win done, t_002–t_010 dropped since the -z removal hit ~110 MB/s). The only open work is the v2 janitor write hook, blocked until t_011 lands."

    Every `t_NNN` is internal noise. Rewrite using titles + counts:

    > ✅ "1 open item left: **Implement the janitor-side write hook** — blocked until the /todo skill PR merges. Everything else in the group is done or dropped."

12. **Decompositions produce individual items — never prose buried inside one item.** When any operation or in-conversation discussion produces N actionable sub-tasks for a single item ("break down", "expand", "split into steps"), those sub-tasks MUST be persisted as N separate items via `/todo-import`, NOT as a `steps` array or numbered list inside the parent's `title`/`body` or any custom field. Prose-inside-one-item hides work from the view, breaks priority ranking across the sub-tasks, and makes completion untrackable per step.

    - Sub-tasks land in the same `group` as the parent, each with its own `priority` (typically decreasing so step 1 sorts first), its own `scope`, `deps` chaining when ordering matters, and a `"sub-task"` tag.
    - The parent stays as a rollup for context UNLESS the user opts to drop it; its priority should sort below its own sub-tasks.
    - **Forbidden**: a custom `steps`, `breakdown`, `subtasks`, or similar array on a single record. Any such field in `todos.json` is a schema violation to be migrated to individual items on the next audit pass.
    - In-conversation decomposition (no explicit edit invocation): offer via `AskUserQuestion` to persist the plan as sub-tasks via `/todo-import` — don't just write prose and assume the work is captured.

13. **Main session orchestrates; a sub-agent does the work.** Every todo operation (mutations AND reads) follows the same four-step delegation pattern. The main session never reads or writes `todos.json` directly — that I/O, along with all reasoning passes (scope/priority/group inference, re-rank, fork-gate analysis, dedup, render, sort), runs in a delegated sub-agent. This keeps mechanical JSON manipulation out of the user's main conversation context.

    **The four steps**:
    1. **Validate** (main): parse the user's input, resolve `<id-or-text>` to a candidate item if needed, surface ambiguity questions via `AskUserQuestion`, refuse if malformed. No file I/O at this step.
    2. **Delegate** (main → sub-agent): spawn a Haiku or Sonnet sub-agent (per convention #7) with a structured mutation/read spec via the `Agent` tool. The sub-agent reads `todos.json`, performs all reasoning, applies the mutation (if any), validates against the schema, writes with epoch check (if writing), and returns:
       ```jsonc
       {
         "todoWritePayload": [/* TodoWrite items per the view render below */],
         "confirmationLine": "TodoWrite refreshed — 4 items now active across 2 groups."
       }
       ```
       On error (validation failure, schema mismatch, epoch race after retry, missing item), the sub-agent returns `{ "error": "<one-sentence description>" }`.
    3. **Render** (main): call `TodoWrite` with the returned payload. The TodoWrite call **must** live in the main session — a sub-agent's TodoWrite affects only its own context, not the parent's IDE-visible widget.
    4. **Report** (main): print the returned `confirmationLine` verbatim. If the sub-agent returned an error, print it verbatim and stop — no fallback, no partial state.

    The per-verb sections below describe the SUB-AGENT'S work. The main session's job is always the same three steps — validate, call Agent, call TodoWrite + print.

    ### Hard enforcement (auto-installed PreToolUse guard)

    Convention #13 is otherwise only a **behavioral contract**: `todos.json` is an ordinary file, and the main agent following these instructions is all that keeps it from writing the store directly. detritus hardens the contract with a `PreToolUse` guard: when this build ships `/todo`, `detritus --setup` (chained by `--update`) installs the hook for Claude Code, scoped to `Edit|Write|MultiEdit`, whose command is the detritus binary itself — `detritus --todo-guard`. The install is idempotent (re-running `--setup` updates the entry in place; a build that drops `/todo` removes it) and touches no other hook.

    **Scope of the guard.** It covers the realistic accidental-drift vector — the agent reaching for `Edit`/`Write`/`MultiEdit` on the store — not *every* possible write. A `Bash` write (`tee`, redirect, `sed -i`) is **not** intercepted; matching all of `Bash` would be fragile and is intentionally out of scope. The guard makes the convention hard to break *by accident*; it is not a sandbox.

    The detection keys on `agent_id` — Claude Code's canonical sub-agent discriminator, present **only** when the hook fires inside a delegated sub-agent and absent for a main session even when launched with `--agent`. The guard deliberately does **not** trust `agent_type` (a weaker signal that could be set by other means). Rule: target path is a `.claude/**/todos.json` store AND the call has no `agent_id` → **deny**; same path with an `agent_id` (the delegated sub-agent — the only legitimate writer) → **allow**. The hook never fires for a human editing the file in their own editor (out-of-band hand-edits stay possible by design; */todo file* prints the path for exactly that). The guard **fails open** on any parse/IO error, so it can never wedge unrelated edits.

    Installed `settings.json` shape:

    ```jsonc
    "hooks": {
      "PreToolUse": [
        { "matcher": "Edit|Write|MultiEdit",
          "hooks": [ { "type": "command", "command": "\"<detritus-binary>\" --todo-guard" } ] }
      ]
    }
    ```

    This is **not** `setup-extra-rules`' job — the `/todo` write-guard ships and installs as part of the `/todo` skill family itself, via `detritus --setup`.

## Store location and freshness

- Derive the project slug from cwd (same logic the memory system uses); resolve `~/.claude/projects/<slug>/todos.json`.
- If the file doesn't exist yet, mutations lazily create it with `{ version: 2, epoch: 0, updatedAt: <ISO>, items: [] }`. Read-only ops on a missing file return "no todos yet" without creating it.
- Honor the user-scoped store over any project-local `.claude/todos.json`; if both exist, prefer user-scoped and surface a one-line note.

**Always re-read fresh — never use cached context.** Every todo invocation MUST re-read `todos.json` via the `Read` tool at the start of the operation, even if the contents appear in earlier conversation context. Another session, the user, or a hook may have written since. This applies to every read-only op, every mutation (Read fresh BEFORE the epoch check), and every sub-agent invocation under convention #13 — main never short-circuits with cached data either before or after delegating. A render-from-memory is the bug this rule exists to prevent: the user just updated the list elsewhere and sees a stale view.

## Routing

Apply the first matching rule. Verbs marked **(this doc)** are sections below; the rest hand off to the named doc.

| Input shape | Route |
|---|---|
| `/todo` alone, no args | */todo view* (this doc) |
| `/todo <id>` alone | */todo view* filtered to that id (this doc) |
| `/todo view [filter]` OR "show/list my todos", "what's pending" | */todo view* (this doc) |
| `/todo add <text>` OR "add X" / "remember to X" / "track this: X" | */todo add* (this doc) |
| `/todo done <id-or-text>` OR "finished X" / "X is done" | */todo done* (this doc) |
| `/todo edit <id> ...` OR "change X to..." / "sharpen X" | */todo edit* (this doc) |
| `/todo defer <id> <when>` OR "snooze X tomorrow" / "later X" | */todo defer* (this doc) |
| `/todo clear` OR "prune failed" / "clean up todos" | */todo clear* (this doc) |
| `/todo file` OR "where is the todo file" | */todo file* (this doc) |
| `/todo audit` OR "re-prioritize" / "what's next" / "rank my todos" | `core/todo-audit` |
| `/todo idle` OR "I'm between tasks" / "let's plan" | `core/todo-audit` → *Idle mode* |
| `/todo fork` OR "parallelize my todos" | `core/todo-audit` → *Fork* |
| `/todo claim <id>` OR (inside a fork) "I'm working on t_005" | `core/todo-audit` → *Claim* |
| `/todo import <bulk-spec>` OR "import this plan as todos" | `core/todo-import` |
| `/todo ingest` OR "parse these notes / this PR review into todos" / a paste of free-form feedback | `core/todo-import` → *Ingest* |
| `/todo work <id-or-text>` OR "work this item with TDD" | `core/todo-work` |
| Ambiguous | Ask via `AskUserQuestion` — list the most plausible 2–3 verbs |

`/todo import` is primarily invoked skill-to-skill (e.g. `/plan` hands off its settled steps); it's also user-invokable directly with a structured spec.

---

## /todo view — render to the TodoWrite UI

Read the store and call **TodoWrite** with the active items. The IDE's native todo UI is the single display surface — no chat-side markdown render. Read-only on the JSON. This **replaces** Claude Code's in-session TodoWrite list — it's a "switch to persistent-todo view" action, not additive.

**Inputs**: no args (all active items); `<id>` or fuzzy text (single-item detail mode); `--tier <Pn>` / `--group <name>` filters (internal flags); `--all` (include `deferred` and `failed` items — normally future-deferred items are hidden until due).

**Filter (default)**: include items where EITHER `status ∈ {open, in-progress}` AND (`deferredUntil` is null OR ≤ now), OR `status == deferred` AND `deferredUntil` ≤ now — an elapsed defer re-surfaces so it can be acted on (the status flips only on the next mutation; the view never mutates). If the file is missing, call `TodoWrite` with an empty list (clears stale items) and print "No todos yet — use `/todo add <text>` to start." If the file is corrupt or `version != 2`, surface the error and STOP — don't render best-effort or partial state (a v1 store — has an `archive` array or `done` items — is migrated by the next mutation per convention #2 before a clean render).

**Sort — pure priority; group is decoration, not a sort key.** Strict deterministic comparator: (1) `priority.tier` ascending by rank (`P0 < P1 < P2 < P3`); (2) within a tier, `priority.score` descending; (3) within tier+score, `addedAt` ascending (older wins). Items in the same group appear adjacent only by coincidence. Group-first sorting is rejected because a single P0 would drag its whole group above other groups' P1s, distorting the priority signal — the bracketed prefix already gives visual grouping. Given the same JSON state, two invocations MUST produce the same order — no LLM judgment.

**Render**: one TodoWrite entry per item, in sorted order:

| Item field | TodoWrite field |
|---|---|
| `[<UPPERCASE-PREFIX>] <title>` (truncate ~100 chars) | `content` |
| Same with the leading verb gerundized when easy | `activeForm` |
| `status: open` | `"pending"` |
| `status: in-progress` | `"in_progress"` |
| `status: deferred` (future) | omitted (rendered only under `--all`) |
| `status: failed` | `"pending"`, content prefixed `⚠ FAILED:` |

Only `title` renders — `body` is detail-mode-only. No markdown syntax in content (the TodoWrite UI renders plain text; literal `**` shows).

**Compact group prefix — deterministic algorithm** (no LLM judgment): for each group present in the active set, the candidate prefix is the first word of the group name; if two active groups collide at the current length, advance ALL colliding groups by one word; repeat until unique (accept a collision only if two groups have identical full names). UPPERCASE the result. Ungrouped items get no brackets and no prefix. Examples: `{"Bulk pivot performance", "Detritus skill development"}` → `BULK`, `DETRITUS`; `{"Bulk pivot performance", "Bulk auth refactor"}` → `BULK PIVOT`, `BULK AUTH`.

### Worked example — exact expected output

Given this store (truncated to relevant fields):

```jsonc
{ "version": 2, "epoch": 1, "items": [
  { "id": "t_001", "title": "Implement the v2 janitor-side write hook", "group": "Detritus skill development", "status": "open",
    "priority": { "score": 50, "tier": "P2" }, "addedAt": "2026-06-01T15:00:00Z" },
  { "id": "t_002", "title": "Optimize Pivot video-pack fleet deploy (rollup goal)", "group": "Bulk pivot performance", "status": "open",
    "priority": { "score": 40, "tier": "P2" }, "addedAt": "2026-06-01T11:10:00Z" },
  { "id": "t_003", "title": "Decide the Pivot-side source for directory rsync", "group": "Bulk pivot performance", "status": "open",
    "priority": { "score": 74, "tier": "P1" }, "addedAt": "2026-06-02T10:00:00Z" },
  { "id": "t_009", "title": "Measure before/after per-device deploy time and roll out", "group": "Bulk pivot performance", "status": "open",
    "priority": { "score": 50, "tier": "P2" }, "addedAt": "2026-06-02T10:00:00Z" },
  { "id": "t_010", "title": "Secondary hardening: rsync --timeout + SSH keepalive", "group": "Bulk pivot performance", "status": "open",
    "priority": { "score": 35, "tier": "P3" }, "addedAt": "2026-06-02T10:00:00Z" }
] }
```

Prefixes: `BULK`, `DETRITUS` (length 1, no collision). Sort: t_003 (P1/74) → then the P2/50 tie between t_001 and t_009 breaks on `addedAt` — t_001 (06-01) is older and wins → t_009 → t_002 (P2/40) → t_010 (P3/35). Exact TodoWrite call:

```jsonc
[
  { content: "[BULK] Decide the Pivot-side source for directory rsync",          activeForm: "[BULK] Deciding the Pivot-side source for directory rsync",          status: "pending" },
  { content: "[DETRITUS] Implement the v2 janitor-side write hook",              activeForm: "[DETRITUS] Implementing the v2 janitor-side write hook",            status: "pending" },
  { content: "[BULK] Measure before/after per-device deploy time and roll out", activeForm: "[BULK] Measuring before/after per-device deploy time and rolling out", status: "pending" },
  { content: "[BULK] Optimize Pivot video-pack fleet deploy (rollup goal)",     activeForm: "[BULK] Optimizing Pivot video-pack fleet deploy (rollup goal)",     status: "pending" },
  { content: "[BULK] Secondary hardening: rsync --timeout + SSH keepalive",     activeForm: "[BULK] Hardening rsync --timeout + SSH keepalive",                 status: "pending" }
]
```

Note rank 2: `[DETRITUS]` sits BETWEEN `[BULK]` rows because its priority wins the tiebreaker — correct under pure-priority sort; the global order determines render position, not the group. Any invocation against this JSON state MUST produce exactly this call. Deviation for the same state is a bug, not a judgment call.

### Detail mode (single id / fuzzy text)

Resolve to the matching item and render its **full record in chat** (not via TodoWrite — a debug query, not a list view): id, group, title, body, status, priority {score, tier, rationale}, scope {repos, paths, evidence as a `file:line` bullet list, concreteness, knownBlockers}, deps, tags, addedAt, editedAt, claimedAt, deferredUntil, forkSession. Internal fields are visible here because the user explicitly asked for the record. On multi-match, list candidates by positional index and ask. There is no archive to reveal — completed items are evicted on */todo done*; history lives in the cited PR/issue/git.

### View report

A single short confirmation line after the TodoWrite call — never a bulleted re-list, never tier/score/id annotations, no parenthetical priority notes, no action hints in list view (hints are detail-mode-only):

```
TodoWrite refreshed — 3 items now active across 2 groups.
```

Append one line if forks are in flight: `1 item is claimed by a fork; excluded from re-ranking until done or released.` If nothing matches the filter: `No matching todos. (TodoWrite list cleared.)` — and still call TodoWrite (empty) so stale rows clear.

---

## /todo add — capture one item

Capture a new item; a Haiku sub-agent infers structure. **Inputs**: `/todo add <text>` (required; ask one concise question if missing), plus optional hints: `--p0`…`--p3`, `--group "<name>"`, `--tag <t>`, `--repo <r>`, `--path <p>`.

**Inference** — the sub-agent receives the text, the last 2–3 conversation turns (for scope context), and the current item list (for relative ranking + dedup), and returns the structured item fields:

- **Title vs body**: split the input into a short imperative `title` (≤80 chars) and optional `body` (longer description). One-sentence input → title only, body null. Longer input → first sentence usually becomes the title, the rest the body. Preserve the user's content — don't paraphrase or invent.
- **Group**: pick an existing group when the item clearly belongs to one; coin a new short Title Case name only when nothing fits; honor `--group` verbatim; null when no clear cluster.
- **Evidence**: parse any literal file:line refs in the input (`services/auth/locker.go:142`, `stream.go:89-94`) into `scope.evidence` as `{file, line, range?}`. Never fabricate evidence.
- **Concreteness rubric**: `concrete` — names repo + files/sections + a specific action (evidence-bearing items are almost always concrete; fork-eligible). `ambiguous` — *what* without *where* or vice versa. `exploratory` — "explore/investigate/research/decide/figure out", placeholders ("TBD", "?", "either X or Y"); never fork-eligible until concretized.
- **Priority — two rubrics by item shape**:
  - **(A) Bug-severity heuristic** (text contains bug signals — *race, panic, leak, off-by-one, security, regression, data loss, deadlock, use-after-free, broken invariant, silent failure, corruption*): 1. correctness bugs that corrupt state / lose data / panic → P0/P1, 80–95; 2. behavior bugs (wrong output, broken error handling, silent failures) → P1, 65–79; 3. resource leaks / perf regressions → P1/P2, 55–70; 4. architectural smells with concrete consequences → P2, 40–55; 5. code-quality cleanup → P3, 20–40; 6. docs/cosmetic → P3, 5–25.
  - **(B) General urgency × impact** (everything else): score 0–100 weighted by urgency, impact, dependency (X blocks Y → boost X), context-match with the current conversation, and effort (quick wins slightly boosted). Tier: 80+ → P0, 60–79 → P1, 35–59 → P2, <35 → P3.
  - An explicit user tier hint (`--p0` etc.) is honored; the sub-agent fills score + rationale around it.

**Dedup (root-cause-aware), before writing**: (1) substring overlap on `title` + first 40 chars of `body`; (2) evidence overlap — same file:line span is a strong signal even when titles differ; (3) Sonnet escalation for semantic equivalence ("same root cause expressed differently") only when the first two passes are inconclusive AND the candidate warrants it (bug-shaped with evidence, or >100 chars). On any match, surface candidates via `AskUserQuestion` by **positional index** and ask: new item / refinement of existing (→ */todo edit*) / merge (append evidence+body to existing).

**Mutate**: new id `t_<NNN>` (max existing + 1); build the item per convention #2 with `status: "open"`, `source: "user"`, `addedAt` stamped; epoch-checked atomic write per convention #3. Then TodoWrite sync (conv #10) and a one-line report (conv #11):

```
Added under "Detritus skill development": <title>.
```

Append a hint when relevant: ambiguous/exploratory scope (*"won't be fork-eligible until /todo edit makes it concrete"*) or null group (*"Ungrouped — use /todo edit --group '<name>'"*). Don't fabricate scope to look concrete — if it can't be inferred, it's `ambiguous`.

---

## /todo done — complete an item

**Inputs**: `/todo done <id-or-fuzzy>`; natural language ("I finished the janitor write hook"); multiple at once ("done with X and Y" — process each).

**Resolve**: internal id (`t_NNN`) directly; otherwise case-insensitive substring match against active items' `title` first, then `body`. 0 matches → "no matching item", stop. >1 → candidates by positional index, user picks. Never proceed on an ambiguous match. (No "already done" guard is needed — completed items are removed from the store, so any resolved match is still active.) `in-progress` with `forkSession` set → warn (*"claimed by a fork — confirm completing from this session releases the fork"*) and require confirmation; don't reveal the fork id.

**Mutate**: **remove the item from `items`** — done is eviction, not a retained status. No `done` status is set, no `completedAt` stamped, and there is no archive to move it to; completion history lives in the PR/issue/git the item cited. A `forkSession` lock is released by the removal itself. Epoch-checked write; the post-eviction store is small, so rewrite it wholesale with `Write`/`Edit` — never shell out to a script (convention #3).

**Quick re-rank (Haiku)**: pass the surviving open items + recent turns; the sub-agent re-scores only items affected by the completion (e.g. deps the completed item unblocked) and returns `{ "rescored": [ { "id", "priority": {score, tier, rationale} } ] }`. Apply, epoch-check, write. This is a light pass — the full audit lives in `core/todo-audit`. Don't skip it even when nothing changes; the check is what makes "next up" trustworthy.

**Report** (no ids/scores/tiers): `Marked done: <title>. Re-ranked 1 survivor: "<survivor title>" moved up.` (first sentence only when nothing rescored), then the TodoWrite confirmation line.

---

## /todo edit — re-text / re-scope without losing history

Mutate `title`, `body`, `scope` (incl. evidence), `group`, `tags`, or `deps` without resetting `priority` history or `addedAt`. Common use: sharpen a vague item until it's concrete (fork-eligible).

**Flags** (combinable): `--title "<t>"`; `--body "<b>"` (`--body ""` or `--clear-body` → null); bare text with no flags → split into title+body by the same first-sentence rule as */todo add* (one sentence → title only, body cleared); `--evidence <file:line>[,...]` (append; validate `path/file.ext:NN` or `:NN-MM`) / `--no-evidence <refs>` / `--clear-evidence`; `--group "<name>"` / `--ungroup`; `--scope <repos,paths,concreteness>`; `--tag <t>` / `--untag <t>`; `--dep <id>` / `--undep <id>`; `--undefer` (clear `deferredUntil`, status back to `open`). Id alone with no changes → treat as detail view + ask what to change.

**Rules**: resolve the target per */todo done*. Editing a fork-claimed item → one-line warning + confirm (the fork's context may invalidate). A title replacement with <30% substring overlap counts as a "rewrite" — flag it in the report. If `concreteness` changes to `concrete`, run a quick Haiku rescore of this single item (concreteness affects rankability) and surface the score delta. Validate `--dep` targets exist; refuse circular deps (direct or transitive) and surface the cycle. `id` and `addedAt` are immutable. Stamp `editedAt`. Epoch-checked write; TodoWrite sync; report per conv #11, e.g.:

```
Edited: "<title>". Text changed (rewrite — low overlap). Scope concreteness: ambiguous → concrete.
```

Append when newly fork-eligible: *"This item is now fork-eligible. Run /todo fork to check for fork groups."*

**Decomposition → `/todo-import`.** If the edit request produces a multi-step breakdown (numbered list, "break into N steps"), do NOT store the breakdown inside the item (convention #12). Leave the parent's fields untouched; hand the N sub-tasks to `/todo-import` (structured shape; same `group`; `source: "decomposition"`; `"sub-task"` tags; chained `deps` when sequential; decreasing priorities so step 1 sorts first). Then ask via `AskUserQuestion` what to do with the parent: **keep as rollup** (default) / lower priority below its sub-tasks / mark done (evict via */todo done*) / drop (delete outright — abandon rather than complete). If `/todo-import` is unavailable, surface the error and stop — never fall back to writing the breakdown into the parent.

---

## /todo defer — snooze

Set `status: deferred` + `deferredUntil` so the item drops out of the default view until the duration elapses. Deferred items are excluded from audit/idle re-ranking until they un-defer.

**Relative durations only**: `tomorrow` (+1d), `next-week` (+7d), `next-month` (+30d), `+Nd`, `+Nw`, `+1m` (+30d). Absolute dates, times, and recurring schedules are NOT supported — ask the user to pick a supported form. **Inputs**: `/todo defer <id-or-fuzzy> <when>`; ask if the duration is missing.

**Rules**: `failed`, or already-deferred with a future date → surface the current state and ask overwrite-or-cancel (and if the new duration is *shorter* than the existing deferral, surface both before proceeding). There is no `done` state to check — completed items are evicted from the store. Fork-claimed → refuse (*"release it from the fork first, or complete it"*). Huge durations (`+10y`) → confirm before writing. No re-score on defer. Epoch-checked write; TodoWrite sync (the deferred item disappears from the render); report:

```
Deferred "<title>" until 2026-06-08 (next-week). 14 active items remain in the default view.
```

**Un-defer**: time-based is automatic — the view's default filter re-surfaces the item once `deferredUntil` elapses (no command needed). Early un-defer: `/todo edit <id> --undefer`.

---

## /todo clear — prune failed / deferred

Remove items the user has given up on. There is no archive, and completed items already auto-evict on */todo done*, so clear exists only to prune the lingering non-active states. **Inputs**: `/todo clear` (remove all `failed` items — a fork reported it couldn't finish; clearing means "I'm not retrying it"); `--deferred` (also remove `deferred` items — snoozed work you've decided to drop); `--before <duration>` (only matching items older than that — same duration parser as defer, keyed off `deferredUntil` for deferred items and `editedAt`/`addedAt` for failed ones).

**Rules**: never touches `open` or `in-progress` items — that's the active working set. >10 items in one pass → confirm via `AskUserQuestion` first (soft gate). Removal is permanent — to bring an item back, */todo add* it again (new id, fresh priority). `done` items never reach this operation (*/todo done* already evicted them); finding any means an un-migrated v1 store — the next mutation's v1→v2 migration removes them per convention #2. Epoch-checked wholesale `Write` (convention #3); TodoWrite sync; report:

```
Pruned 3 failed items. Active list now has 11 open + 2 in-progress items.
```

---

## /todo file — print the store path

Print the absolute path to `~/.claude/projects/<slug>/todos.json`. Read-only; never mutates; never dumps file contents inline (the user asked for the path — they can `cat` it). With `--json`, append: `` Inspect: `cat <path>` or open in your editor. Schema in flows/project/todo. `` If the file doesn't exist yet, print the path anyway with *"(file not yet created — first `/todo add` will create it)"*.

---

## Manual refresh, not auto-refresh on session start

A new Claude Code session always starts with an empty TodoWrite UI; the persistent JSON survives but the in-session widget doesn't. This is intentional — the user prompts for their todos when they want them (invoke `/todo`), matching detritus's explicit, user-invoked pattern (`/janitor`, `/smith`, `/gh`). A SessionStart auto-render hook was considered and deliberately omitted. After any mutation, convention #10 keeps the widget in sync for the rest of the session.

## Working with /janitor

When the cwd contains a `.janitor/` scratchpad, the audit pass (`core/todo-audit` → *Janitor import*) MAY read the State block's `Blockers & feature-splits` section and offer to import entries as todos — always user-confirmed, never automatic.

## Router guardrails

- Don't dispatch without a clear classification — ambiguous input → ask.
- Don't bypass confirmation gates (fork approval, dedup decisions, >10-item clears).
- Don't accumulate state across operations; each is a unit.
- Don't confuse `/todo` (cross-session, JSON-backed) with `TodoWrite` (in-session, harness-managed).
- Don't write `todos.json` outside a todo operation, and never from the main session even inside one — the delegated sub-agent is the only writer (convention #13, code-enforced by the PreToolUse guard).

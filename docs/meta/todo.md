---
description: Router for cross-session todo workflows — reads conversation context and dispatches to the right /todo-* sub-skill (view, add, done, audit, idle, edit, defer, clear, file, fork, claim, import, ingest, work).
category: meta
triggers:
  - todo
  - todos
  - track this
  - remind me
  - add a todo
  - what's next
  - my todos
when: User invokes /todo as the single entry point for any cross-session todo workflow — viewing, adding, completing, re-prioritizing, deferring, or forking parallel-safe work.
related:
  - meta/todo-view
  - meta/todo-add
  - meta/todo-done
  - meta/todo-audit
  - meta/todo-idle
  - meta/todo-edit
  - meta/todo-defer
  - meta/todo-clear
  - meta/todo-file
  - meta/todo-fork
  - meta/todo-claim
  - meta/todo-import
  - meta/todo-ingest
  - meta/todo-work
  - meta/janitor
---

# /todo — Router for Cross-Session Todo Workflows

One entry point for the `/todo-*` skill family. Reads the conversation + any arguments, picks the right sub-skill, and hands off. Sub-skills stay focused; this file is the dispatcher and the home for cross-skill conventions so they live in one place.

`/todo` is **cross-session** by design. Use it for items you want to carry across IDE restarts, project switches, and weeks of calendar time. For ephemeral in-conversation task tracking, use the built-in `TodoWrite` tool instead — that's the right surface for "things to do in this exact session." Mixing the two surfaces is fine; just be deliberate about which one each item belongs in.

## Cross-skill conventions (inherited by all /todo-* sub-skills)

These apply to every sub-skill this router dispatches to.

1. **Single canonical store: `~/.claude/projects/<slug>/todos.json`.** `<slug>` is the project slug the memory system uses at the same location (e.g. `c--ClintonStuff-github` mirrors the cwd path). One file per project. The JSON is the source of truth; any markdown view is derived.
2. **Schema**: `{ version: 1, epoch: <int>, updatedAt: <ISO>, items: [...], archive: [...] }`. Each item: `{ id, title, body?, group, status, priority: {score, tier, rationale}, scope: {repos, paths, evidence?, concreteness, knownBlockers}, addedAt, editedAt?, claimedAt?, completedAt, deferredUntil, forkSession, deps, tags, source }`.

   **Field semantics**:
   - `title` — short, imperative, one-line summary of the work. Used as the TodoWrite content text. Required.
   - `body` — optional longer description: motivation, context, why-it-matters, design notes. Shown in detail-mode (`/todo view <id>`); NOT shown in the TodoWrite UI.
   - `group` — optional human-readable cluster title (e.g. `"Detritus skill development"`). Used as the bracketed prefix in /todo-view. Metadata for the row prefix and filter target, NOT a sort key — items are sorted globally by priority per `meta/todo-view` Phase 3. `null` means ungrouped (no bracket prefix).
   - `scope.evidence` — optional array of `{file, line, range?}` references. For bug-shaped items, this is where file:line evidence lives. /todo-fork uses evidence to sharpen conflict detection beyond `scope.paths` alone.
   - `editedAt` (optional) — stamped by /todo-edit on first edit.
   - `claimedAt` (optional) — stamped by /todo-claim when the item goes in-progress.

   The `version` field is a forward-compatibility hook — if a future PR changes the schema shape, that PR bumps the version and ships its own migration logic. There is no migration in this PR; the schema ships at version 1.

   See the per-skill docs for which fields each operation touches.
3. **Atomic, epoch-checked writes.** Every mutating sub-skill reads the file, captures `epoch`, computes the mutation, and writes only if `epoch` is unchanged. Mismatched epoch → another session wrote first → re-read + retry. Prevents two parallel sessions clobbering each other.
4. **Status values**: `open`, `in-progress`, `done`, `failed`, `deferred`. Lifecycle: open → (claimed by /todo-claim) in-progress → (/todo-done) done → (/todo-clear) moved to archive. `failed` is set when a forked session reports it couldn't complete its assignment. `deferred` is set by /todo-defer with a `deferredUntil` date.
5. **Priority always carries rationale.** Every item has `priority.score` (0–100), `priority.tier` (P0/P1/P2/P3), and `priority.rationale` (one sentence). Sub-agents that re-rank surface the rationale so the user can override.
6. **Scope is persistent, not lazy.** Every item carries `scope.concreteness` (`concrete`, `ambiguous`, or `exploratory`) along with its target repos and paths. /todo-add infers and stores; /todo-fork reads stored scope to gate fork eligibility. An item with `concreteness != "concrete"` is automatically ineligible for forking.
7. **Sub-agent model tiering.** Per convention #13, every `/todo-*` skill (mutations AND reads) delegates its I/O + reasoning work to a sub-agent. The model tier varies by skill: routine mutations and read-only ops (add, done, edit, defer, claim, clear, view, file) use Haiku — these are mechanical schema/render work. Reasoning passes (audit, idle, fork-gating, import-dedup) use Sonnet — these need genuine LLM judgment.
8. **Pivot detection is the main agent's job.** When the user's new prompt introduces work, changes course, or marks something complete, the main agent invokes `/todo-audit` via the `Agent` tool *before* responding to the prompt. Pivot signals: "actually...", "wait, let's first...", "before that...", "change of plan", "I'm done with X, move on to Y", or a clear topic shift from the prior turn. Do **not** audit on every prompt — only on detected pivots.
9. **Forks are user-prompted, never automatic.** When a sub-skill (typically /todo-audit or /todo-fork) identifies a fork-safe group of items, it presents a fork plan via `AskUserQuestion`. The user approves, and only then does the chat output the per-fork assignments for the user to launch via Claude Code's conversation-fork UI. The skill never spawns parallel agents on its own.
10. **TodoWrite re-sync is mandatory after every mutation — no exceptions.** Any sub-skill that writes to `todos.json` (add, done, edit, defer, clear, claim, import, fork, audit, idle, and any future mutation skill) MUST call the `TodoWrite` tool **as its final step** with the current open + in-progress items, so Claude Code's native todo UI always reflects the persistent state right after a change. This is the "show the user the updated list whenever something is added/updated/done/removed" rule: the user shouldn't have to invoke `/todo-view` separately to see the result of their own mutation — the mutation skill itself refreshes the view.

   The call uses the **bracketed-uppercase-prefix rendering** defined in `meta/todo-view` Phase 4: each row is `[<UPPERCASE-PREFIX>] <item text>` (ungrouped items have no brackets and no prefix — just bare text). The base prefix is the shortest 1–2 word form of the group name that uniquely identifies the group among the active items, then UPPERCASE'd for visual emphasis (Claude Code's TodoWrite UI renders content as plain text, so markdown bold/italics show literally; uppercase + brackets is the working substitute). No synthetic header rows. `/todo-view` also calls TodoWrite so the UI stays current even on read-only views — **and TodoWrite is the only display surface**; sub-skills do not also emit a chat-side markdown render of the list.

   Each mutation skill's chat report must end with a one-line confirmation that the refresh happened, e.g. `TodoWrite refreshed — 4 items now active across 2 groups.` This makes the sync auditable from the chat alone; if the report doesn't say it, the sync is presumed to have been skipped (a bug to fix).

11. **Hide internal metadata from user-facing output.** The TodoWrite UI and all sub-skill reports show only the human-readable `group` name (as the bracketed prefix) and `title` (in the row). Optional `body` content is reserved for detail-mode (`/todo view <id>`). The following are **internal-only** and must NOT appear in TodoWrite content, in sub-skill confirmation messages, or in any default chat output:
    - Item ids (`t_001`, `t_042`, etc.)
    - Priority scores (the numeric `priority.score`)
    - P-tiers (`P0`, `P1`, etc.)
    - Epoch, addedAt, editedAt, claimedAt timestamps
    
    These fields remain in the JSON for sorting and stability, but they're accounting, not UX. **Sub-skill action commands (`/todo-done`, `/todo-edit`, `/todo-defer`, `/todo-claim`) accept either an internal id (for forks and skill-to-skill handoffs) OR a fuzzy text match** — when the user types `/todo done "janitor write hook"`, the skill substring-matches the item text and acts on the match. On ambiguity (multiple matches), the skill surfaces candidates **by positional index** (`1.`, `2.`, `3.`) — never by internal id — and asks the user to pick. The internal id is only ever shown when a debug flag (`--show-ids`, a future enhancement) is passed, or in the fork-assignment block produced by `/todo-fork` (where forked sessions need the id to call `/todo-claim`).
    
    `/todo-view <id>` (single-item detail render) is the one exception — the user explicitly asked for the record, so the full record is fine to show including the id, score, tier, scope, etc. That's debug-mode behavior the user opted into.

    **This applies to free-form narration too, not just the formatted confirmation line.** The most common leak is a prose *summary* of list state — "what got closed", "what's blocked", "what's next" — that names items by their internal id. Ids are accounting; the user thinks in titles. Such a summary confuses more than it informs. Describe items by **title** (and group), give **counts** for the rest, and reference real external things (PR/issue numbers) only when they're actually actionable for the user — don't pad the line with internal bookkeeping.

    Worked anti-pattern (this is the failure to avoid):

    > ❌ "1 open item remaining — the rest of the group is closed (t_013 quick win done, t_002–t_010 dropped since the -z removal hit ~110 MB/s). The only open work is the v2 janitor write hook, blocked until t_011 lands."

    Every `t_NNN` is internal noise, and "t_011 lands" is meaningless to the user. Rewrite using titles + counts:

    > ✅ "1 open item left: **Implement the janitor-side write hook** — blocked until the /todo skill PR merges. Everything else in the group is done or dropped."

    Same information, zero internal ids, no throughput trivia the user didn't ask for. If the user genuinely needs the id (e.g. to hand to a fork), they're in the `--show-ids` / fork-assignment exception path — otherwise, titles only.

12. **Decompositions produce individual items — never prose buried inside one item.** When a mutation (typically `/todo-edit` but also `/todo-audit`, `/todo-idle`, or any in-conversation discussion that asks the agent to "break down", "expand", "split into steps", or otherwise enumerate sub-tasks) produces N actionable sub-tasks for a single item, those sub-tasks MUST be persisted as N separate todo items via `/todo-import`, NOT as a `steps` array or numbered list inside the parent item's `text` or any custom field.

    Rationale: prose-inside-one-item hides work from `/todo-view`, breaks priority ranking across the sub-tasks, and makes `/todo-done` unable to track per-step progress. The user invoked `/todo edit` to update the todo state, so the breakdown is a state change — it has to land in items.

    **Skill behavior on detected decomposition**:
    - Spawn the N sub-tasks as new items in the same `group` as the parent.
    - Each sub-task carries its own `priority` (sub-agent infers; typically decreasing scores so step 1 sorts first), its own `scope` (often more specific than the parent's), and `deps` chaining when one step must follow another.
    - Tag each sub-task with `"sub-task"` so `/todo-view --tag sub-task` can filter to the actionable level.
    - The parent item stays for context (it carries the *why* / motivation / overall goal that the sub-tasks don't repeat) UNLESS the user explicitly opts to delete it. The parent's priority should be lowered to sort below its own sub-tasks (or the user can `/todo defer` it).
    - Forbidden field: items MUST NOT carry a custom `steps`, `breakdown`, `subtasks`, or similar array on a single record. Any such field that appears in `todos.json` is a schema violation introduced by an older skill version and should be migrated to individual items on the next `/todo-audit` pass.

    **In-conversation decomposition** (no explicit `/todo-edit` invocation): if the agent and user are discussing a tracked item and the discussion produces a multi-step plan, the agent should offer (via `AskUserQuestion`) to persist the plan as sub-tasks via `/todo-import`. Don't just write prose and assume the work is captured.

13. **Main session orchestrates; a sub-agent does the work.** Every `/todo-*` sub-skill (mutations AND reads) follows the same four-step delegation pattern. The main session never reads or writes `todos.json` directly — that I/O, along with all reasoning passes (scope/priority/group inference, re-rank, fork-gate analysis, dedup, render, sort), runs in a delegated sub-agent. This keeps mechanical JSON manipulation out of the user's main conversation context, regardless of how many items the store holds.

    **The four steps**:
    1. **Validate** (main): parse the user's input, resolve `<id-or-text>` to a candidate item if needed, surface ambiguity questions via `AskUserQuestion`, refuse if the input is malformed. No file I/O at this step.
    2. **Delegate** (main → sub-agent): spawn a Haiku or Sonnet sub-agent (per convention #7's tier rules) with a structured mutation/read spec via the `Agent` tool. The sub-agent reads `todos.json`, performs all reasoning, applies the mutation (if any), validates against the schema, writes the JSON with epoch check (if writing), and returns a structured payload:
       ```jsonc
       {
         "todoWritePayload": [/* the TodoWrite items array per meta/todo-view Phase 4 */],
         "confirmationLine": "TodoWrite refreshed — 4 items now active across 2 groups."
       }
       ```
       On error (validation failure, schema mismatch, epoch race after retry, missing item), the sub-agent returns `{ "error": "<one-sentence description>" }`.
    3. **Render** (main): call the `TodoWrite` tool with the returned `todoWritePayload`. The TodoWrite call **must** live in the main session — sub-agents calling TodoWrite affect only their own context, not the parent's IDE-visible widget. This is the architectural reason TodoWrite stays in main even though everything else delegates.
    4. **Report** (main): print the returned `confirmationLine` verbatim. That's the entire user-visible chat output for the operation. If the sub-agent returned an error, print it verbatim and stop — no fallback, no partial state.

    **Why this matters**: with 10+ items each carrying ~15 schema fields, every mutation today drops 1-2 KB of JSON manipulation into the main conversation as the agent reads + edits + writes. Across a session, that's a real token tax. Delegation pushes that work into a single sub-agent call per operation; main sees only `{payload, confirmationLine}` (a few hundred bytes) coming back.

    **Sub-skill phase docs** describe the SUB-AGENT'S work (what it reads, what it reasons about, what it validates). The main session's job for every sub-skill is the same three steps above — validate, call Agent, call TodoWrite + print. Sub-skill docs reference this convention rather than re-describing the orchestration shape.

    ### Hard enforcement (auto-installed PreToolUse guard)

    Convention #13 is otherwise only a **behavioral contract**: `todos.json` is an ordinary file, and the main agent following these instructions is all that keeps it from writing the store directly. An agent that drifts (or "helpfully" cuts the corner) writes the store from the main session — silently defeating the delegation model and dropping the JSON manipulation back into the user's context.

    detritus hardens the contract with a `PreToolUse` guard. When this build ships the `/todo` family, `detritus --setup` (chained by `--update`) installs the hook for Claude Code, scoped to `Edit|Write|MultiEdit`. The hook command is the detritus binary itself — `detritus --todo-guard` — so there is no external script or runtime dependency, and the guard logic is versioned with the binary. The install is idempotent: re-running `--setup` updates the entry in place (e.g. if the binary moved) and never duplicates it; a future build that drops `/todo` removes the entry instead. It lives entirely in the user's `~/.claude/settings.json` and touches no other hook.

    **Scope of the guard.** It covers the realistic accidental-drift vector — the agent reaching for `Edit`/`Write`/`MultiEdit` on the store — not *every* possible write. A `PreToolUse` matcher keys on the tool name, and a `Bash` call carries an opaque `command` string rather than a `file_path`, so a determined main-session write via `Bash` (`tee`, a redirect, `sed -i`) is **not** intercepted. Matching all of `Bash` to parse arbitrary command strings would be fragile and is intentionally out of scope. The guard makes the convention hard to break *by accident*; it is not a sandbox against deliberate circumvention.

    The detection keys on `agent_id` — Claude Code's **canonical sub-agent discriminator**. The hook docs state it is present **only** when the hook fires inside a delegated sub-agent (`Task`/`Agent`) and to "use this to distinguish subagent hook calls from main-thread calls". A main session carries no `agent_id` even when it is launched with `--agent <name>`. The guard deliberately does **not** trust `agent_type` (the agent *name*) — it is a weaker signal that could be set by other means, which would open a main-session bypass. So the rule the guard applies is:

    - target path is a `.claude/**/todos.json` store **AND** the call has **no** `agent_id` (main session) → **deny**;
    - same path **with** an `agent_id` (delegated sub-agent — the only legitimate writer) → **allow**;
    - the hook never fires for a human editing the file in their own editor, so out-of-band hand-edits stay possible by design (`/todo file` prints the path for exactly that).

    The guard **fails open** — any parse/IO error allows the call — so it can never wedge unrelated `Edit`/`Write` work. The installed `settings.json` entry has this shape:

    ```jsonc
    "hooks": {
      "PreToolUse": [
        { "matcher": "Edit|Write|MultiEdit",
          "hooks": [ { "type": "command", "command": "\"<detritus-binary>\" --todo-guard" } ] }
      ]
    }
    ```

    This is **not** `setup-extra-rules`' job — that skill manages its own `detritus-*` rule/hook catalog separately. The `/todo` write-guard ships and installs as part of the `/todo` skill family itself, via `detritus --setup`.

## Manual refresh, not auto-refresh on session start

A new Claude Code session always starts with an empty TodoWrite UI. The persistent JSON store survives sessions, but the in-session widget doesn't — it has to be populated by an explicit action. This is intentional: a user prompts for their todos when they want to see them, rather than the agent auto-loading them on every session start.

To see the persistent list, invoke `/todo` (which routes to `/todo-view` and calls TodoWrite). To mutate the list, invoke any `/todo-*` sub-skill — each one calls TodoWrite as its final step per convention #10, so the IDE view stays in sync with the JSON for the rest of the session.

A SessionStart hook that auto-renders on session open was considered and intentionally **omitted**: prompting for state matches the rest of detritus's "explicit, user-invoked" pattern (`/janitor`, `/smith`, `/gh` — all wait for user invocation).

## Inputs

- `<verb> [args]` — e.g. `/todo add "fix the trendboard SIGTERM bug"`, `/todo done t_003`, `/todo audit`.
- `<id>` alone — treated as a context query: "what's the status of t_003?". Routes to /todo-view filtered to that id.
- Nothing at all — routes to /todo-view (show current todos).
- Free-text natural language — router classifies and routes (see Phase 1).

## Phase 0: Locate the store

- Derive the project slug from cwd (same logic the memory system uses).
- Resolve `~/.claude/projects/<slug>/todos.json`. If the file doesn't exist yet, the router lazily creates it with `{ version: 1, epoch: 0, items: [], archive: [] }` on the first mutation. Read-only ops on a missing file return "no todos yet" without creating it.
- Honor `~/.claude/projects/<slug>/todos.json` over any project-local `.claude/todos.json`. If both exist, prefer the user-scoped store and surface a one-line note to the user.

### Always re-read fresh — never use cached context

**Every `/todo*` invocation MUST re-read `todos.json` via the `Read` tool at the start of the operation, even if the contents appear in the agent's earlier conversation context.** The cached context can be stale: another session may have written to the file, the user may have edited it directly, a hook may have mutated it. The only reliable view of the persistent store is what `Read` returns right now.

The agent's earlier mental model of the file is debugging context, not source of truth. A `/todo` invocation that skips the `Read` and renders from memory is the bug this rule exists to prevent — it shows the user a stale list, the user thinks "I just updated it, why didn't it refresh," and trust in the skill erodes.

This rule applies to:
- Every read-only op (`/todo`, `/todo-view`, `/todo-file`)
- Every mutation op (which already re-reads for the epoch check, but must Read fresh BEFORE the epoch check — not assume cached `epoch` is current)
- Sub-agent invocations under convention #13 (the sub-agent does the Read; main never short-circuits with cached data either before or after delegating)

## Phase 1: Classify the input

Apply the first matching rule:

| Input shape | Route to |
|---|---|
| `/todo` alone, no args | `/todo-view` |
| `/todo <id>` alone | `/todo-view` filtered to that id |
| `/todo add <text>` OR free-text "add X" / "remember to X" / "I need to X" / "track this: X" | `/todo-add` |
| `/todo done <id>` OR "finished t_001" / "completed X" / "X is done" | `/todo-done` |
| `/todo audit` OR "re-prioritize" / "what's next" / "rank my todos" | `/todo-audit` |
| `/todo idle` OR "I'm between tasks" / "take a breather" / "let's plan" | `/todo-idle` |
| `/todo edit <id> <text>` OR "change t_003 to..." / "rename t_001..." | `/todo-edit` |
| `/todo defer <id> <when>` OR "snooze t_002 tomorrow" / "later t_004" | `/todo-defer` |
| `/todo clear` OR "archive done" / "clean up completed" | `/todo-clear` |
| `/todo file` OR "where is the todo file" / "show the path" | `/todo-file` |
| `/todo fork` OR "fork these in parallel" / "parallelize my todos" | `/todo-fork` |
| `/todo claim <id>` OR (inside a forked conversation) "I'm working on t_005" | `/todo-claim` |
| `/todo import <bulk-spec>` OR "import this plan as todos" / "bulk add these items" | `/todo-import` |
| `/todo ingest` OR "ingest this PR review" / "parse these notes into todos" / a paste of free-form feedback (prose, transcript, PR-review JSON, numbered list) | `/todo-ingest` |
| `/todo work <id-or-text>` OR "work this item with TDD" / "run the per-item protocol on X" | `/todo-work` |
| Ambiguous | Ask via `AskUserQuestion` — list the most plausible 2–3 verbs |

`/todo-import` is primarily invoked skill-to-skill (e.g. `/plan` hands off its settled steps via Shape B). It's also user-invokable directly with a structured spec when the user wants to bulk-add items under a shared group — that's the `/todo-import` route in the table.

The router never executes the sub-skill's logic itself. It picks the route, then hands off.

## Phase 2: Hand off

Call the selected sub-skill with the resolved context (slug, items file path, parsed arguments, original user prompt). Do NOT re-do phases the sub-skill will re-do — let the sub-skill read the file itself.

## Phase 3: Report

After the sub-skill returns, print a one-line summary of what was done. If the sub-skill produced a fork plan or asked the user to confirm something, surface that to the user; otherwise the result is just confirmation.

## /janitor cross-import

When the cwd contains a `.janitor/` scratchpad directory, the router (and `/todo-audit` specifically) MAY read the State block's `Hazards / Deferred` section and offer to import entries as todos. Import is always user-confirmed via `AskUserQuestion` — never automatic. See `meta/todo-audit` → *Janitor import* for the mechanism.

The reverse direction (`/janitor` writing its deferred findings into `~/.claude/projects/<slug>/todos.json`) is tracked as issue #39 and will land in a follow-up PR that modifies `meta/janitor`. This PR doesn't touch `meta/janitor` — only the import side ships here, fully contained inside the `/todo` skill family.

## Guardrails

- Don't dispatch to a sub-skill without a clear classification. Ambiguous input → ask the user.
- Don't bypass a sub-skill's confirmation gates. `/todo-fork`'s user-approval step is non-negotiable.
- Don't accumulate state across sub-skill calls. Each sub-skill is a unit; the router hands off and reports, nothing more.
- Don't confuse `/todo` (cross-session, JSON-backed, this family) with `TodoWrite` (in-session, harness-managed). They serve different scopes.
- Don't write to `todos.json` outside of a /todo-* sub-skill, and never from the main session even inside one — the delegated sub-agent is the only writer (convention #13). This is code-enforced: `detritus --setup` auto-installs a PreToolUse guard (see convention #13, *Hard enforcement*) that denies main-session `Edit`/`Write` of the store outright. The schema and concurrency rules live in the sub-skills; the router is read-and-route only.

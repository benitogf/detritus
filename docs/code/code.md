---
description: Router for the /code workspace-pack workflow — user-invoked entry point for pack-backed code exploration. Dispatches to admin sub-actions (pack/list/refresh/unpack) or runs the rest of the line as a task backed by code_* MCP tools.
category: code
triggers: []
when: User explicitly invokes /code. Never auto-fire on natural-language phrases — this skill is explicit-only, matching the /gh router pattern. Triggers are deliberately empty so harnesses that fuzzy-match phrases can't grab on the string "code".
related: []
---

# /code — Workspace-pack router

One entry point for pack-backed code exploration. Packs are built-and-stored indexes of a workspace's source files; the `code_*` MCP tools (`code_list`, `code_tree`, `code_search`, `code_get`, `code_outline`, `code_pack`) operate against them.

This skill is **explicit-only**. Do not invoke it on natural-language phrases like "search the code" or "where does X happen" during normal conversation. Only route when the user typed `/code ...`.

## Grammar

The first token after `/code` selects the mode:

| First token | Mode | Behavior |
|---|---|---|
| *(empty)* | **status** | Show the pack covering the user's CWD (if any), all packs, and the sub-command list. |
| `pack` | **admin** | Create or refresh a pack. |
| `list` | **admin** | Call `code_list`. |
| `refresh <name>` | **admin** | Shell out to `detritus --refresh <name>`. |
| `unpack <name>` | **admin** | Shell out to `detritus --unpack <name>`. |
| anything else | **task** | Treat the rest of the line as a task; use `code_*` tools. |

## Admin sub-actions

### pack

- `/code pack` → `code_pack(name=<cwd-basename>, roots=[cwd])`.
- `/code pack <name>` → refresh an existing pack `<name>`, or create over CWD with that name if it doesn't exist yet.
- `/code pack <name> <root>...` → create/refresh a multi-root workspace pack. All roots must be absolute paths.

After calling `code_pack`, print the pack stats line it returned, verbatim.

### list, refresh, unpack

- `/code list` → call `code_list`, print the result.
- `/code refresh <name>` → run `detritus --refresh <name>` via Bash. Print its stdout.
- `/code unpack <name>` → run `detritus --unpack <name>` via Bash. Print its stdout.

## Status mode (`/code` alone)

1. Call `code_list` to get all packs.
2. Determine the user's CWD (via `pwd` or environment).
3. Identify the pack whose roots contain CWD, if any.
4. Print:
   - "Active pack for this CWD: `<name>`" (or "No pack covers this directory. Run `/code pack` to create one.").
   - The full pack list.
   - A reminder of the sub-command grammar.

## Task mode (the interesting one)

When the first token isn't an admin keyword, the rest of the line is a task description. The user's intent is: **"for this task, use code_* tools instead of walking the filesystem with Grep/Read/Glob."**

### Pack resolution

1. Determine CWD.
2. Call `code_list` and find packs whose `roots` contain CWD.
3. If exactly one pack matches → use it.
4. If multiple → ask the user to disambiguate.
5. If none → tell the user to run `/code pack` first.

### Exploration protocol

Follow this order, and prefer `code_*` over Grep/Read/Glob:

1. **Orient (optional):** if the task references unfamiliar territory, call `code_tree` to see the structure.
2. **Locate:** call `code_search` with the most salient keywords from the task (feature name, symbol, domain noun). Review ranked hits.
3. **Shape:** call `code_outline` on the top 1–2 candidate files to confirm they're the right place before reading full content.
4. **Read:** call `code_get` with a line range to fetch only the relevant slice. Avoid fetching whole files without range when a range will do.
5. **Act:** once the target area is identified, proceed with the actual task (edit, plan, explain) using your normal tools.

### Fallback

If `code_search` returns zero useful results, or if follow-up `code_get` / `code_outline` reveal the answer isn't in the pack (e.g. the file is gitignored and was skipped), fall back to Grep/Read. When doing so, say in your response: "Pack didn't cover this — falling back to filesystem tools."

### What counts as a task

All of these are valid task-mode invocations:

- `/code fix the rate-limiter bug in the signup flow` (bug fix — use the protocol to land on the right file, then fix).
- `/code add a Prometheus counter to the pack command` (feature — outline + read to find the insertion point).
- `/code plan a refactor that moves auth out of main.go` (planning — tree + outline + strategic gets).
- `/code where does JWT validation happen` (localization — search + get).
- `/code what are the main domain types here` (orientation — tree + outline across entry points).

### Boundaries

- Don't use `code_*` tools outside of a `/code` invocation. During normal conversation, Grep/Read/Glob remain the defaults.
- Don't make edits without the pack-backed localization step — the whole point is to land on the right spot cheaply before acting.
- Don't batch multiple unrelated tasks under one `/code` invocation. One task per call keeps the scope tight.

## Guardrails

- Never auto-fire on natural-language phrases that merely sound code-related.
- Never skip `code_list` when resolving the pack from CWD — it's the only way to find covering roots.
- Never call `code_pack` with arbitrary roots without the user explicitly listing them in `/code pack <name> <roots>`.
- If `code_*` returns a schema-mismatch error ("was built with schema X; current is Y"), instruct the user to run `/code refresh <name>`.

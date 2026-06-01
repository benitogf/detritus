---
description: Print the absolute path to the underlying todos.json. Useful for direct inspection, backup, or debugging.
category: meta
triggers:
  - todo-file
  - todo path
  - where is the todo file
  - show todo file
when: User wants to know where the JSON store lives — for backup, manual inspection, or debugging.
related:
  - meta/todo
---

# /todo-file — Print the Store Path

_Follows `meta/todo` convention #13: main session validates input + calls TodoWrite + prints the confirmation line; the phases below describe the work the delegated sub-agent performs._

Output the absolute path to `~/.claude/projects/<slug>/todos.json`. Read-only; the path-resolution + optional archive-size summary runs in a Haiku sub-agent per convention #13.

## Inputs

- `/todo file` — print the path.
- `/todo file --archive` — print the path plus a one-line summary of the archive size.
- `/todo file --json` — print the path plus a hint to `cat` it if the user wants to inspect.

## Phase 1: Resolve

- Derive the slug (Phase 0 of `meta/todo`).
- Resolve the absolute path.

## Phase 2: Report

```
/Users/clinton/.claude/projects/c--ClintonStuff-github/todos.json
```

With `--archive`:

```
/Users/clinton/.claude/projects/c--ClintonStuff-github/todos.json
Archive: 47 completed items (oldest 2026-04-12, newest 2026-05-29).
```

With `--json`:

```
/Users/clinton/.claude/projects/c--ClintonStuff-github/todos.json
Inspect: `cat <path>` or open in your editor. Schema in meta/todo.
```

## Guardrails

- Read-only. Never mutates.
- If the file doesn't exist, print the path anyway with a note: *"(file not yet created — first `/todo add` will create it)"*.
- Don't dump the file contents inline. The user asked for the path, not the content. They can `cat` it themselves.

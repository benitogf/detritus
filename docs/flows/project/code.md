---
description: Explicit entry point for code exploration backed by the zero-setup code_* tools (code_map, code_outline, code_graph) plus native Grep. No packs, no index, no setup — context is read live from the working tree.
triggers: []
when: User explicitly invokes /code. Never auto-fire on natural-language phrases — this skill is explicit-only, matching the /gh router pattern. Triggers are deliberately empty so harnesses that fuzzy-match phrases can't grab on the string "code".
related: []
---

# /code — Seamless code exploration

One explicit entry point for exploring and working on a codebase with the live, zero-setup code-context tools. There is **no pack and no stored index**: structure is computed from the Go AST on demand (with an mtime-keyed parse cache outside the repo), and text search is the agent's native Grep. Switching branches, editing files, and not-yet-initialized folders all just work — nothing to build or refresh first.

The same tools are available always-on inside `/plan`, `/vibe`, and `/smith`; `/code` is the explicit, task-scoped way to invoke them directly.

This skill is **explicit-only**. Do not invoke it on natural-language phrases like "search the code" or "where does X happen" during normal conversation — only when the user typed `/code ...`. The rest of the line is the task.

## The tools

| Tool | Use it for |
|---|---|
| `code_map(scope?, focus?, budget?)` | A ranked, token-budgeted structural overview of a project — the most-referenced files first, each with its signatures. The default starting point when orienting. `focus` biases toward named identifiers/paths. |
| `code_outline(path)` | Signature-only view of one file or a directory. Confirm a candidate is the right place before reading it in full. |
| `code_graph(symbol)` | Precise navigation: who-calls / reachable-from for a function, implementers for an interface. Heavier (loads type info); use when you need exact relationships, not a broad overview. |
| native **Grep** | Text/keyword search across files — always fresh, no index. The default for "find the string/identifier X". |

## Exploration protocol

For a task that references unfamiliar territory, prefer this order:

1. **Orient:** `code_map` (optionally `focus` on the feature/symbol) for a ranked overview of the relevant project. Pass a `scope` for a subdir or a workspace root for a cross-repo map.
2. **Locate:** native **Grep** for the salient keyword/symbol/domain noun to find candidate files.
3. **Shape:** `code_outline` on the top one or two candidates to confirm before reading.
4. **Navigate (when relationships matter):** `code_graph` for who-calls a function, what it reaches, or who implements an interface.
5. **Read:** `Read` the confirmed slice.
6. **Act:** proceed with the task (edit, plan, explain) using your normal tools.

Steps are optional and reorderable — a one-symbol localization may be just Grep + Read; a refactor plan may lean on `code_map` + `code_graph`.

## Boundaries

- **Explicit-only.** Outside a `/code` invocation, use Grep/Read/Glob as normal during conversation; the `code_*` tools stay available but aren't forced.
- **No setup step.** Never tell the user to "build" or "refresh" anything first — there is no pack. If `code_map`/`code_graph` returns nothing useful, fall back to Grep/Read and say so.
- **One task per invocation** keeps scope tight.
- `code_graph` is never auto-run by `code_map`; reach for it only when you need exact call/implementation relationships.
- **Incident hook.** A self-acknowledged mistake/doctrine violation ("you are right, I …", "I didn't follow …", "I ignored /…") or a blocker surfacing on a PR this flow authored is an incident — detect and route per `core/ego` (→ `/grow` / `/absorb`), after finishing the deliverable.

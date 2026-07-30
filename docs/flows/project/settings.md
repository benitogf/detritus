---
description: Show or change global detritus settings — the model and effort the generated reviewer and coder agents run on. Read or set per-role model/thinking; default reviewer stays claude-fable-5.
triggers:
  - settings
  - review model
  - reviewer model
  - change model
  - coder model
  - fable
  - allowance
  - usage limit
  - model settings
  - effort
  - thinking
argument-hint: "[role] [model|thinking] [value] | reset [role]"
when: User wants to see or change the global model/thinking the generated reviewer and coder agents run on.
related:
  - roles/reviewer
  - flows/github/gh-pr
  - flows/build/forge
  - core/review-rigor
---

# /settings — Global detritus agent settings

`/settings` shows or changes the **global** detritus settings: the model and thinking effort the generated reviewer and coder agents run on. There is **one setting per user/machine**, applying to every session and every project — this is **not** a per-session toggle. You always go through the MCP tools, never by hand-editing files.

## Scope — global, per user/machine

A change made here writes the durable setting once and it applies to **every** future session and project on this machine until changed again. It is not scoped to the current conversation, repo, or worktree. (Contrast the transient in-flow opus fallback in `roles/reviewer` → *In-session model-limit fallback*, which degrades a single session only.)

## Read — bare `/settings`

On a bare `/settings`, call the `settings_get` MCP tool and present a **compact table** — one row per role with columns `role · model · thinking · default-or-set` — plus any warnings the tool returns. Do not paraphrase or drop warnings.

## Set — `/settings [role] [model|thinking] [value]`

Interpret the user's natural phrasing and map it to `settings_set` calls:

- **role** — `reviewer` or `coder`.
- **field** — `model` or `thinking`.
- **value** — the model/effort token, or `reset` to restore defaults for a role (or everything).

The tool already normalizes short aliases (`opus`, `fable`, `sonnet`, `haiku`, and `session` → inherit the session model), so **pass the user's token straight through** rather than expanding it yourself. Confirm the result the tool reports and state **when it takes effect** (a reviewer/coder model change applies to the next such agent spawned; a durable model change persists across sessions).

**Never edit `settings.json` or the agent definition files directly** — every read and write goes through `settings_get` / `settings_set`. Direct edits desync the two and are overwritten.

The default reviewer model stays `claude-fable-5` at `high` thinking; `/settings reviewer model …` is the **durable** override, while the in-flow opus fallback (`roles/reviewer`) is the transient one.

## Fallback — tools unavailable

If `settings_get` / `settings_set` are not available, the installed detritus binary predates this settings surface. Tell the user to run `detritus --update` and start a new session; do not attempt to hand-edit the settings file as a workaround.

## Usage samples

```
/settings
→ reviewer: claude-fable-5 (default) · thinking high (default)
  coder:    inherit (default)        · thinking low (default)

/settings reviewer model opus
→ reviewer now reviews on claude-opus-4-8; applies to the next review in this session

/settings put reviews back on fable
→ reviewer.model = claude-fable-5

/settings reviewer model session
→ reviewer.model = inherit — reviews run on whatever model your session uses

/settings coder thinking medium
→ coder.thinking = medium

/settings reset reviewer
→ reviewer back to defaults (claude-fable-5, high)

/settings reset
→ everything back to defaults
```

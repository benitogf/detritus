---
description: Feature loop — passes the first invocation through /plan to settle scope, then drives a recurring build/audit loop until the feature is merged and stable
argument-hint: <feature description> [interval] [--platform auto|codex|claude-code|github-actions|cursor|windsurf|generic]
---

# Smith With Detritus

The user invoked this command with: $ARGUMENTS

Call the detritus MCP tool `kb_get` with `name="meta/smith"` and follow the returned guidance. The first invocation passes the feature description to `/plan` for a live scope/acceptance conversation; only after `/plan` settles does `/smith` schedule the recurring build loop and (after the resulting PR merges) transition into a scoped maintenance audit phase on the changed code.

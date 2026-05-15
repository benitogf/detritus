---
description: Process a flat audit / review / feedback list one entry at a time — prove, fix, ship via /gh, advance on "next"
argument-hint: [list-file]
---

# Handle audit list

The user invoked this command with: $ARGUMENTS

Call the detritus MCP tool `kb_get` with `name="meta/audit-handle"` and follow the returned per-item loop against the working list file ($ARGUMENTS if supplied, otherwise `review.md` by convention).

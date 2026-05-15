---
description: Normalize arbitrary feedback into review.md entries with deduping and priority sort
argument-hint: [raw-feedback-or-list-file]
---

# Add to audit list

The user invoked this command with: $ARGUMENTS

Call the detritus MCP tool `kb_get` with `name="meta/audit-add"` and follow the returned workflow to parse the raw feedback in $ARGUMENTS (and any pasted text in the surrounding conversation) into entries appended to the working list file (`review.md` by default, or a user-specified filename).

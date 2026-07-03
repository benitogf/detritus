---
description: Pre-PR security gate — predict the Aikido CI security verdict locally, offline, before pushing: compute the merge-base diff, run each mapped open-source scanner scoped to only the changed files/deps, print a per-scanner per-severity green/red matrix under Aikido threshold semantics, then fix the in-scope findings before the PR opens. Scoped to the merge-base diff, never a whole-repo scan.
---

The user invoked this command with: $ARGUMENTS

Call the detritus MCP tool `kb_get` with `name="flows/github/aikido-guard"` and follow the returned guidance.

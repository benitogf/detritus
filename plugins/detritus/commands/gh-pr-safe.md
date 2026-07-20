---
description: Hard-review a posted GitHub PR with the same truthseeker rigor as /gh-pr and auto-post an APPROVE or REQUEST_CHANGES review, but never mutate the working tree of the clone it runs from. Reads via `gh api` or read-only git and isolates any build or test in a throwaway detached worktree; use it instead of /gh-pr when reviewing from a clone you actively work in.
---

<!-- detritus-generated-command -->

The user invoked this command with: $ARGUMENTS

Call the detritus MCP tool `kb_get` with `name="flows/github/gh-pr-safe"` and follow the returned guidance.

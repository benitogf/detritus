---
name: research-first
description: Exhaust available resources (KB, source code, docs) before asking the user. Invoke to reinforce self-service research behavior.
disable-model-invocation: true
---

# /research-first — Never Ask What You Can Look Up

## Core Rule

Before asking the user how something works, exhaust all available resources:

1. **KB docs** — `kb_search` and `kb_get` cover ooo, pivot, auth, testing, and more
2. **Source code** — if the repo is in the workspace, grep and read the implementation
3. **Existing docs/comments** — check inline documentation, READMEs, godoc

Only ask the user when none of these resources can answer the question.

## Anti-Pattern

- ❌ Ask the user: "Is X true? Does Y work this way? Can you verify?"
- ✅ Search the KB, read the source, read the docs. Prove the answer yourself.

## Why This Matters

- The user maintains KB docs specifically so the agent can self-serve
- Asking the user to verify researchable questions wastes their time
- Getting it wrong because you didn't look is worse than spending an extra tool call to check

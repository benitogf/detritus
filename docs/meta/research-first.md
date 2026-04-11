---
description: General agent behavior - exhaust available resources before asking the user
category: principles
triggers:
  - uncertain about API
  - how does this work
  - is this true
  - does this work
  - can you verify
  - asking user to confirm
when: Agent is uncertain about how something works and is about to ask the user instead of researching
related:
  - meta/truthseeker
---

# Research First — Never Ask What You Can Look Up

## Core Rule

Before asking the user how something works, exhaust all available resources:

1. **KB docs** — `kb_search` and `kb_get` cover all available knowledge base topics
2. **Source code** — if the repo is in the workspace, grep and read the implementation
3. **Existing docs/comments** — check inline documentation, READMEs, godoc

Only ask the user when none of these resources can answer the question.

## Anti-Pattern

❌ Ask the user: "Is X true? Does Y work this way? Can you verify?"  
✅ Search the KB, read the source if available, read the docs. Prove the answer yourself. Report what you found — don't ask the user to do your research.

## Why This Matters

- The user maintains KB docs specifically so the agent can self-serve
- Asking the user to verify researchable questions wastes their time
- Getting it wrong because you didn't look is worse than spending an extra tool call to check

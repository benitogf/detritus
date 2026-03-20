---
description: Re-index and optimize KB docs for agent retrieval efficiency
category: meta
triggers:
  - optimize
  - re-index
  - reindex
  - improve retrieval
  - doc quality
  - agent indexing
  - detection efficiency
when: User invokes /optimize to improve how effectively the agent discovers and applies KB guidance
related:
  - grow
  - truthseeker
---

# /optimize — KB Retrieval Optimization

> ## THIS COMMAND IMPLEMENTS DIRECTLY
>
> Unlike /grow and /plan, /optimize may edit files immediately.
> It targets only the local detritus clone (docs/, templates/, main.go).
> All changes aim to improve how reliably the agent detects and applies KB guidance.

---

## Step 0: Workspace Precheck

Search workspace roots for a local detritus clone:
- Look for path containing `github.com/benitogf/detritus` with a `docs/` directory
- If found: proceed to Step 1
- If NOT found:
  - Output warning:
    ```
    ⚠️ /optimize requires a local clone of the detritus MCP knowledge base.
    
    This command audits and improves KB docs so the agent can detect and apply
    guidance more reliably during tasks. Without a local clone, no changes can
    be made.
    
    Repository: https://github.com/benitogf/detritus
    ```
  - STOP. Do not proceed.

---

## Step 1: Audit All Docs

Read every file in `docs/` and check against the optimization schema below.

### Required Frontmatter Fields
Every doc MUST have:
```yaml
---
description: [one-line, keyword-rich]
category: [core|storage|sync|auth|frontend|testing|patterns|principles|meta]
triggers: [list of keywords/phrases that should cause this doc to be consulted]
when: [one sentence: under what task conditions this doc applies]
related: [list of other doc names]
---
```

### Required Body Structure
Every doc SHOULD have near the top (after the first heading):
- **Detection cues**: keywords, API names, error messages, patterns that indicate this doc is relevant
- **Anti-patterns**: concrete examples of what NOT to do (agent reads these to self-check)
- **Canonical actions**: what the agent MUST do when this doc applies

### Optimization Targets
For each doc, check:

| Check | Pass Criteria |
|-------|--------------|
| frontmatter complete | all required fields present and non-empty |
| triggers comprehensive | includes API names, error keywords, common misspellings, related concepts |
| description keyword-dense | contains the top 3-5 terms someone would use when needing this doc |
| anti-patterns present | at least one concrete "don't do X" with example |
| detection cues near top | first 20 lines contain enough keywords for kb_search to match |
| cross-references correct | related docs exist and reference back |
| no dead references | all mentioned doc names / paths are valid |
| content agent-optimized | structured as rules/tables, not prose paragraphs; "if X then Y" format preferred |

---

## Step 2: Audit main.go kb_get Description

The `kb_get` tool description is the **primary discovery mechanism** — it's what the LLM sees in its tool list to decide whether to call `kb_get`.

Check that:
- Every doc in `docs/` has a corresponding entry in the description string
- Each entry contains the doc's top trigger keywords
- New docs added since last optimization are included
- Keywords match what actually appears in conversations (not just technical terms)

---

## Step 3: Audit templates/.windsurfrules

Check that:
- The Workflows table includes all docs that are meant to be user-invokable commands
- The Foundational Principles section references the correct doc name
- No stale references to renamed/deleted docs

---

## Step 4: Implement Improvements

For each issue found:
1. Edit the file directly (docs, main.go, or templates)
2. Keep changes minimal and focused
3. Preserve existing content — add structure, don't rewrite meaning
4. After all edits, output a summary of what changed and why

---

## Step 5: Rebuild Guidance

After making changes, remind the user:
```
Changes made to the local detritus clone. To activate:
1. Rebuild: cd <detritus_path> && go build -o detritus .
2. Install: cp detritus /usr/local/bin/detritus
3. Restart Windsurf for the MCP server to reload
```

---

## Optimization Principles

- **Agent-first**: these docs exist for the agent to read, not humans. Optimize for keyword density and rule clarity.
- **Detection over explanation**: a doc that gets found and applied beats a doc that explains beautifully but never gets consulted.
- **Triggers are the index**: the frontmatter `triggers` list and the `kb_get` description string are the two primary ways docs get discovered. They must be comprehensive.
- **Anti-patterns prevent recurrence**: every time /grow identifies a failure mode, /optimize should ensure the relevant doc's anti-patterns section covers it.
- **Minimal prose**: prefer tables, bullet lists, "if X then Y" rules. Reduce paragraphs to single-line rules where possible.

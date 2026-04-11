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
  - meta/grow
  - meta/truthseeker
---

# /optimize — KB Retrieval Optimization

> ## THIS COMMAND IMPLEMENTS DIRECTLY
>
> Unlike /grow and /plan, /optimize may edit files immediately.
> It targets only the local detritus clone (docs/).
> All changes aim to improve how reliably the agent detects and applies KB guidance.

---

## Step 0: Workspace Precheck

Search workspace roots for a local detritus clone:
- Look for path containing `github.com/benitogf/detritus` with a `docs/` directory
- If found: proceed to Step 1
- If NOT found:
  - Output warning:
    ```
    /optimize requires a local clone of the detritus MCP knowledge base.
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
category: [core|storage|testing|patterns|principles|meta|planning]
triggers: [list of keywords/phrases that should cause this doc to be consulted]
when: [one sentence: under what task conditions this doc applies]
related: [list of other doc names, using full paths like meta/truthseeker]
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
| triggers comprehensive | includes API names, error keywords, related concepts |
| description keyword-dense | contains the top 3-5 terms someone would use when needing this doc |
| anti-patterns present | at least one concrete "don't do X" with example |
| detection cues near top | first 20 lines contain enough keywords for kb_search to match |
| cross-references correct | related docs exist and reference back |
| no dead references | all mentioned doc names / paths are valid |
| content agent-optimized | structured as rules/tables, not prose paragraphs |

Note: trigger keywords are also auto-enriched at build time via TF-IDF (see `cmd/generate/main.go`). Manual triggers in frontmatter should focus on domain-specific terms that TF-IDF might miss.

---

## Step 2: Cross-Reference Integrity

Check that:
- Every `related:` entry uses a full doc path (e.g. `meta/truthseeker`, not just `truthseeker`)
- Every `related:` entry points to a doc that actually exists in `docs/`
- Related docs reference back bidirectionally where it makes sense

---

## Step 3: Implement Improvements

For each issue found:
1. Edit the file directly in `docs/`
2. Keep changes minimal and focused
3. Preserve existing content — add structure, don't rewrite meaning
4. After all edits, output a summary of what changed and why

---

## Step 4: Rebuild

After making changes:
```
cd <detritus_path>
go generate ./...
go build -o detritus .
detritus --setup
```

Or if updating a released version, commit, push, and tag a new release.

---

## Optimization Principles

- **Agent-first**: these docs exist for the agent to read, not humans. Optimize for keyword density and rule clarity.
- **Detection over explanation**: a doc that gets found and applied beats a doc that explains beautifully but never gets consulted.
- **Triggers are the index**: the frontmatter `triggers` list and the auto-generated TF-IDF keywords are the primary ways docs get discovered. Manual triggers should cover domain terms that content analysis misses.
- **Anti-patterns prevent recurrence**: every time /grow identifies a failure mode, /optimize should ensure the relevant doc's anti-patterns section covers it.
- **Minimal prose**: prefer tables, bullet lists, "if X then Y" rules. Reduce paragraphs to single-line rules where possible.

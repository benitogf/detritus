---
name: optimize
description: Re-index and optimize KB docs for agent retrieval efficiency. Audits frontmatter, triggers, descriptions, cross-references. This command IMPLEMENTS directly.
---

# /optimize — KB Retrieval Optimization

> Unlike /grow and /plan, /optimize may edit files immediately.
> Targets only the local detritus clone (docs/, templates/).

## Step 0: Workspace Precheck

Look for a local detritus clone. If not found, warn and stop.

## Step 1: Audit All Docs

Check every file in `docs/` against:

### Required Frontmatter
```yaml
---
description: [one-line, keyword-rich]
category: [core|storage|testing|patterns|principles|meta|planning|scaffold]
triggers: [keywords that should cause this doc to be consulted]
when: [one sentence: task conditions where this applies]
related: [list of related doc names]
---
```

### Optimization Targets

| Check | Pass Criteria |
|-------|--------------|
| frontmatter complete | all required fields present |
| triggers comprehensive | includes API names, error keywords, related concepts |
| description keyword-dense | top 3-5 search terms |
| anti-patterns present | at least one "don't do X" example |
| cross-references correct | related docs exist and reference back |

For the full workflow, call `kb_get(name="meta/optimize")` if the detritus MCP server is available.

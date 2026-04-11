---
description: Learn from conversation corrections - distill manual fixes into KB updates
category: meta
triggers:
  - grow
  - learn
  - correction
  - you missed
  - wrong approach
  - should have used
  - already exists
  - violated rule
  - improve knowledge
  - feedback loop
when: User invokes /grow after correcting agent behavior, or wants to check last interaction against established KB guidance
related:
  - meta/truthseeker
  - meta/optimize
  - plan/index
---

# /grow — Conversation-Driven KB Improvement

> ## CRITICAL: THIS IS A CONVERSATION, NOT AN IMPLEMENTATION
>
> When `/grow` is invoked:
> 1. **DO NOT** call file editing tools (edit, multi_edit, write_to_file)
> 2. **DO NOT** run commands that modify the codebase
> 3. **ONLY** produce: Extraction, Compliance Check, Proposed KB Deltas, Questions
> 4. **ALWAYS** end with questions and wait for user confirmation before implementing

---

## Step 0: Workspace Precheck

Search workspace roots for a local detritus clone:
- Look for path containing `github.com/benitogf/detritus` with a `docs/` directory
- If found: proceed to Step 1
- If NOT found:
  - Output warning:
    ```
    ⚠️ /grow requires a local clone of the detritus MCP knowledge base to propose changes.
    
    This command analyzes conversation corrections and rule violations to improve
    the KB docs that guide agent behavior. Without a local clone, changes cannot
    be drafted.
    
    Repository: https://github.com/benitogf/detritus
    ```
  - STOP. Do not proceed.

---

## Step 1: Conversation Signal Extraction

Scan the current conversation for these signal types:

| Signal | Detection Cues |
|--------|---------------|
| **explicit_correction** | user says "no", "that's wrong", "don't do that", "stop", "I said", "not like that" |
| **missed_existing** | user points to existing doc/workflow/rule that agent should have used: "already exists", "check the docs", "use /X", "kb_get" |
| **rule_violation** | agent action contradicts established KB guidance (e.g., used sleep in tests, added backwards compat, skipped WaitGroup) |
| **implicit_redirect** | user silently fixes something agent did wrong (e.g., provides corrected code, rewrites a section) |
| **scope_drift** | agent changed requirements without approval, added unrequested features, or deviated from the task |
| **quality_bar** | user raises or clarifies quality expectations ("these docs are for you not humans", "optimize for retrieval") |

For each detected signal, produce a row:

```
| SignalType | Evidence (quote or summary) | FailureMode (short key) | Target Doc | Delta Summary |
```

Apply truthseeker principles: extract what actually happened, not what's comfortable. If the agent made no mistakes, say so — do not fabricate issues.

---

## Step 2: Guidance Compliance Check

Independently of user corrections, check the last interaction against established KB:

1. Call `kb_list()` to get all available docs
2. Identify which docs were relevant to the task (by topic, triggers, keywords)
3. For each relevant doc, call `kb_get()` and check:
   - Did the agent follow `forbidden_actions` / `anti_patterns`?
   - Did the agent satisfy `required_outputs` / `canonical_actions`?
   - Were `detection_phrases` / `triggers` present in the conversation but the doc was never consulted?
4. Output any violations as additional rows in the signal table

---

## Step 3: Proposed KB Deltas

For each failure mode identified:

### Prefer editing existing docs
- If a relevant doc already covers the topic but is missing the specific guidance, propose an edit:
  - Target file path in `docs/`
  - Section to add/modify
  - Exact content to add (optimized for agent detection, not human readability)

### New doc only when
- The failure mode is cross-cutting (applies to multiple docs)
- Or it's a repeated pattern with no natural home in existing docs
- Propose: doc name, description, triggers, and content outline

### Content style requirements
- Write for agent retrieval: keywords, detection phrases, "if X then Y" rules
- Include anti-patterns with concrete examples
- Keep prose minimal — prefer structured lists and tables
- Add to frontmatter: triggers, detection cues, related docs

---

## Step 4: Output Format

```
## Workspace
[detritus clone path or warning]

## Signals Detected
| SignalType | Evidence | FailureMode | Target Doc | Delta Summary |
|------------|----------|-------------|------------|---------------|
| ... | ... | ... | ... | ... |

## Compliance Check
| Relevant Doc | Consulted? | Violations Found |
|-------------|------------|-----------------|
| ... | ... | ... |

## Proposed KB Changes
### [target doc or new doc name]
- Type: edit / new
- Section: [where]
- Content: [draft, agent-optimized]
- Rationale: [why this prevents recurrence]

## Questions
1. [Confirm proposed changes]
2. [Clarify scope if ambiguous]
```

---

## Step 5: Wait for Confirmation

**DO NOT implement changes.** Present the plan and wait for user to:
- Approve all changes
- Approve selectively
- Request modifications
- Reject and explain why

Only after explicit approval, implement changes in the local detritus clone.

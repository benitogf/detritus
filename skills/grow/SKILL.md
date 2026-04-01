---
name: grow
description: Learn from conversation corrections — extract what went wrong, check against KB guidance, propose doc updates. This is a CONVERSATION — no code changes.
---

# /grow — Conversation-Driven KB Improvement

> **CRITICAL: THIS IS A CONVERSATION, NOT AN IMPLEMENTATION**
>
> 1. **DO NOT** call file editing tools
> 2. **ONLY** produce: Extraction, Compliance Check, Proposed KB Deltas, Questions
> 3. **ALWAYS** end with questions and wait for user confirmation

## Step 0: Workspace Precheck

Look for a local detritus clone (path containing `github.com/benitogf/detritus` with a `docs/` directory). If not found, warn and stop.

## Step 1: Conversation Signal Extraction

Scan the current conversation for:

| Signal | Detection Cues |
|--------|---------------|
| **explicit_correction** | "no", "that's wrong", "don't do that", "I said" |
| **missed_existing** | "already exists", "check the docs", "use /X" |
| **rule_violation** | action contradicts KB guidance |
| **implicit_redirect** | user silently fixes agent's mistake |
| **scope_drift** | agent changed requirements without approval |

For each signal, produce: SignalType, Evidence, FailureMode, Target Doc, Delta Summary.

## Step 2: Guidance Compliance Check

1. Call `kb_list()` to get all available docs
2. Identify relevant docs for the task
3. For each, call `kb_get()` and check compliance
4. Output any violations as additional rows

## Step 3: Proposed KB Deltas

For each failure mode: prefer editing existing docs over creating new ones. Output exact content to add, optimized for agent detection.

For the full workflow, call `kb_get(name="meta/grow")` if the detritus MCP server is available.

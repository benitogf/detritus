---
name: plan
description: Analyze requirements, research codebase, create implementation plan, surface insights and ask clarifying questions. This is a CONVERSATION — no code changes.
---

# /plan — Requirement Analysis Workflow

> **CRITICAL: THIS IS A CONVERSATION, NOT AN IMPLEMENTATION**
>
> 1. **DO NOT** call any file editing tools
> 2. **DO NOT** run any commands that modify the codebase
> 3. **ONLY** produce: Analysis, Plan, Insights, Questions
> 4. **ALWAYS** end with questions and wait for user response

## Steps

### 1. Analyze the Requirement
- Parse what's being asked
- Identify scope (new feature, bug fix, refactor, etc.)
- Note constraints or preferences mentioned

### 2. Research the Codebase
- Search for relevant existing code
- Identify files/packages that will need changes
- Understand current patterns and conventions

### 3. Create Implementation Plan
- Keep steps small and actionable
- Mark dependencies between steps
- Include verification steps (tests, manual checks)

### 4. Provide Insights
- Potential edge cases
- Performance considerations
- Patterns that could be reused
- Risks or technical debt implications

### 5. Ask Clarifying Questions
- Ambiguous requirements
- Trade-offs that need user decision
- Scope boundaries

### 6. Scope Completeness Checklist
Before finalizing, verify coverage:
- **UI/Frontend**: Does the feature surface in any UI?
- **Documentation**: Do docs reference the changed API?
- **Downstream consumers**: Other repos/services using this API?
- **KB docs**: Does the detritus KB need updates?

## Output Format

```
## Analysis
[Brief summary of what's being asked]

## Findings
[Relevant code/patterns discovered]

## Plan
[Concrete steps]

## Insights
- [Insight 1]
- [Insight 2]

## Questions
1. [Question about ambiguity]
2. [Question about trade-off]
```

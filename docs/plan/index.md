---
description: Analyze requirements/feedback, create implementation plan, provide insights and questions
category: patterns
triggers:
  - requirements
  - plan
  - analyze
  - design
  - feedback
  - specification
  - feature request
when: User provides requirements, feature request, or asks for analysis/planning before implementation
related:
  - meta/grow
  - meta/truthseeker
---

# Requirement Analysis Workflow

> ## ⛔ CRITICAL: THIS IS A CONVERSATION, NOT AN IMPLEMENTATION
> 
> When `/plan` is invoked:
> 1. **DO NOT** call any file editing tools (edit, multi_edit, write_to_file)
> 2. **DO NOT** run any commands that modify the codebase
> 3. **ONLY** produce: Analysis, Plan, Insights, Questions
> 4. **ALWAYS** end with questions and wait for user response
> 
> The purpose of `/plan` is to have a **discussion** about the task before doing anything.

When the user provides requirements, feedback, or a task description, follow this structured approach:

## 1. Analyze the Requirement

- Parse what's being asked
- Identify the scope (new feature, bug fix, refactor, etc.)
- Note any constraints or preferences mentioned

## 2. Research the Codebase

- Use code_search to find relevant existing code
- Identify files/packages that will need changes
- Understand current patterns and conventions

## 3. Create Implementation Plan

Call `update_plan` with concrete steps:
- Keep steps small and actionable
- Mark dependencies between steps
- Include verification steps (tests, manual checks)

## 4. Provide Insights

Surface any observations:
- Potential edge cases
- Performance considerations
- Patterns that could be reused
- Risks or technical debt implications

## 5. Ask Clarifying Questions

Before starting implementation, ask about:
- Ambiguous requirements
- Trade-offs that need user decision
- Scope boundaries (what's explicitly out of scope)
- Priority if multiple approaches exist

## 6. Scope Completeness Checklist

Before finalizing the plan, verify coverage of all downstream impacts:
- **UI/Frontend**: Does the feature surface in any UI? Does the UI need updates?
- **Documentation**: Are there docs, samples, or README files that reference the changed API?
- **Downstream consumers**: Are there other repos or services that use this API?
- **Samples**: Do existing samples need updating? Should a new sample be added?
- **KB docs**: Does the detritus KB need updates for the changed package?

Surface any gaps as explicit questions.

## Output Format

```
## Analysis
[Brief summary of what's being asked]

## Findings
[Relevant code/patterns discovered]

## Plan
[Call update_plan tool]

## Insights
- [Insight 1]
- [Insight 2]

## Questions
1. [Question about ambiguity]
2. [Question about trade-off]
```

---

## ⛔ STOP HERE

**DO NOT proceed to implementation.** 

After outputting the analysis, plan, insights, and questions:
1. Wait for the user to answer questions
2. Only implement when the user explicitly says to proceed
3. If no questions, still wait for user confirmation before implementing

> **Common failure mode**: User answers your questions. This is NOT confirmation to implement.
> Their answers refine the plan. You must still explicitly ask "Shall I proceed with implementation?"
> and wait for a clear "yes" / "go ahead" / "implement it".

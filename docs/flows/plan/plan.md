---
description: Analyze requirements/feedback, create implementation plan, provide insights and questions
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
  - core/planning
  - core/dream
  - flows/build/forge
  - flows/maintainer/grow
  - flows/maintainer/learn
  - flows/maintainer/absorb
  - flows/principles/truthseeker
  - core/todo-import
---

# Requirement Analysis Workflow

`/plan` is the **developer** intake of the shared planning core (`core/planning`): an open-ended conversation where a technical user steers the approach. What a settled plan *is*, the readiness and consistency checks, and the `.plan/<slug>.md` contract are defined in `core/planning` and not restated here — this doc owns the developer conversation and the on-confirm handoff. `/dream` is the executive-mode sibling (multiple-choice, architect owns tech).

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

Code context is zero-setup (no pack, no index — see `flows/project/code`):

- `code_map` for a ranked structural overview of the area (optionally `focus` on a feature/symbol); `code_outline` for a file's signatures; `code_graph` for who-calls / implementers
- `code_graph`'s `impacted_by` sizes the blast radius of the change (what transitively depends on the symbols it will touch) and `affected_tests` names the tests that must pass — run both while scoping so the plan states the change's reach and its gating tests up front, before any review
- native Grep for text/keyword search across files
- Identify files/packages that will need changes; understand current patterns and conventions

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

## On user confirmation: persist the plan as /todo items

Once the user has explicitly confirmed implementation ("yes", "go ahead", "proceed", "implement it"), AND before the first implementation tool call, invoke `/todo-import` to persist the settled plan steps as cross-session todos. This makes the plan resumable if the implementation spans multiple sessions, and gives the user a checkbox view that's automatically synced to Claude Code's TodoWrite UI.

Call shape (Shape B, structured — see `core/todo-import`):

```jsonc
{
  "group": "<short title derived from the plan's topic — e.g. 'Detritus todo skill', 'Trendboard SIGTERM fix'>",
  "source": "plan",
  "items": [
    {
      "title": "<one plan step, verbatim or lightly polished>",
      "body": null,
      "scope": { "repos": [...], "paths": [...], "concreteness": "concrete" },
      "tags": ["plan"]
    }
  ]
}
```

One item per plan step. Scope is filled in per step where you can — `/plan`'s analysis already named the files; reuse them. Items inherit the same group title (the plan's topic) so they cluster in the /todo view.

**Skip the import** if any of these hold:
- The plan has only one step (just do it; no need to track a singleton).
- The user explicitly opts out (says "skip the todo", "no need to track").
- The conversation is clearly a one-shot ("just answer this question", "explain X").

If `/todo-import` reports duplicates (the plan re-introduces items already tracked), use its dedup confirmation flow — the user picks skip / add anyway / replace per duplicate.

After the import returns, **then** proceed with implementation. Use `/todo done <id>` after each meaningful chunk so the persistent view and the in-session TodoWrite UI both reflect progress.

If `/todo-import` isn't available (older detritus build, no skill installed), fall back silently to implementing without persistence — note in the report that the plan wasn't persisted ("plan-import unavailable on this setup").

## On user confirmation: write the plan contract for an implementation loop

If the build will be driven by an implementation loop rather than implemented in-session — the user asks for `/forge`, hands the plan to candyland, or routes through `/vibe` — write the settled plan to `.plan/<slug>.md` per `core/planning` → *The plan contract*. That artifact is what `/forge` and the candyland conductor ingest; the `/todo` import above is the in-session resumable view. They are complementary: write the contract when a loop will consume the plan, the `/todo` items when you implement it here. Every task spec must meet `core/planning` → Decision-completeness.

## Incident hook — capture the lesson after the plan is settled

A detected failure, misalignment, **self-acknowledged mistake/doctrine violation**, or user correction during planning is a learning signal, but it never preempts the primary deliverable: **finish settling the plan first — never trade the deliverable for the lesson.** Detection (including the agent's own acknowledgment: "you are right, I …", "I didn't follow …", "I ignored /…") and routing are canonical in `core/ego`: user correction/self-acknowledgment → `/grow`, a PR blocker (a gate miss) → `/absorb`, telemetry → `/learn`.


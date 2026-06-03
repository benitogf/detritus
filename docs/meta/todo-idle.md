---
description: Idle-mode re-prioritization pass — Sonnet sub-agent re-ranks the full list with deep context analysis and asks the user about contentious re-orderings before committing.
category: meta
triggers:
  - todo-idle
  - idle review
  - between tasks
  - what should I do next
when: User is between tasks (no active in-flight work) and wants a thorough re-prioritization with their input on contentious calls.
related:
  - meta/todo
  - meta/todo-audit
---

# /todo-idle — Idle-Mode Re-Prioritization with User Feedback

_Follows `meta/todo` convention #13: main session validates input + calls TodoWrite + prints the confirmation line; the phases below describe the work the delegated sub-agent performs._

A deeper variant of `/todo-audit`. Where audit re-ranks based on conversation context and surfaces fork groups, idle-mode does that plus **actively asks the user about contentious items** — items where the sub-agent's confidence in the re-ranking is low, or where two items have nearly-identical scores and the user's preference would break the tie.

Idle-mode assumes the user has time to engage. Don't auto-invoke this from pivot-detection — that's `/todo-audit`'s job. Idle is explicit only.

## When to invoke

- User explicitly types `/todo idle`, "I'm between tasks", "let's plan", "what should I do next".
- A natural cadence: at the start of a new working session, after a major milestone (PR merged, sprint boundary), or when the user signals they want strategic input rather than tactical re-ranking.

## Phase 1: Read + scope

Same as `/todo-audit` Phase 1.

## Phase 2: Sonnet sub-agent (deeper analysis)

Pass to the sub-agent:
- Full active items list with current priority and scope.
- Last 20–40 conversation turns (or a summary if longer) — broader context than `/todo-audit` since idle-mode wants strategic ranking, not just pivot-reactive.
- The user's recent completed items (last 5–10 from `archive`) so the sub-agent can infer current focus areas.
- Any `/janitor` scratchpad hazards if `.janitor/` exists.

The sub-agent returns a re-ranking plus a `contentious` array:

```jsonc
{
  "items": [ ... new priorities ... ],
  "contentious": [
    {
      "ids": ["t_005", "t_008"],
      "question": "Two items rank almost level. The trendboard SIGTERM bug is operational reliability; the dashboard auth refresh is a user-visible feature gap. Which should be next?",
      "options": [
        { "id": "t_005", "label": "Trendboard SIGTERM", "implication": "Trendboard SIGTERM moves to the top; dashboard auth stays just below." },
        { "id": "t_008", "label": "Dashboard auth refresh", "implication": "Dashboard auth refresh moves to the top; trendboard SIGTERM stays just below." },
        { "id": "both", "label": "Both top — fork-eligible?", "implication": "Check fork eligibility; if both pass the gates, surface a fork plan." }
      ]
    }
  ],
  "forkGroups": [ ... same as /todo-audit ... ]
}
```

The `ids`/`id` fields are internal — the main session uses them to apply the chosen re-ranking. The `question`, `label`, and `implication` strings are shown **verbatim** to the user via `AskUserQuestion`, so they MUST obey convention #11: describe items by **title**, never by internal id and never by P-tier (`P0`/`P1`). Say "moves to the top" / "stays just below", not "goes P0, stays P1".

## Phase 3: Surface + ask

1. Print the re-ranked top 5 items (the strategic shape).
2. For each entry in `contentious`, ask via `AskUserQuestion`. Use the labels and implications from the sub-agent. Cap at 4 contentious items per pass (AskUserQuestion's max).
3. If more than 4 contentious items exist, ask the top-4 in this pass and surface the rest in the report ("3 more contentious items deferred to the next `/todo-idle` pass: ...").
4. After the user answers, recompute priorities based on their decisions and run one more sub-agent pass to settle the scores.

## Phase 4: Mutate

Same as `/todo-audit` Phase 4 — epoch-checked atomic write.

## Phase 5: TodoWrite sync + Report

After Phase 4 writes the new priorities, call `TodoWrite` per `meta/todo` convention #10. Items render as `[UPPERCASE-PREFIX] item title` per `meta/todo-view` Phase 4.

Then print, per `meta/todo` convention #11 (no ids, scores, or tiers):

```
Idle pass complete. Top 3 for now:
  • Trendboard SIGTERM bug (you chose this over Dashboard auth refresh)
  • Dashboard auth refresh (paired with Trendboard SIGTERM; possible fork)
  • Add cross-import hooks between /todo and /janitor

3 contentious items deferred to next pass.
```

If fork groups were approved, hand off to `/todo-fork` for the assignment prompts.

## Guardrails

- Don't run idle-mode reactively. It's an explicit-only command — pivot-driven re-ranking belongs in `/todo-audit`.
- Don't ask more than 4 questions in one pass (`AskUserQuestion` limit). Defer the rest, surface in the report.
- Don't skip the user-feedback step. The whole point of idle-mode vs. audit is that the user is engaged; if you don't ask, you've just run an expensive audit.
- Don't change priorities mid-question round. Apply all the user's decisions after the round is complete, not in between.
- Same fork-gate discipline as `/todo-audit` — never propose a fork that violates either gate.

---
description: Executive planning intake — turn a non-technical stakeholder's requirement into a buildable, verifiable plan through multiple-choice questions, with the agent owning every technical decision. Produces a settled spec, acceptance criteria, and recorded decisions; no build, no PR. The non-developer counterpart to /plan.
argument-hint: <desired outcome / requirement, any length>
---

# Vibe-Plan With Detritus

The user invoked this command with: $ARGUMENTS

Call the detritus MCP tool `kb_get` with `name="meta/vibe-plan"` and follow the returned guidance. Treat the user as a non-technical stakeholder: own every technical decision as the software architect, clarify intent through quick **multiple-choice** questions (no fixed cap — as many as the ambiguity needs, never open-ended prompts that make them type), and refine the requirement into a buildable, verifiable plan. Stop at the settled plan — `/vibe-plan` does **not** build, invoke `/smith`, or open a PR. Hand the plan back to whoever called it (a person, `/vibe`, or an orchestrator).

---
description: Executive intake → plan via multiple-choice questions → autonomous build → PR. Describe a requirement; the agent makes every technical decision as the architect, clarifies intent with quick multiple-choice questions while planning, then drives it through /smith to a PR with no separate approval gate.
argument-hint: <desired outcome / requirement, any length>
---

# Vibe With Detritus

The user invoked this command with: $ARGUMENTS

Call the detritus MCP tool `kb_get` with `name="meta/vibe"` and follow the returned guidance. Treat the user as a non-technical stakeholder: own every technical decision as the software architect, clarify intent through quick **multiple-choice** questions while planning (no fixed cap — as many as the ambiguity needs, never open-ended prompts that make them type), and once the plan is ready drive the work autonomously through `/smith` to an open PR — no separate "go"/approval gate. Always plan first; never start building without a plan.

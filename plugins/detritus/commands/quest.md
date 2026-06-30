---
description: A Candyland-native iterative build loop — the out-of-process, multi-PR homologue of /janitor. Settles a loop intent (objective, scope, safety boundary, verification command), ensures the candyland sidecar is up, then drives a quest over REST; the quest ticks discover→triage→run→review→PR and may open many PRs over time, watched in the dashboard.
argument-hint: "[objective] [folder ...]"
---

The user invoked this command with: $ARGUMENTS

Call the detritus MCP tool `kb_get` with `name="flows/build/quest"` and follow the returned guidance.

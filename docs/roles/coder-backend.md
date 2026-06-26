---
description: Implementation-loop coder role — implements a server/backend task until its defining test is green. Do not invoke directly — spawned by a tech-lead.
triggers:
  - backend coder
  - server task
  - backend implementation
when: Internal. Loaded by a coder agent assigned the backend role in the parallel implementation loop.
related:
  - core/coder
  - roles/tech-lead
  - roles/coder-test-engineer
  - roles/coder-frontend
  - roles/coder-fullstack
---

# Coder Role — Backend

Composes `core/coder` and adds the server/backend domain. Takes one partitioned backend task — defined by a failing test from `roles/coder-test-engineer` — and drives it to green inside its worktree.

> ## ⛔ Do not invoke directly
> No slash command. Spawned by `roles/tech-lead`; loaded via `kb_get`.

## Role delta over core/coder

- **Scope:** server-side code — handlers/endpoints, domain logic, storage, jobs, and the backend's own tests. In the ooo ecosystem, follow the relevant library guidance (`ooo/package`, `ooo/state-patterns`, `ooo/filters-internals`) and the repo's patterns.
- **Honor the interface contract.** Implement to the signatures/endpoints the test-engineer's tests assert and the tech-lead's partition names, so the frontend coder's parallel work integrates cleanly.
- **Gate:** the assigned defining test goes green and the canonical verification passes; smallest delta only; report a blocker rather than reaching outside the boundary (e.g. into shared types another task owns).

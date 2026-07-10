---
description: Implementation-loop coder role — implements one atomic task that spans both server and client until its defining test is green. Do not invoke directly — spawned by a tech-lead.
triggers:
  - fullstack coder
  - fullstack task
  - backend and frontend task
  - cross-domain task
when: Internal. Loaded by a coder agent assigned the fullstack role — a single coupled task that owns both the backend and the frontend side of one slice.
related:
  - core/coder
  - roles/tech-lead
  - roles/coder-test-engineer
  - roles/coder-backend
  - roles/coder-frontend
---

# Coder Role — Fullstack

Composes `core/coder` and owns **both** the server and client side of a single slice. It is the right role when the tech-lead judged the work **coupled** — the UI consumes an API shaped in the same change — so splitting it across two parallel agents would drift the API contract against its consumer and force a dirty merge (`roles/tech-lead` → *Partition*). Coupled work is one task regardless of size. It is **not** a new agent kind or a new control surface — it is the existing coder executor given a boundary that spans two domains.

> ## ⛔ Do not invoke directly
> No slash command. Spawned by `roles/tech-lead`; loaded via `kb_get`.

## Role delta over core/coder

- **Scope:** both sides of one slice — the backend (handler/endpoint/domain/storage, per `roles/coder-backend`) *and* the frontend that consumes it (component/view/state wiring, per `roles/coder-frontend`), plus their tests. The task's `files` boundary lists both; it is still **disjoint** from every other task's boundary (fork-safe).
- **Keep the contract consistent within the slice.** Because one agent owns both sides, the API shape and its consumer move together in the same change — there is no cross-agent contract to drift. Implement the server, then wire the client to exactly what it exposes.
- **Coupled work stays one task, regardless of size.** Backend/frontend split into separate tasks only when fully independent (neither side consumes the other's output) — that is the tech-lead's call, not the coder's. A fullstack slice that turns out to contain genuinely independent halves is a blocker to report for re-partition, not a license to sprawl.
- **Gate:** the assigned defining test goes green and the canonical verification passes; smallest delta only; report a blocker rather than reaching outside the boundary.

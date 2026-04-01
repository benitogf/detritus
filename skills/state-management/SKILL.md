---
name: state-management
description: State management patterns — single source of truth, immutable updates, optimistic UI, cache invalidation.
user-invocable: false
---

# State Management Patterns

For full reference, call `kb_get(name="patterns/state-management")` if the detritus MCP server is available.

## Key Principles

- **Single source of truth**: one authoritative location for each piece of state
- **Immutable updates**: create new state rather than mutating in place
- **Optimistic UI**: update UI immediately, reconcile with server response
- **Cache invalidation**: prefer event-driven invalidation over TTL when possible
- **Derived state**: compute from source data, don't store duplicates

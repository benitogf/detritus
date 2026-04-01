---
name: async-events
description: Async event patterns — channel-based pub/sub, event sourcing, debouncing, backpressure handling in Go.
user-invocable: false
---

# Async Event Patterns

For full reference, call `kb_get(name="patterns/async-events")` if the detritus MCP server is available.

## Key Patterns

- Use **channels** for internal pub/sub; avoid shared mutable state
- Use `select` with `context.Done()` for cancellation-aware consumers
- Apply **backpressure** via buffered channels — drop or block, never silently lose
- Debounce rapid events with `time.AfterFunc` reset pattern
- Prefer fan-out/fan-in over complex mutex orchestration

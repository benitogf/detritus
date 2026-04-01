---
name: go-backend-async
description: Testing async Go backends — polling helpers, race condition prevention, deterministic event waiting.
user-invocable: false
---

# Testing Async Go Backends

For full reference, call `kb_get(name="testing/go-backend-async")` if the detritus MCP server is available.

## Key Techniques

- **Never `time.Sleep`** in tests — use polling with timeout or channel signals
- Use `assert.Eventually` (testify) or custom poll helpers
- Use `t.Deadline()` to derive timeouts from the test runner
- Guard shared test state with `sync.Mutex` or use channels
- For WebSocket tests: read with deadline, fail fast on unexpected messages

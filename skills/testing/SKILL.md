---
name: testing
description: Testing workflows entry point — decision table for choosing mock, async, or E2E testing patterns. Links to detailed testing guides.
---

# /testing — Testing Workflows Index

Entry point for all testing-related workflows. Use this to find the right testing pattern.

## Decision Table

| Scenario | Pattern | Key Rule |
|----------|---------|----------|
| External dependency (DB, API) | **Mock** | Mock only at boundaries. Use state toggles, function injection. Real business logic. |
| Async callbacks, events, WebSocket | **Async** | Sync with `sync.WaitGroup`. NEVER use `time.Sleep` or `require.Eventually`. Calculate callback counts; assert after `wg.Wait()`. |
| Full server lifecycle | **E2E** | One test per lifecycle. Test state transitions in sequence. Phase-based structure. |

## Detailed Guides

For detailed testing guides, use the detritus MCP server:
- `kb_get(name="testing/go-backend-mock")` — Mock testing patterns
- `kb_get(name="testing/go-backend-async")` — Async testing with WaitGroup
- `kb_get(name="testing/go-backend-e2e")` — End-to-end lifecycle tests
- `kb_get(name="patterns/async-events")` — General async event principles

## Universal Rules

- **Never use `time.Sleep`** for synchronization
- **Never use `require.Eventually`** — use explicit signaling
- **One consolidated test > many small tests** for lifecycle scenarios
- **Test the real thing** — mock only external boundaries

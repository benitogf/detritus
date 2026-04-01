---
name: go-modern
description: Modern Go idioms — generics, structured logging (slog), errors.Join, context propagation, testing patterns.
user-invocable: false
---

# Modern Go Patterns

For full reference, call `kb_get(name="patterns/go-modern")` if the detritus MCP server is available.

## Key Idioms

- Use **generics** for type-safe collections and utilities (Go 1.18+)
- Prefer `log/slog` for structured logging over `log.Printf`
- Use `errors.Join` for combining multiple errors (Go 1.20+)
- Propagate `context.Context` as first parameter
- Use `t.Cleanup()` over `defer` in tests for deterministic teardown
- Use `testing/fstest` for filesystem testing

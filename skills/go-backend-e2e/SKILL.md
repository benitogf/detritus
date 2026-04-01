---
name: go-backend-e2e
description: Go backend E2E testing — test server lifecycle, database setup/teardown, parallel test isolation.
user-invocable: false
---

# Go Backend E2E Testing

For full reference, call `kb_get(name="testing/go-backend-e2e")` if the detritus MCP server is available.

## Key Techniques

- Use `TestMain` for one-time server setup and teardown
- Each test gets its own **isolated key space** to enable `t.Parallel()`
- Use `t.Cleanup()` for per-test resource teardown
- Test the full HTTP stack: real HTTP requests against a running server
- Verify both success paths and error responses (status codes, error bodies)

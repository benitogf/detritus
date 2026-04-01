---
name: go-backend-mock
description: Go backend mocking — interface-based mocks, httptest, storage mocks, dependency injection patterns.
user-invocable: false
---

# Go Backend Mocking

For full reference, call `kb_get(name="testing/go-backend-mock")` if the detritus MCP server is available.

## Key Techniques

- Define **small interfaces** at the consumer site for easy mocking
- Use `httptest.NewServer` for HTTP integration tests
- Use `httptest.NewRecorder` for handler unit tests
- Inject dependencies via struct fields or constructor functions — avoid globals
- Prefer hand-written mocks over code generation for simple interfaces

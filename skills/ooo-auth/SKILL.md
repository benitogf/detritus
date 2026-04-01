---
name: ooo-auth
description: ooo JWT authentication — /register, /authorize, /verify routes, token validation middleware, role-based access.
user-invocable: false
---

# ooo Auth Reference

For full API reference, call `kb_get(name="ooo/auth")` if the detritus MCP server is available.

Simple JWT authentication for ooo servers via `github.com/benitogf/auth`.

## Key Routes
- `POST /register` — create account
- `POST /authorize` — get JWT token
- `GET /verify` — validate token

## Integration
Auth integrates with ooo via `OpenFilter` to validate tokens on WebSocket connections and REST requests.

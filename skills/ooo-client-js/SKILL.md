---
name: ooo-client-js
description: ooo JavaScript/React WebSocket client — reconnection, JSON Patch, useSubscribe and usePublish hooks, real-time UI bindings.
user-invocable: false
---

# ooo JavaScript Client Reference

For full API reference, call `kb_get(name="ooo/client-js")` if the detritus MCP server is available.

WebSocket client for ooo servers with automatic reconnection and JSON Patch support.

## React Hooks
- `useSubscribe(path)` — subscribe to real-time updates
- `usePublish(path)` — publish data to a key path

## Features
- Automatic WebSocket reconnection
- JSON Patch for efficient updates
- TypeScript support

---
name: ooo-package
description: ooo core package — real-time state management server, filters (Read/Write/Delete/Open/Limit), CRUD helpers, WebSocket subscriptions, custom endpoints, remote operations, multi-glob patterns.
user-invocable: false
---

# ooo Core Package Reference

For the full API reference and code samples, call `kb_get(name="ooo/package")` if the detritus MCP server is available.

## Overview

ooo is a real-time state management server for Go. Key concepts:

- **Filters**: `ReadObjectFilter`, `ReadListFilter`, `WriteFilter`, `AfterWriteFilter`, `DeleteFilter`, `OpenFilter`, `LimitFilter`
- **CRUD**: `Set`, `Get`, `Del` on key paths
- **WebSocket**: Real-time subscriptions with JSON Patch updates
- **Custom endpoints**: Extend the REST API
- **Remote operations**: Cross-server data sync
- **Storage adapters**: Memory, LevelDB (ko), PostgreSQL (nopog)

## Key Anti-Patterns

- ❌ Don't bypass filters by accessing storage directly for user-facing operations
- ❌ Don't use `time.Sleep` in tests involving ooo callbacks — use `sync.WaitGroup`
- ❌ Don't forget `LimitFilter` has cleanup goroutines that need proper shutdown

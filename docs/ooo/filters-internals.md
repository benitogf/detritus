---
description: ooo filters internals - how filters are enforced and when they are bypassed
category: core
triggers:
  - filter bypass
  - direct storage
  - store.Set
  - storage.Set
  - LimitFilter internals
  - AfterWrite
  - WriteFilter enforcement
  - filters gate writes
  - filters read-side
when: Agent is reasoning about whether filters apply to direct storage operations, or claiming filters prevent/gate storage writes
related:
  - ooo-package
---

# ooo Filters Internals

## Filter Enforcement Points

Filters are enforced at the **HTTP/WebSocket API layer** (`rest.go`), NOT at the storage layer.

| Filter | Enforced Where | Trigger |
|--------|---------------|---------|
| `WriteFilter` | `rest.go` POST/PATCH handler | `server.filters.Write.Check()` before `storage.Set()` |
| `ReadObjectFilter` | `rest.go` GET handler | `server.filters.ReadObject.Check()` after `storage.Get()` |
| `ReadListFilter` | `rest.go` GET handler | `server.filters.ReadList.Check()` after `storage.GetList()` |
| `DeleteFilter` | `rest.go` DELETE handler | `server.filters.Delete.Check()` before `storage.Del()` |
| `AfterWriteFilter` | `rest.go` POST handler (after successful write) | Callback after write completes |

## Direct Storage Bypasses All Filters

```go
// ❌ WRONG assumption: "this bypasses server filters"
// Filters don't apply here — they never did. This is expected behavior.
store.Set(key, data)

// Filters only apply to HTTP API writes:
// POST /key → WriteFilter.Check() → storage.Set() → AfterWriteFilter
```

Direct `storage.Set()`, `storage.Del()`, `storage.Get()` calls **never** pass through filters. This is by design — filters are an API-layer concern, not a storage-layer concern.

## LimitFilter Specifics

`LimitFilter` registers multiple sub-filters:

1. **`AddWrite(path, NoopFilter)`** — allows REST writes (permissive)
2. **`AddDelete(path, NoopHook)`** — allows REST deletes (permissive)
3. **`ReadListFilter`** — trims results to limit on read (enforced on read)
4. **`ReadObjectFilter`** — allows individual item reads
5. **`AfterWrite(path, lf.Check)`** — deletes oldest entries over limit **after REST writes**

The cleanup (`lf.Check()`) only fires via `AfterWrite`, which only triggers on writes through the HTTP API. Code that calls `storage.Set()` directly must handle its own cleanup if limit enforcement is needed.

## Anti-Patterns

```
❌ "package-level QueueMessage bypasses server filters"
   → Filters are API-layer. Direct storage writes are not "bypassing" anything.

❌ "LimitFilter prevents writes beyond the limit"  
   → LimitFilter does NOT prevent writes. It cleans up old entries AFTER a write
     via AfterWrite, and trims read results via ReadListFilter.

❌ "WriteFilter gates all writes to storage"
   → WriteFilter only gates writes coming through the REST API (POST/PATCH).
     Direct storage.Set() is unaffected.
```

## Correct Mental Model

```
HTTP POST /key
  → WriteFilter.Check() — can reject/transform
  → storage.Set()
  → AfterWriteFilter — side effects, cleanup
  → broadcast to WebSocket subscribers

Direct storage.Set(key, data)
  → storage.Set()
  → broadcast to WebSocket subscribers (via storage watch)
  → NO filters involved
```


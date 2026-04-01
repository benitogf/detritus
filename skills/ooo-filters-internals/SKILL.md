---
name: ooo-filters-internals
description: ooo filter pipeline internals — filters enforce at HTTP layer not storage, LimitFilter specifics, direct storage bypasses filters.
user-invocable: false
---

# ooo Filter Internals

For full reference, call `kb_get(name="ooo/filters-internals")` if the detritus MCP server is available.

## Key Facts

- Filters enforce at the **HTTP layer**, NOT at the storage layer
- Direct storage access (`app.Storage.Set()`) **bypasses all filters**
- `LimitFilter` runs cleanup goroutines — ensure proper shutdown in tests
- Filter chain order matters: earlier filters can short-circuit later ones

---
description: Server-side state management using ooo typed CRUD and persistent flags
category: core
triggers:
  - ooo state
  - ooo metrics
  - ooo pending
  - ooo flag
  - ooo.Get state
  - ooo.Set state
  - pending reset
  - metrics tick
when: Managing server-side state (metrics, flags, scheduled actions) through ooo's typed CRUD helpers
related:
  - ooo/package
  - patterns/state-management
  - patterns/coding-style
---

# Server-Side State with ooo

Applies the patterns from `patterns/state-management` (single-writer, no wasted writes, persistent flags) using ooo's typed CRUD helpers. Read that doc first for the general principles.

---

## Typed State Access

Use generic helpers instead of raw JSON:

```go
// Read typed state
metrics, err := ooo.Get[Metrics](server, "metrics/myservice")

// Write typed state
ooo.Set(server, "metrics/myservice", Metrics{Count: 42})

// Delete state
ooo.Delete(server, "metrics/myservice")
```

---

## Conditional Writes

Avoid writing to ooo storage when the value hasn't changed:

```go
func UpdateStatus(server *ooo.Server, status string) {
    current, err := ooo.Get[Status](server, "status")
    if err == nil && current.Value == status {
        return
    }
    ooo.Set(server, "status", Status{Value: status})
}
```

---

## Path Conventions

| State type | Path pattern | Example |
|-----------|-------------|---------|
| Pending action | `pending/{action}/{domain}/{service}` | `pending/reset/metrics/myservice` |
| Domain state | `{domain}/{service}` | `metrics/myservice` |
| Settings | `settings` | `settings` |

All paths used for state should have appropriate filters registered (see `ooo/package`).

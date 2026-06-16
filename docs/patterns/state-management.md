---
description: State mutation patterns - single-writer, consolidation, no wasted writes
triggers:
  - state management
  - state mutation
  - wasted write
  - double write
  - overwrite
  - single writer
  - consolidate
  - counter
  - increment
  - clear
  - pending
  - flag
  - schedule
  - defer
when: Writing code that reads, modifies, or writes shared state (metrics, flags, state, settings)
related:
  - flows/principles/coding-style
---

# State Mutation Patterns

> ## AUTOMATIC TRIGGER RULE
>
> When writing code that mutates shared state, MUST:
> 1. **Never write a value that will be immediately overwritten** — consolidate into one write
> 2. **Use a single function** for conditional mutations on the same state
> 3. **Deferred actions use persistent flags** — not in-memory state that can be lost

---

## 1. No Wasted Writes

If two writes to the same key happen in sequence and the second overwrites the first, the first write is wasted.

### Rule: Consolidate sequential writes into one

```go
// BAD: Two writes — first is wasted
metrics := store.GetMetrics()
metrics.Count++
store.SetMetrics(metrics)   // write 1
// ... then later in same flow:
store.SetMetrics(Metrics{}) // write 2 overwrites write 1

// GOOD: Single function decides what to write
func MetricsTick(store Store, increment bool) {
    if store.HasPendingReset() {
        ResetMetrics(store)
    }
    if increment {
        metrics := store.GetMetrics()
        metrics.Count++
        store.SetMetrics(metrics)
    }
}
```

### Rule: Read state before deciding whether to write

```go
// BAD: Always writes, even when value hasn't changed
func UpdateStatus(store Store, status string) {
    store.Set("status", Status{Value: status})
}

// GOOD: Only writes when needed
func UpdateStatus(store Store, status string) {
    current, err := store.Get("status")
    if err == nil && current.Value == status {
        return
    }
    store.Set("status", Status{Value: status})
}
```

---

## 2. Single Writer Per State Mutation

When a state value can be mutated by multiple conditions (increment, clear, reset), consolidate into one function that handles all cases.

### Rule: One function owns the decision of what to write

```go
// BAD: Caller decides — logic scattered
if shouldReset {
    store.SetMetrics(Metrics{})
} else if shouldIncrement {
    m := store.GetMetrics()
    m.Count++
    store.SetMetrics(m)
}

// GOOD: Single function owns it
MetricsTick(store, shouldIncrement)
```

### Rule: Cleanup related state in the same function

```go
// GOOD: ResetMetrics owns all side effects of resetting
func ResetMetrics(store Store) {
    store.SetMetrics(Metrics{})
    store.Delete(pendingKey) // also cancels any pending schedule
}
```

---

## 3. Deferred Actions via Persistent Flags

When an action should happen later (e.g., "reset on next state transition"), use a persistent flag in storage — not in-memory state that can be lost on restart.

### Rule: Use storage flags, not in-memory state

```go
// BAD: In-memory flag — lost on restart
var pendingReset bool

// GOOD: Persistent flag in storage
func ScheduleResetMetrics(store Store) {
    store.Set("pending/reset/metrics", PendingReset{Pending: true})
}
```

### Rule: The consumer of the flag must delete it after acting

```go
func MetricsTick(store Store, increment bool) {
    if store.HasPendingReset() {
        ResetMetrics(store) // ResetMetrics deletes the flag internally
    }
    if increment {
        metrics := store.GetMetrics()
        metrics.Count++
        store.SetMetrics(metrics)
    }
}
```

### Rule: Immediate actions must cancel pending deferred actions

If an immediate clear happens, any pending scheduled clear must be cancelled — otherwise the flag persists and causes a spurious clear later.

```go
// GOOD: Immediate reset also deletes pending flag
func ResetMetrics(store Store) {
    store.SetMetrics(Metrics{})
    store.Delete(pendingKey)
}
```

---

## 4. Flag Naming Conventions

| Pattern | Key Format | Example |
|---------|-----------|---------|
| Pending action | `pending/{action}/{domain}/{variant}` | `pending/reset/metrics/myservice` |
| State value | `{domain}/{variant}` | `metrics/myservice` |
| Settings | `settings` | `settings` |

---

## 5. Edge Cases to Consider

1. **Deferred reset on already-zero state**: The reset is a no-op but the flag still gets consumed — the increment must not be skipped
2. **Multiple schedules before consumption**: Idempotent — setting the flag twice is harmless
3. **Immediate action while flag is pending**: Immediate action must delete the flag to prevent double-reset

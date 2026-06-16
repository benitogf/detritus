---
description: Self-documenting code - naming, extraction, readability rules for AI
triggers:
  - naming
  - function name
  - rename
  - readability
  - self-documenting
  - extract function
  - inline code
  - coding style
  - code style
  - comment
  - comments
  - commenting
when: Writing or modifying any code - names must reflect current behavior, inline operations extracted into named functions, and comments kept terse (only for a non-obvious WHY)
related:
  - patterns/state-management
---

# Self-Documenting Code

> ## AUTOMATIC TRIGGER RULE
>
> When writing or modifying code, MUST ensure:
> 1. **Every function name describes its current behavior** — not its historical or intended behavior
> 2. **Inline multi-step operations are extracted** into named functions
> 3. **Side effects are visible in the name** — if it writes, deletes, or schedules, the name says so
> 4. **Comments are terse** — default to none; one short line max; only when the WHY is genuinely non-obvious, never restating the code or paraphrasing the name

---

## 1. Names Must Reflect Current Behavior

A function name is a contract. If the behavior changes, the name must change.

### Rule: If a function no longer does what its name says, rename it immediately

```go
// BAD: ResetMetrics used to reset immediately, now it schedules a deferred reset
func ResetMetrics(store Store) {
    store.Set("pending/reset/metrics", PendingReset{Pending: true})
}

// GOOD: Name reflects actual behavior
func ScheduleResetMetrics(store Store) {
    store.Set("pending/reset/metrics", PendingReset{Pending: true})
}
```

### Rule: Side effects must be visible in the name

```go
// BAD: Name suggests read-only, but it writes
func GetOrCreateUser(db *sql.DB, name string) User { ... }

// GOOD: Side effect is clear
func EnsureUser(db *sql.DB, name string) User { ... }
```

### Rule: Deferred vs immediate must be distinguished

```go
// GOOD: Clear distinction between immediate and scheduled
func ResetMetrics(store Store)         { ... } // zeros metrics now
func ScheduleResetMetrics(store Store) { ... } // sets flag for later
```

---

## 2. Extract Inline Operations

When multiple steps are performed inline, extract them into a named function that describes the combined operation.

### Rule: If a block of code has a describable purpose, it should be a function

```go
// BAD: Inline multi-step operation
metrics := store.GetMetrics()
metrics.Count++
store.SetMetrics(metrics)

// GOOD: Extracted with clear name
func IncrementMetrics(store Store) {
    metrics := store.GetMetrics()
    metrics.Count++
    store.SetMetrics(metrics)
}
```

### Rule: Conditional mutation blocks should be functions

```go
// BAD: Inline conditional logic scattered across caller
pending, err := store.Get("pending/reset", &reset)
if err == nil {
    store.SetMetrics(Metrics{})
    store.Delete("pending/reset")
} else {
    metrics := store.GetMetrics()
    metrics.Count++
    store.SetMetrics(metrics)
}

// GOOD: Extracted: caller just says what it wants
MetricsTick(store, true)
```

---

## 3. Naming Conventions

| Pattern | Convention | Example |
|---------|-----------|---------|
| Immediate action | Verb + Noun | `ResetMetrics`, `SetMetrics` |
| Deferred/scheduled action | `Schedule` + Verb + Noun | `ScheduleResetMetrics` |
| Conditional tick/step | Noun + `Tick` | `MetricsTick` |
| Predicate check | `Is`/`Has`/`Can` + Noun | `IsReady`, `HasPending` |

---

## 4. When Refactoring

1. **Change behavior -> change name** in the same commit
2. **Update all callers** — never leave a caller using a stale name
3. **If two functions now exist** (immediate + deferred), ensure both names make the distinction obvious
4. **Search the codebase** for all usages before renaming — use grep, not assumptions

---

## 5. Comments — Terse, and Only for a Non-Obvious WHY

Default to **no comment**. Names (sections 1–3) carry the meaning; a comment is a last resort for the rare case where the *why* genuinely can't live in the code.

### Rule: Never restate what the code says or paraphrase the name

```go
// BAD: restates the code / paraphrases the function name
count++ // increment the counter

// setUser sets the user
func setUser(...) { ... }

// GOOD: no comment — the code and name already say it
count++
func setUser(...) { ... }
```

**Exported (public) APIs are the exception:** keep their doc comment, but make it state the *contract* or a non-obvious *why* — not a restatement of the name. `// SetUser sets the user` is noise; `// SetUser replaces any existing record for the user.` earns its place.

### Rule: Comment only when the WHY is non-obvious — one short line

```go
// GOOD: explains a reason the code itself cannot convey
// porter swallows the retry cause, so own the loop to surface each attempt's error
for attempt := 1; attempt <= maxAttempts; attempt++ { ... }
```

A comment earns its place only when deleting it would lose information not recoverable from the code: a non-obvious constraint, why a workaround exists, why *not* the obvious approach. Keep it to one short line. When the code changes, update or delete a now-stale comment in the same commit — the same discipline as names in section 1.

---
name: line-of-sight
description: Line-of-sight code style — flat code, early returns, error handling separated from business logic, happy path left-aligned.
disable-model-invocation: true
---

# /line-of-sight — Flat Code Style

## Core Principle

The happy path should be visible at a glance — left-aligned, no deep nesting.

## Rules

1. **Error checks first**: Handle errors immediately, return early
2. **Happy path left-aligned**: Business logic stays at the lowest indentation level
3. **No else after return**: If the `if` block returns, don't use `else`
4. **Guard clauses at top**: Validate inputs before proceeding

## Examples

❌ Bad — nested, hard to follow:
```go
func process(data []byte) error {
    if data != nil {
        result, err := parse(data)
        if err == nil {
            if result.Valid {
                return save(result)
            }
        } else {
            return err
        }
    }
    return errors.New("no data")
}
```

✅ Good — flat, line of sight:
```go
func process(data []byte) error {
    if data == nil {
        return errors.New("no data")
    }
    result, err := parse(data)
    if err != nil {
        return err
    }
    if !result.Valid {
        return errors.New("invalid result")
    }
    return save(result)
}
```

For the full guide, call `kb_get(name="patterns/line-of-sight")` if the detritus MCP server is available.

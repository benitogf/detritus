---
description: Line-of-sight code style - flat code, early returns, separate error handling from business logic
category: style
triggers:
  - error handling
  - nested if
  - happy path
  - line of sight
  - early return
  - guard clause
  - nesting
  - flat code
  - if err == nil
  - err != nil
  - error checking
related:
  - go-modern
  - truthseeker
---

# Line of Sight

Reference: https://medium.com/@matryer/line-of-sight-in-code-186dd7cdea88

All languages. Not Go-specific.

## Rules

1. Happy path is left-aligned — the success case runs at the lowest indentation
2. Handle errors/edge cases first with early return/continue
3. Error checks are always their own `if` block — never combined with business logic
4. Never use `if err == nil` — flip to `if err != nil { return }`
5. Never rename `err` — handle it immediately after the call, reuse the name
6. Log errors with function context prefix before returning
7. Business logic conditions come after all error handling, as their own `if` block
8. No rightward drift — if code indents more than 2 levels, refactor

## Error Handling vs Business Logic

These are separate concerns. Never combine them.

Error handling: `if err != nil { log; return }` — immediately after the call
Business logic: `if condition { ... }` — only after all data is successfully loaded

## Anti-patterns

### NEVER: mixed error and business logic
```
settings, errSettings := getSettings()
device, errDevice := getDevice()
if errSettings == nil && settings.Enabled && errDevice == nil && !device.Open {
```
Problems: renamed err, mixed error checks with conditions, no logs, not debuggable.

### NEVER: nested success checks
```
if err == nil && condition {
    device, err := getDevice()
    if err == nil && !device.Closed {
```
Problems: happy path drifts right, errors are implicit, not debuggable.

### NEVER: err == nil check
```
if err == nil {
    // do thing
}
```
Flip it. Always check for the error, not the success.

## Correct Pattern

```
data, err := getData()
if err != nil {
    log.Println("functionName: failed to get data", err)
    return err // or http 400, or continue in loops
}

device, err := getDevice()
if err != nil {
    log.Println("functionName: failed to get device", err)
    return err
}

if condition && !device.Open {
    log.Println("functionName: skipping because reason")
    return
}

// happy path — left-aligned, all data available, no error checks
doThing(data, device)
```

## Detection — self-check before generating code

- `if err == nil &&` — wrong, flip it
- `errFoo` or `errBar` variable names — wrong, just use `err` and handle immediately
- `if errA == nil && x.Field && errB == nil && !y.Field` — wrong, separate error checks from logic
- error check and business condition in same `if` — wrong
- happy path indented 2+ levels — refactor

## In loops (goroutines, tickers)

Same rules but use `continue` instead of `return` for non-fatal errors:
```
data, err := getData()
if err != nil {
    log.Println("functionName: failed to get data", err)
    continue
}
```

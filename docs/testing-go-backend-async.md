---
description: Async testing - deterministic async event synchronization patterns
category: testing
triggers:
  - WaitGroup
  - wg.Add
  - wg.Wait
  - async
  - deterministic
  - callback
  - subscription test
  - race condition
  - flaky test
  - require.Eventually
  - Eventually
  - polling
when: Writing tests with async operations, WaitGroup patterns, subscription tests, fixing flaky tests
related:
  - testing
  - testing-go-backend-mock
  - testing-go-backend-e2e
  - ooo-package
---

# Async Testing Patterns

This skill covers deterministic testing of asynchronous events using `sync.WaitGroup` with precise counters. **Never use sleep, channels, or timing-based synchronization.**

---

## Core Principles

1. **WaitGroups are deterministic** - if they fail, your count is wrong
2. **Calculate exact counts before the test runs** - never guess
3. **Callbacks only update state** - assert outside after `wg.Wait()`
4. **"Unpredictable" and "timing" are not valid explanations** for WaitGroup issues
5. **Add logs before attempting fixes** - prove what's happening
6. **Consistency over cleverness** - even if an alternative "works," use WaitGroup

---

## ⚠️ Consistency Over Correctness

Even if a channel-based or alternative approach "works," **do not use it**. The codebase must be consistent.

**Why consistency matters:**
- New developers learn ONE pattern, not variations
- Code review is faster when patterns are predictable
- Refactoring is safer with uniform synchronization
- "It works" is not sufficient justification for divergence

**If you find yourself justifying a channel with "but it's deterministic because..."** - stop and use WaitGroup instead.

### ❌ Wrong: Channels for Sync (even if "deterministic")

```go
// DON'T DO THIS - even with atomic gating
statusCh := make(chan Status, 100)
var expected atomic.Int32

callback := func(s Status) {
    if expected.Add(-1) >= 0 {
        statusCh <- s  // "deterministic" channel
    }
}

expected.Add(1)
<-statusCh  // Still wrong - uses channel
```

### ✅ Correct: Always WaitGroup

```go
var statusWg sync.WaitGroup

callback := func(s Status) {
    statusWg.Done()
}

statusWg.Add(1)
statusWg.Wait()  // Consistent pattern
```

---

## Basic Pattern

```go
func TestAsyncEvent(t *testing.T) {
    var wg sync.WaitGroup
    
    // Calculate exact expected callback count BEFORE setup
    expectedCallbacks := 3
    wg.Add(expectedCallbacks)
    
    handler := func(data SomeType) {
        defer wg.Done()
        // process data
    }
    
    // Trigger async events
    triggerEvents()
    
    wg.Wait()
}
```

---

## Subscription Testing Pattern

When testing WebSocket subscriptions with callbacks:

```go
func TestSubscription(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    
    var wg sync.WaitGroup
    
    // Count: 1 for initial state + N for each subsequent event
    wg.Add(1 + len(eventsToTrigger))
    
    go client.Subscribe(cfg, "path", client.SubscribeEvents[T]{
        OnMessage: func(item client.Meta[T]) {
            wg.Done()
            // assertions...
        },
    })
    
    // Wait for initial subscription message
    // Then trigger events...
    
    wg.Wait()
}
```

---

## Multiple Subscribers Pattern

When multiple subscriptions observe the same events:

```go
func TestMultipleSubscribers(t *testing.T) {
    var wg sync.WaitGroup
    
    // Each subscriber gets: initial message + each event
    numSubscribers := 2
    numEvents := 3
    callbacksPerSubscriber := 1 + numEvents
    
    wg.Add(numSubscribers * callbacksPerSubscriber)
    
    // Setup subscribers...
    // Trigger events...
    
    wg.Wait()
}
```

---

## Debugging WaitGroup Issues

When WaitGroup deadlocks or panics with negative counter:

### Step 1: Add Logging to Callbacks

```go
OnMessage: func(item client.Meta[T]) {
    log.Printf("CALLBACK: key=%s, index=%s", key, item.Index)
    wg.Done()
}
```

### Step 2: Log Before Each Add()

```go
log.Printf("ADD: count=%d, reason=%s", count, reason)
wg.Add(count)
```

### Step 3: Run with Verbose Output

```bash
go test -v -run TestYourTest -timeout 5s 2>&1 | head -100
```

### Step 4: Analyze the Output

The count mismatch will be obvious:
- More `CALLBACK` logs than `ADD` → negative counter panic
- Fewer `CALLBACK` logs than `ADD` → deadlock

**If you're on your 3rd attempted fix without logs, you're guessing - stop and add logs.**

---

## Common Pitfalls

### ❌ Wrong: Using Sleep

```go
// NEVER DO THIS
time.Sleep(100 * time.Millisecond)
if result != expected {
    t.Fatal("wrong result")
}
```

### ❌ Wrong: Using require.Eventually

```go
// NEVER DO THIS - polling is non-deterministic
require.Eventually(t, func() bool {
    return countItems(storage) == 0
}, time.Second, 10*time.Millisecond, "queue should drain")
```

**Why it's wrong:**
- Polling interval adds arbitrary timing
- Hides the actual synchronization problem
- Can pass with race conditions that later fail in CI
- Doesn't prove causality between action and result

**The fix:** Add a callback hook to the component being tested (see `/testing-go-backend-mock`).

### ✅ Correct: Precise WaitGroup

```go
var wg sync.WaitGroup
wg.Add(exactExpectedCount)

callback := func() {
    defer wg.Done()
    // work...
}

wg.Wait() // Blocks until exactly N callbacks complete
```

---

## State Flow Testing Pattern

**Critical**: Keep subscription callbacks simple - only update state. Assert outside after `wg.Wait()`.

```go
var state Status
var wg sync.WaitGroup

// Callback ONLY updates state
go client.Subscribe(cfg, "status", client.SubscribeEvents[Status]{
    OnMessage: func(item client.Meta[Status]) {
        state = item.Data
        wg.Done()
    },
})

// Wait for initial state, then assert
wg.Add(1)
wg.Wait()
require.Equal(t, "ok", state.State, "initial state must be ok")

// Trigger action, wait, assert
wg.Add(1)
triggerStateChange()
wg.Wait()
require.Equal(t, "offline", state.State, "after trigger, must be offline")
```

---

## Checklist Before Saying "It's Unpredictable"

1. [ ] Did you add logs showing every callback invocation?
2. [ ] Did you add logs before every `wg.Add()` call?
3. [ ] Did you run with `-v` and read the actual output?
4. [ ] Did you verify the count for the FIRST event separately from subsequent events?
5. [ ] Did you check if certain input patterns trigger extra callbacks?

If any answer is "no", you haven't proven the behavior yet - add the logs.

---

## Related Workflows

- `/testing` - Testing workflow index
- `/testing-go-backend-mock` - What and how to mock
- `/testing-go-backend-e2e` - Consolidated E2E test patterns

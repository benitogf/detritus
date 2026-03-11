---
description: E2E testing - consolidated tests covering full state lifecycles
category: testing
triggers:
  - e2e
  - end-to-end
  - integration
  - lifecycle
  - state transitions
  - consolidated test
  - single test
  - coverage
when: Structuring tests, deciding between many small tests vs one comprehensive test, testing state machines
related:
  - testing
  - testing-go-backend-async
  - testing-go-backend-mock
---

# E2E Testing Patterns

This workflow covers how to structure comprehensive end-to-end tests that verify full state lifecycles rather than isolated function coverage.

---

## Core Principle: Test Behavior, Not Functions

**One comprehensive E2E test** that exercises the full lifecycle is better than many small tests that cover individual functions.

### ❌ Wrong: Disjointed Function Coverage Tests

```go
func TestStatusOK(t *testing.T) {
    server := setup(t)
    // assert initial state is OK
}

func TestStatusOffline(t *testing.T) {
    server := setup(t)  // duplicate setup
    // somehow get to offline state
    // assert offline
}

func TestStatusOverflow(t *testing.T) {
    server := setup(t)  // duplicate setup
    // somehow get to overflow state
    // assert overflow
}

func TestQueueDrains(t *testing.T) {
    server := setup(t)  // duplicate setup
    // populate queue, wait, assert empty
}
```

**Problems:**
- Each test duplicates setup/teardown
- Cannot verify state transitions in sequence
- Often leads to `require.Eventually` to check intermediate states
- Hard to verify ordering and causality
- Tests functions in isolation, not real behavior

### ✅ Correct: Single Consolidated E2E Test

```go
func TestPublisherE2E(t *testing.T) {
    server := newE2EServer(t)
    kafka := newMockKafka()
    
    w := NewWorker(WorkerConfig{
        Server:   server,
        SendFunc: kafka.send,
    })
    w.Start()
    defer w.Close()
    
    var sendWg sync.WaitGroup
    kafka.setOnSend(func(msg Message) { sendWg.Done() })
    
    // Phase 1: Connected - direct send works
    sendWg.Add(2)
    w.QueueMessage("topic", "msg1")
    w.QueueMessage("topic", "msg2")
    sendWg.Wait()
    require.Len(t, kafka.getMessages(), 2)
    require.Equal(t, 0, countQueue(server), "no queue when connected")
    
    // Phase 2: Disconnected - messages queue
    kafka.connected.Store(false)
    w.QueueMessage("topic", "msg3")
    require.Equal(t, 1, countQueue(server), "queued when disconnected")
    
    // Phase 3: Reconnected - queue drains
    sendWg.Add(1)
    kafka.connected.Store(true)
    sendWg.Wait()
    require.Equal(t, 0, countQueue(server), "drained on reconnect")
    
    // Phase 4: Verify ordering preserved
    msgs := kafka.getMessages()
    require.Equal(t, "msg1", msgs[0].Payload)
    require.Equal(t, "msg2", msgs[1].Payload)
    require.Equal(t, "msg3", msgs[2].Payload)
}
```

**Benefits:**
- Tests complete workflow in sequence
- Proves state transitions and causality
- No `require.Eventually` needed with proper callbacks
- Single setup/teardown
- Verifies ordering across state changes

---

## The Phase Pattern

Structure E2E tests as sequential phases, each testing a state transition:

```go
func TestE2E(t *testing.T) {
    // Setup once
    server, component, mock := setup(t)
    defer cleanup()
    
    // Phase 1: Initial state
    // - Verify starting conditions
    // - No external triggers yet
    
    // Phase 2: First transition
    // - Trigger state change
    // - Wait for async completion
    // - Assert new state
    
    // Phase 3: Second transition
    // - Trigger another change
    // - Wait
    // - Assert
    
    // Phase N: Final state
    // - Verify end conditions
    // - Check ordering/history
}
```

---

## Event Triggering and State Verification

Each phase follows the same pattern:

```go
// 1. Set expectations (WaitGroup count)
sendWg.Add(expectedSends)
statusExpected.Add(expectedStatusUpdates)

// 2. Trigger the action
component.DoSomething()

// 3. Wait for completion
sendWg.Wait()
<-statusCh  // or however you sync status

// 4. Assert the result
require.Equal(t, expectedState, actualState)
require.Equal(t, expectedCount, actualCount)
```

**Critical**: The wait proves the action completed. The assertion proves the result.

---

## Status Subscription Pattern

For components that publish status updates:

```go
var statusMu sync.Mutex
var status QueueStatus
var statusExpected atomic.Int32
statusCh := make(chan QueueStatus, 100)

go client.Subscribe(cfg, statusKey, client.SubscribeEvents[QueueStatus]{
    OnMessage: func(item client.Meta[QueueStatus]) {
        statusMu.Lock()
        status = item.Data
        statusMu.Unlock()
        if statusExpected.Add(-1) >= 0 {
            statusCh <- item.Data
        }
    },
})

// Wait for initial status
statusExpected.Add(1)
<-statusCh
require.Equal(t, StateOK, status.State)

// Trigger change, wait, assert
statusExpected.Add(1)
triggerOffline()
<-statusCh
require.Equal(t, StateOffline, status.State)
```

---

## What to Cover in E2E Tests

| Aspect | Include | Why |
|--------|---------|-----|
| Happy path | ✅ | Basic functionality works |
| State transitions | ✅ | System responds to changes |
| Error recovery | ✅ | System handles failures |
| Ordering | ✅ | Operations maintain order |
| Edge cases | ✅ | Boundaries are handled |
| Capacity limits | ✅ | Limits are enforced |
| Final state | ✅ | System ends in valid state |

---

## When to Use Separate Tests

Not everything belongs in one E2E test. Use separate tests for:

| Scenario | Approach |
|----------|----------|
| Unit logic (pure functions) | Separate unit tests |
| Configuration validation | Separate config tests |
| Concurrency stress | Separate concurrent test |
| Benchmark performance | Separate benchmark |
| Edge case that needs special setup | Separate focused test |

**Rule**: If it requires fundamentally different setup, make it a separate test.

---

## E2E Test Structure Template

```go
func TestComponentE2E(t *testing.T) {
    // === SETUP ===
    server := newE2EServer(t)
    mock := newMockExternal()
    component := NewComponent(Config{
        Server:       server,
        ExternalFunc: mock.send,
    })
    component.Start()
    defer component.Close()
    
    // Setup sync primitives
    var wg sync.WaitGroup
    mock.setOnSend(func(r Record) { wg.Done() })
    
    // Setup status subscription if needed
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    // ... subscribe to status
    
    // === PHASE 1: Initial State ===
    // Wait for initial status
    // Assert starting conditions
    
    // === PHASE 2: Normal Operation ===
    wg.Add(N)
    // trigger N operations
    wg.Wait()
    // assert results
    
    // === PHASE 3: Failure Mode ===
    mock.connected.Store(false)
    // trigger operations that should queue/fail
    // assert failure handling
    
    // === PHASE 4: Recovery ===
    wg.Add(M)
    mock.connected.Store(true)
    wg.Wait()
    // assert recovery behavior
    
    // === PHASE 5: Final Verification ===
    // assert final state
    // verify ordering
    // check no leaked resources
}
```

---

## Avoiding Common E2E Mistakes

### ❌ Using require.Eventually

```go
// WRONG: Polling hides sync issues
require.Eventually(t, func() bool {
    return queue.Len() == 0
}, time.Second, 10*time.Millisecond)
```

**Fix**: Use callback hooks with WaitGroup (see `/testing-go-backend-async`).

### ❌ Testing in Arbitrary Order

```go
// WRONG: Testing states without transitions
t.Run("overflow", func(t *testing.T) { /* jump to overflow state */ })
t.Run("ok", func(t *testing.T) { /* test ok state separately */ })
```

**Fix**: Test transitions in sequence within one test.

### ❌ Ignoring Ordering

```go
// WRONG: Only checking counts, not order
require.Len(t, messages, 5)
```

**Fix**: Verify exact ordering matches expectations.

```go
// CORRECT
require.Equal(t, "msg1", messages[0].Payload)
require.Equal(t, "msg2", messages[1].Payload)
// ...
```

---

## Related Workflows

- `/testing` - Testing workflow index
- `/testing-go-backend-async` - WaitGroup patterns for async synchronization
- `/testing-go-backend-mock` - Minimal mocking at boundaries

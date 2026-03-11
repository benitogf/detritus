---
description: Testing index - entry point for all testing workflows
category: testing
triggers:
  - test
  - testing
  - unit test
  - how to test
when: Starting to write tests, deciding testing approach
related:
  - testing-go-backend-mock
  - testing-go-backend-async
  - testing-go-backend-e2e
---

# Testing Workflows Index

This is the entry point for all testing-related workflows. Use this to find the right testing pattern for your situation.

---

## Quick Decision

| I need to... | Use |
|--------------|-----|
| Decide what to mock vs run real | `/testing-go-backend-mock` |
| Synchronize async events with WaitGroup | `/testing-go-backend-async` |
| Structure comprehensive lifecycle tests | `/testing-go-backend-e2e` |

**Most tests need all three** - mock at boundary, sync with WaitGroup, structure as E2E.

---

## Workflow Summaries

### `/testing-go-backend-mock` - Minimal Mocking
**When:** Deciding what to mock, structuring mocks

Key principles:
- Mock only the external boundary (network, API)
- Use simple state toggle (`connected.Store(true/false)`)
- Let all business logic run through real code
- Use function injection for clean test seams
- Provide callback hooks for async sync

### `/testing-go-backend-async` - Async Synchronization
**When:** Testing code with callbacks, subscriptions, or async events

Key principles:
- Use `sync.WaitGroup` with precise counters
- Never use `time.Sleep` or `require.Eventually`
- Calculate exact callback counts before test runs
- Callbacks only update state; assert outside after `wg.Wait()`
- Add logs before fixing "unpredictable" behavior

### `/testing-go-backend-e2e` - Consolidated E2E Tests
**When:** Structuring tests, testing state machines, covering full lifecycles

Key principles:
- One comprehensive test > many small disjointed tests
- Test state transitions in sequence (phases)
- Verify ordering across state changes
- Single setup/teardown for full lifecycle

---

## Anti-Patterns Summary

| Anti-Pattern | Correct Approach | Workflow |
|--------------|------------------|----------|
| `time.Sleep()` | `sync.WaitGroup` | `/testing-go-backend-async` |
| `require.Eventually()` | Callback hooks + WaitGroup | `/testing-go-backend-async` |
| Complex mock simulation | Simple `connected` toggle | `/testing-go-backend-mock` |
| Many small tests | Single E2E test | `/testing-go-backend-e2e` |
| Mocking business logic | Mock only boundary | `/testing-go-backend-mock` |

---

## Combined Example

Most tests combine all three workflows:

```go
func TestComponentE2E(t *testing.T) {
    // FROM /testing-go-backend-mock: Simple mock at boundary
    kafka := newMockKafka()
    
    // FROM /testing-go-backend-async: WaitGroup for callback sync
    var wg sync.WaitGroup
    kafka.setOnSend(func(msg Message) { wg.Done() })
    
    // Real server, real storage, real subscriptions
    server := newE2EServer(t)
    component := NewComponent(Config{
        Server:   server,
        SendFunc: kafka.send,
    })
    
    // FROM /testing-go-backend-e2e: Phase-based structure
    
    // Phase 1: Connected
    wg.Add(2)
    component.Send("msg1")
    component.Send("msg2")
    wg.Wait()
    require.Len(t, kafka.getMessages(), 2)
    
    // Phase 2: Disconnected
    kafka.connected.Store(false)
    component.Send("msg3")
    require.Equal(t, 1, component.QueueLen())
    
    // Phase 3: Recovery
    wg.Add(1)
    kafka.connected.Store(true)
    wg.Wait()
    require.Equal(t, 0, component.QueueLen())
}
```

---

## See Also

- `/testing-go-backend-mock` - What and how to mock
- `/testing-go-backend-async` - WaitGroup synchronization patterns
- `/testing-go-backend-e2e` - E2E test structure and phases

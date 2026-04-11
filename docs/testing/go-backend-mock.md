---
description: Mock testing - minimal mocking at boundaries, simple state toggles
category: testing
triggers:
  - mock
  - mockKafka
  - mockSender
  - SendFunc
  - injection
  - seam
  - testability
  - fake
  - stub
  - boundary
  - what should I mock
  - mocking too much
  - test real behavior
  - how to structure mocks
  - external dependency in test
when: Deciding what to mock, how to structure mocks, avoiding over-mocking
related:
  - testing/index
  - testing/go-backend-async
  - testing/go-backend-e2e
---

# Mock Testing Patterns

This workflow covers how to mock external dependencies while maximizing real code execution. The goal is to test actual behavior, not mock behavior.

---

## Core Principle: Mock Only the Boundary

**Mock the smallest possible surface area** - typically the external system boundary (network, filesystem, external API).

### ❌ Wrong: Over-mocking with Complex Simulation

```go
// DON'T: Mock internal state with complex simulation
type mockSender struct {
    attempts    atomic.Int32
    failUntil   atomic.Int32   // complex failure simulation
    onAttempt   func(attempt int, success bool)
}

func (m *mockSender) send(...) bool {
    attempt := int(m.attempts.Add(1))
    return attempt > int(m.failUntil.Load())  // simulating behavior
}

// Test manipulates internal mock state
mock.setFailUntil(3)  // "fail first 3 attempts"
```

**Why it's wrong:**
- You're testing your mock's simulation logic, not the real code
- Mock complexity grows to mirror production code
- Tests pass but production fails due to simulation gaps

### ✅ Correct: Simple State Toggle

```go
// DO: Simple connected/disconnected toggle at the boundary
type mockKafka struct {
    mu        sync.Mutex
    connected atomic.Bool
    messages  []Message
    onSend    func(msg Message)
}

func (m *mockKafka) send(kafkaURL, topic, payload string) bool {
    if !m.connected.Load() {
        return false  // simple: connected or not
    }
    m.mu.Lock()
    msg := Message{Topic: topic, Payload: payload}
    m.messages = append(m.messages, msg)
    cb := m.onSend
    m.mu.Unlock()
    if cb != nil {
        cb(msg)
    }
    return true
}

// Test toggles simple state
kafka.connected.Store(false)  // "kafka is down"
kafka.connected.Store(true)   // "kafka is back"
```

**Why it's right:**
- All queue logic, persistence, status updates run through real code
- Only the actual network call is mocked
- Test verifies real behavior under real conditions

---

## The Injection Pattern

Create a clean seam at the external boundary using function injection:

### Production Code

```go
// Define the function signature
type SendFunc func(kafkaURL, topic, payload string) bool

// Default implementation calls real external system
func defaultSendToKafka(kafkaURL, topic, payload string) bool {
    conn, err := kafka.DialLeader(...)
    // ... real Kafka code
}

// Worker accepts injectable function
type WorkerConfig struct {
    SendFunc SendFunc  // nil = use default
}

func NewWorker(cfg WorkerConfig) *Worker {
    sendFunc := cfg.SendFunc
    if sendFunc == nil {
        sendFunc = defaultSendToKafka
    }
    return &Worker{sendFunc: sendFunc}
}
```

### Test Code

```go
func TestWorker(t *testing.T) {
    kafka := newMockKafka()
    
    w := NewWorker(WorkerConfig{
        SendFunc: kafka.send,  // inject mock
    })
    
    // All Worker logic runs real code
    // Only the final send() is mocked
}
```

---

## What Should Run Real vs Mocked

| Component | Real or Mock | Reason |
|-----------|--------------|--------|
| Business logic | **Real** | This is what you're testing |
| State management | **Real** | Critical behavior |
| Storage | **Real** | Use test server with temp dir |
| WebSocket subscriptions | **Real** | Test actual event flow |
| Status updates | **Real** | Verify state transitions |
| External network calls | **Mock** | Boundary to external system |
| External APIs | **Mock** | Boundary to external system |

---

## Mock Structure Template

```go
type mock[External] struct {
    mu        sync.Mutex
    connected atomic.Bool      // simple on/off state
    records   []Record         // track what was "sent"
    onSend    func(r Record)   // callback for WaitGroup sync
}

func newMock[External]() *mock[External] {
    m := &mock[External]{}
    m.connected.Store(true)  // default: connected
    return m
}

func (m *mock[External]) send(...) bool {
    if !m.connected.Load() {
        return false
    }
    m.mu.Lock()
    record := Record{...}
    m.records = append(m.records, record)
    cb := m.onSend
    m.mu.Unlock()
    if cb != nil {
        cb(record)
    }
    return true
}

func (m *mock[External]) getRecords() []Record {
    m.mu.Lock()
    defer m.mu.Unlock()
    result := make([]Record, len(m.records))
    copy(result, m.records)
    return result
}

func (m *mock[External]) setOnSend(cb func(r Record)) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.onSend = cb
}
```

---

## Callback Hooks for Async Sync

Mocks should provide callback hooks for deterministic test synchronization:

```go
type mockKafka struct {
    onSend func(msg Message)  // fires after each successful send
}

// In test: connect callback to WaitGroup
var wg sync.WaitGroup
kafka.setOnSend(func(msg Message) {
    wg.Done()
})

wg.Add(3)
// trigger 3 sends...
wg.Wait()  // deterministic sync
```

See `/testing-go-backend-async` (`testing/go-backend-async`) for full WaitGroup patterns.

---

## Checklist Before Writing a Mock

1. [ ] Identified the external boundary (what actually needs mocking)
2. [ ] Mock uses simple state toggle, not behavior simulation
3. [ ] All business logic runs through real code paths
4. [ ] Storage uses real server with temp directory
5. [ ] Mock provides callback hooks for async sync
6. [ ] Mock tracks records for assertions

---

## Related Workflows

- `/testing` - Testing workflow index
- `/testing-go-backend-async` (`testing/go-backend-async`) - WaitGroup patterns for async synchronization
- `/testing-go-backend-e2e` (`testing/go-backend-e2e`) - Consolidated E2E test patterns

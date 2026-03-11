---
description: General principles for working with asynchronous events in any language
category: principles
triggers:
  - async events
  - asynchronous
  - event-driven
  - synchronization
  - race condition
  - timing
  - sleep
  - polling
  - callback
  - event ordering
when: Working with async events, callbacks, subscriptions, event-driven architectures, or debugging race conditions
related:
  - testing-go-backend-async
  - _truthseeker
---

# Async Events: General Principles

Universal principles for working with asynchronous events. These apply regardless of language, framework, or whether you're writing tests or production code.

---

## 1. Never Use Time as a Synchronization Mechanism

Time-based synchronization (sleep, delays, timeouts-as-sync) is **always wrong** as a coordination strategy. This is the single most common source of async bugs.

### Why it fails

- **Environment variance**: Execution speed varies across machines, load conditions, CI vs local, debug vs release. A sleep that "works" on your machine fails elsewhere.
- **Compounding fragility**: Each sleep adds a point where assumptions about speed can break. Two sleeps in sequence multiply the failure probability.
- **Hidden coupling**: A sleep encodes an implicit contract — "this will finish in X ms." That contract is invisible, unversioned, and untested. When the upstream changes speed, nothing warns you.
- **Masking root causes**: If you need a sleep, it means you don't know when the operation completes. The sleep hides that ignorance instead of fixing it.
- **Resource waste**: Sleep either waits too long (slow) or not long enough (broken). There is no correct duration — the concept itself is flawed.

### What counts as time-based sync

- `sleep()` / `time.Sleep()` / `Thread.sleep()` / `setTimeout()` used to wait for a result
- Polling with fixed intervals (`while (!ready) { sleep(10ms) }`)
- Arbitrary timeouts used for correctness rather than failure detection
- `require.Eventually` / retry loops where the "eventually" hides the real sync point
- Debounce/throttle used as synchronization (they are flow control, not sync)

### The fix: explicit signaling

The event source **must tell** the consumer when it's done. The mechanism varies by language:

| Language | Mechanism |
|----------|-----------|
| Go | `sync.WaitGroup`, channels, `context.Done()` |
| JavaScript | `Promise`/`async-await`, event emitters, `AbortController` |
| Python | `asyncio.Event`, `threading.Event`, `Future` |
| Rust | `tokio::sync::Notify`, channels, `JoinHandle` |
| General | Callbacks, condition variables, semaphores |

The principle is the same everywhere: **the producer signals the consumer**. If you cannot add a signal, you don't control the boundary well enough — fix the boundary, don't add a sleep.

### Timeouts are for liveness, not correctness

Timeouts have a legitimate use: **detecting that something failed to respond**. A timeout says "if I haven't heard back in 5s, something is broken — abort." This is fundamentally different from "wait 5s because it should be done by then."

- **Correct**: `context.WithTimeout` to abort a hanging RPC
- **Wrong**: `time.Sleep(5 * time.Second)` to wait for a write to propagate

---

## 2. Don't Assume Behavior — Prove It

When an async system behaves unexpectedly, the instinct is to guess: "it's probably a timing issue," "maybe events arrive out of order," "the callback must fire twice."

**These are hypotheses, not facts.** Before acting on them:
- Add logging/tracing at every event boundary
- Record the actual order, count, and timing of events
- Compare observed behavior against expected behavior
- Only then form a conclusion

"Unpredictable" is never a valid characterization — it means you haven't observed enough.

---

## 3. Calculate Expected Events Before Execution

Before triggering async operations, know **exactly** how many events you expect and in what order.

- Count the events that each operation produces
- Account for initialization events (e.g., initial state on subscription)
- Account for fan-out (N subscribers × M events = N×M callbacks)
- Document these counts explicitly in your code

If you can't calculate the expected count, you don't understand the system well enough to work with it safely.

---

## 4. Separate Event Handling from Event Verification

Callbacks and event handlers should do **minimal work**: capture state, record data, signal completion.

**Don't** mix verification logic (assertions, validation, branching decisions) into event handlers. This creates:
- Race conditions between handler execution and verification timing
- Complex control flow that's hard to reason about
- Coupling between "what happened" and "was it correct"

**Pattern:** Handle → Signal → Verify (in that order, with explicit synchronization between each step).

---

## 5. Add Observability Before Fixing

When async behavior seems wrong, the first step is **always** to add observability — never to change logic.

**Before any fix:**
1. Log every event arrival (what, when, from where)
2. Log every signal/notification sent
3. Log the state at each transition point
4. Run and read the actual output

**If you're on your 3rd attempted fix without logs, you're guessing.** Stop and instrument first.

---

## 6. Treat Every Event Boundary as Untrusted

At every boundary where events cross (network, thread, process, callback), assume:
- Events may arrive in any order (unless explicitly guaranteed)
- Events may arrive more times than expected
- Events may not arrive at all
- The time between send and receive is unbounded

Design your handlers to be **idempotent** where possible, and always have explicit handling for unexpected event counts or ordering.

---

## 7. Don't Share Mutable State Across Async Boundaries

When multiple async operations access the same data without coordination, every possible interleaving is a potential bug.

- **Shared mutable state is the root of most race conditions.** If two callbacks can write to the same variable, one will eventually overwrite the other's result.
- Prefer message passing over shared state. Send data to a single owner rather than sharing it.
- When sharing is unavoidable, use proper synchronization primitives (mutexes, atomics, synchronized collections) — never rely on "it's fast enough that it won't overlap."
- Immutable data is always safe to share. If you can make it immutable, do.

---

## 8. Propagate Errors Across Async Chains

In synchronous code, errors bubble up the call stack naturally. In async code, errors get swallowed silently unless you explicitly propagate them.

- **Every async operation can fail.** If you don't handle the failure, it disappears.
- Unhandled promise rejections, ignored callback errors, and fire-and-forget goroutines are all the same bug: a failure that nobody notices until it's too late.
- Design error paths as carefully as success paths. Every callback needs an error case. Every promise needs a `.catch`. Every goroutine needs a way to report failure.
- When an async chain has multiple steps, a failure in step 2 must prevent step 3 — don't let later steps run on stale or invalid data.

---

## 9. Design for Cancellation and Cleanup

Async operations outlive the scope that started them. If the initiator goes away (user navigates, request times out, component unmounts), the operation keeps running — consuming resources and potentially mutating state that nobody cares about anymore.

- **Every long-lived async operation needs a cancellation mechanism**: `context.Cancel()`, `AbortController.abort()`, cancellation tokens, unsubscribe callbacks.
- Cancellation must propagate: if a parent operation is cancelled, all child operations must also cancel.
- Always clean up resources when an async operation ends — whether by success, failure, or cancellation. Subscriptions must be unsubscribed. Connections must be closed. Timers must be cleared.

---

## 10. Use State Machines for Complex Async Flows

When an entity moves through multiple async states (connecting → connected → syncing → ready → disconnecting), represent it as an explicit state machine rather than a collection of boolean flags.

- Boolean flags create an exponential number of implicit states, most of which are invalid (`isConnected && isDisconnecting && !isReady` — is this legal?).
- A state machine makes transitions explicit: from state X, only transitions Y and Z are valid.
- Invalid transitions become obvious (and can be logged/rejected) instead of silently creating impossible states.
- Every async event maps to a state transition. If an event arrives in a state where it makes no sense, you have a clear bug to investigate.

---

## Summary

| Principle | Anti-pattern | Correct approach |
|-----------|-------------|-----------------|
| No time-based sync | `sleep(100ms)` then check | Explicit signal from producer |
| Prove, don't assume | "Probably a timing issue" | Add logs, observe, then conclude |
| Pre-calculate events | "Wait and see how many arrive" | Calculate exact count upfront |
| Separate handling/verification | Assert inside callbacks | Handle → Signal → Verify |
| Observe before fixing | Change code, hope it works | Instrument first, fix second |
| Untrusted boundaries | Assume happy path | Handle unexpected counts/order |
| No shared mutable state | Two callbacks write same var | Message passing or synchronization |
| Propagate errors | Ignore callback/promise errors | Every async op has an error path |
| Design for cancellation | Fire and forget | Context/abort + cleanup on end |
| State machines | Boolean flag soup | Explicit states and transitions |

---

## Related

- `testing-go-backend-async` — Go-specific implementation of these principles using `sync.WaitGroup`
- `_truthseeker` — "Prove don't assume" as a foundational principle

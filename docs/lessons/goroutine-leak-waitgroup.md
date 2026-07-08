---
description: Verified lesson — a spawned goroutine that a function never waits on outlives the call and leaks; Wait on the WaitGroup (or bound the goroutine to a cancellable context) before returning.
triggers:
  - goroutine leak
  - WaitGroup
  - leaked goroutine
  - go func not awaited
  - background goroutine outlives caller
when: Retrieved via kb_search when reviewing or writing code that spawns goroutines, or diagnosing a goroutine leak.
related:
  - core/memory
  - flows/project/code
---

# Lesson — Wait on goroutines before returning

A goroutine spawned with `go func()` that the enclosing function never joins **outlives the call** and
leaks: it keeps running (and holding its captured references) after the caller has returned. This is a
recurring, cross-project failure mode, not a one-repo quirk.

- **Symptom.** Goroutine count grows over the process lifetime; `pprof` goroutine profiles show many
  copies of the same stack blocked on a channel send/receive or a `sync` primitive.
- **Fix.** Join every spawned goroutine before returning — `wg.Add(1)` / `defer wg.Done()` in the
  goroutine and `wg.Wait()` before the function returns — **or** bind the goroutine's lifetime to a
  cancellable `context.Context` the caller controls, and return only after cancel + drain.
- **Review lens.** For each `go func`, ask: *who waits on it, and on what path does the wait run?* An
  unbounded, unjoined goroutine whose only exit is a blocking channel op is a leak.

> This file is a worked example of the `docs/lessons/` format. It is a real, applicable lesson and it is
> intentionally shippable — it is retrievable via `kb_search` like any other doc.

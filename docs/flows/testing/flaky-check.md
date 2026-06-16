---
description: Flaky test detection - manually-driven batches of -race runs that PRESERVE full failure logs, reproduce faithfully, and account for cross-test/ordering flakes
triggers:
  - flaky
  - flaky test
  - flaky-check
  - intermittent failure
  - test passes sometimes
  - test fails sometimes
  - rare failure
  - reproduce flake
  - heisenbug
  - non-deterministic test
  - test timed out intermittently
when: A test fails intermittently or rarely (in CI or locally) and you need to reproduce it, capture its full diagnostics, and fix the root cause
related:
  - flows/testing/testing
  - flows/testing/testing-go-backend-async
  - flows/principles/truthseeker
---

# Flaky Test Detection

Run a test suite under the race detector in batches you drive **one at a time** —
reading each batch's result before launching the next — to surface
non-deterministic failures, **capture the full diagnostics of any failure**, then
root-cause and fix it.

> ## ⛔ THE CARDINAL RULE: NEVER DISCARD A FAILURE'S OUTPUT
>
> A flake may take hundreds of iterations and a long time to hit. When it does,
> its **complete** output (`--- FAIL` lines, assertion messages, `panic` stack,
> `WARNING: DATA RACE` blocks, full goroutine dump on timeout) is the entire
> payoff. If you lose it you have to reproduce from scratch — often many more
> minutes or hours.
>
> Therefore, **every batch writes its full combined output to a file**, and on
> failure that file is preserved verbatim. Do **NOT**:
> - pipe through `| tail -N` or `| head -N` as the only record (truncates the
>   real failure),
> - `grep`-filter as the only record (a pattern like `FAIL` also matches benign
>   lines such as `failed to get from storage` and *drops* the real `--- FAIL`),
> - capture into a shell variable that dies with the process,
> - keep only a summary.
>
> Save the raw bytes first; analyze second.

---

## What a flake actually is (don't tunnel on one test)

The test the runner names as failed is **not necessarily the buggy one**. Flakes
commonly come from *interaction between tests*, not a single test in isolation:

- **Shared/global state** — a package var, registry, singleton, or `monotonic`
  clock left dirty by an earlier test.
- **Goroutine / resource leaks** — a server, watcher, or ticker from test A still
  running when test B starts, racing it or exhausting ports/FDs.
- **Ordering & parallelism** — `t.Parallel()` interleavings, or order-dependence
  that only appears at a particular `-count`/`-shuffle` seed.
- **Cross-iteration accumulation** — under `-count=N` the same process reruns
  every test N times; state that survives between reruns can trip the Nth run
  when a single run is always clean.
- **Environmental** — OOM/host crash under sustained `-race` load, port/TIME_WAIT
  exhaustion, disk fill. These look like test failures but are not test bugs.

Because of this: **reproduce with the whole suite, the same way it failed** (same
`-count`, `-failfast`, `-race`, `-timeout`, and ideally the same `-shuffle`
seed) — not a narrowed `-run` of the one named test. Narrowing to the named test
is the fastest way to "fail to reproduce" an overlap flake.

---

## Procedure

### 1. Pin the failure conditions
If a specific run failed (e.g. CI), copy its **exact** invocation — `-count`,
`-failfast`, `-race`, `-timeout`, `-shuffle`, `-p`/`GOMAXPROCS`, the package set.
Reproduce under those. CI is usually `go test -race -count=1 -failfast -timeout 60s ./...`;
a batch reproducer raises `-count` to pack more iterations per process (which also
exercises cross-iteration accumulation).

### 2. Baseline the duration
```bash
go test -race -count=1 -timeout 600s ./... 2>&1 | tail -1   # note the "ok ... Ns"
```
Per-batch timeout = `baseline × count × 1.5`, floor 60s. Set it generously: a
timeout that fires on a slow-but-healthy run destroys the signal.

### 3. Run batches one at a time — manually, never in an unattended loop
Batching only earns its keep if **you run one batch, read its result, then
decide** whether to run the next. Do **not** wrap the batches in a shell loop
(`for b in $(seq 1 100); do go test ...; done`): an unattended sweep is just
`-count=(batches × count)` with extra steps — it surrenders the per-batch
judgment that is the entire point (reading each result as it lands, pacing a
fragile host, stopping the instant something looks wrong) and tends to bury the
failure mid-stream. Drive the batches yourself, one invocation each.

Pick a **persistent** artifact dir that survives host restarts — a gitignored
dir inside the repo (e.g. `.flaky/`), **not** `/tmp` (cleared on reboot). Each
batch writes its complete combined output to its own file; on failure, keep that
exact file and stop.

```bash
mkdir -p .flaky
COUNT=10 TO=300s                 # per-batch iterations and timeout (from steps 1-2)
b=1                              # ← bump this yourself before each new batch
out=".flaky/batch-$b.log"
go test -race -count=$COUNT -failfast -timeout $TO ./... > "$out" 2>&1; echo "rc=$?"
# If your suite leaves scratch between runs, clean ONLY that dir here — uncomment
# and point it at the real scratch path, never a source dir like test/:
# rm -rf .scratch/* 2>/dev/null
tail -1 "$out"                   # quick health line; the full log is in $out
```

After each batch:
- **rc = 0** → note progress (`batch $b OK, ~$((b*COUNT)) cumulative iters`),
  increment `b`, and run the next batch. Pace between batches on constrained
  hosts (see Pacing).
- **rc ≠ 0** → **stop.** Leave `.flaky/batch-$b.log` untouched and go to step 4.

Keep the running tally yourself — batch N ≈ N × COUNT cumulative iterations — so
you can state the real number reached. Keep `-v` **off** for the batch run (it
floods output and can bury the failure); the saved full log already has
everything, and you can re-run the one failing seed with `-v` later.

### 4. On failure — read the FULL artifact, find the REAL marker
Open the preserved `.flaky/batch-N.log` and locate the actual failure — not the
first line that contains "fail":
```bash
grep -nE '^--- FAIL|^=== FAIL|^panic:|^fatal error:|^FAIL\b|test timed out|WARNING: DATA RACE|Error Trace:' .flaky/batch-N.log
```
- `--- FAIL: TestX` → which test the runner blamed (read its assertion message).
- `WARNING: DATA RACE` → read both stacks; the racing writers/readers name the
  real culprit (often production code or a leaked goroutine from another test).
- `panic: test timed out` → read the goroutine dump: the `[running]`/`chan
  receive`/`sync.*` stacks show what hung. A `(48s)` next to a test name is the
  one that blew the budget — but check whether it's waiting on something another
  test/goroutine should have signalled.

### 5. Root-cause, classify, then fix
Per `flows/principles/truthseeker`: prove what happened from the captured evidence before
theorizing. Classify:
- **Real test bug** (wrong WaitGroup count, missing sync, order dependence, shared
  state) → fix the test deterministically (see `flows/testing/testing-go-backend-async`).
- **Real production bug** (data race, a genuine hang) → fix production; the flaky
  test earned its keep.
- **Environmental** (host crash, OOM, port exhaustion) → not a test bug; reduce
  load (Pacing) and note it, don't "fix" a test that isn't broken.

After a fix, restart from batch 1 (reset `b=1`) — a fix invalidates prior clean
batches.

### 6. Stop when
- The target iteration count passes clean (e.g. 1000), **or**
- a failure is reproduced and fixed (then re-run to confirm), **or**
- the user stops it.
State the real number reached; never imply more coverage than ran.

### 7. Hand off to /gh
This skill ends at **reproduced → captured → root-caused → fixed → re-verified**.
Hand the verified fix to `/gh` to ship it (it gates the diff with `gh-self-review`,
then opens the PR). Don't commit to the default branch or open a PR yourself.

---

## Pacing (hosts that crash under load)

Sustained `-race` runs are memory-heavy and can crash constrained hosts (WSL
especially). If the host struggles between batches:
- pause briefly before the next batch so each `-race` process fully exits and
  frees memory before the next starts,
- consider a smaller `COUNT` per batch (more, lighter processes),
- do **not** lower `GOMAXPROCS`/`-p` just to survive — that reduces the scheduling
  pressure that surfaces races, defeating the check.

Driving batches by hand makes recovery trivial: if the host dies, just run the
next batch — the persistent `.flaky/` logs preserve every batch that already
passed. A crash resets in-process state, so restart the count from batch 1 —
that's correct (the goal is N *consecutive* clean iterations).

## Relationship to CI

Green CI on a flake-prone suite is necessary but not sufficient — CI runs each
test once. This sweep is the supplementary high-iteration confidence. When a CI
run flakes, reproduce it here with CI's exact flags; when this sweep flakes, the
fix still has to land and go green in CI.

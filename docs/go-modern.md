---
description: Modern Go patterns - auto-fix with gopls modernize after Go edits
category: patterns
triggers:
  - go 1.22
  - go 1.24
  - modern go
  - modernize
  - gopls
  - interface{}
  - any
when: After editing Go files, run gopls modernize -fix to auto-apply modern patterns
related:
  - testing-go-backend-async
---

# Modern Go Patterns

This workflow ensures Go code follows modern idioms by running `gopls modernize -fix` after editing `.go` files.

---

## ⚠️ AUTOMATIC TRIGGER RULE

**After editing any `.go` file, Cascade MUST run gopls modernize with `-fix` to auto-apply fixes:**

```bash
// turbo
go run golang.org/x/tools/gopls/internal/analysis/modernize/cmd/modernize@latest -fix ./path/to/edited/package/...
```

The `-fix` flag automatically applies all suggested modernizations. No manual intervention needed.

---

## Commands

Auto-fix a specific package:
```bash
go run golang.org/x/tools/gopls/internal/analysis/modernize/cmd/modernize@latest -fix ./your-package/...
```

Auto-fix entire codebase:
```bash
go run golang.org/x/tools/gopls/internal/analysis/modernize/cmd/modernize@latest -fix ./...
```

Check only (no changes):
```bash
go run golang.org/x/tools/gopls/internal/analysis/modernize/cmd/modernize@latest ./...
```

---

## Auto-Fixed Patterns

The `-fix` flag automatically handles all of these:

| Pattern | Modernization |
|---------|---------------|
| `interface{}` | → `any` |
| `for i := 0; i < n; i++` | → `for i := range n` |
| `context.WithCancel(context.Background())` in tests | → `t.Context()` (Go 1.24+) |
| `// +build` constraint | → removed (use `//go:build` only) |
| `reflect.TypeOf(x)` | → `reflect.TypeFor[T]()` |
| Loop checking slice contains | → `slices.Contains()` |
| `sort.Slice(s, func...)` | → `slices.Sort(s)` |
| `for range strings.Split()` | → `for range strings.SplitSeq()` |
| `m[k] = v` loop | → `maps.Copy()` |
| `if x < y { return x }` | → `return min(x, y)` |
| `wg.Add(1); go func(){ defer wg.Done() }()` | → `wg.Go(func(){})` (Go 1.25+) |

---

## When Generating Code

Cascade should generate modern Go by default:
- Use `any` instead of `interface{}`
- Use `for range n` instead of 3-clause for loops
- Use `t.Context()` in tests instead of `context.WithCancel`
- Use `slices.Contains` instead of manual loops
- Use `min`/`max` builtins instead of if/else

### Loop Variable Semantics

Go 1.22 fixed the loop variable capture bug - each iteration now has its own variable:

```go
// Now safe in Go 1.22+ (was a bug before)
for i := range n {
    go func() {
        fmt.Println(i) // Each goroutine gets its own i
    }()
}
```

---

## Benchmark Patterns (Go 1.24+)

### Loop-Based Benchmarks

```go
// ❌ Old style
func BenchmarkOld(b *testing.B) {
    for i := 0; i < b.N; i++ {
        doWork()
    }
}

// ✅ Modern (Go 1.24+)
func BenchmarkNew(b *testing.B) {
    for b.Loop() {
        doWork()
    }
}
```

**Benefits of `b.Loop()`:**
- Cleaner syntax
- Framework controls iteration
- Better warm-up handling

---

## Switch Patterns

### Tagged Switch Instead of If-Else Chain

```go
// ❌ If-else chain
if status == "pending" {
    handlePending()
} else if status == "active" {
    handleActive()
} else if status == "completed" {
    handleCompleted()
} else {
    handleUnknown()
}

// ✅ Tagged switch
switch status {
case "pending":
    handlePending()
case "active":
    handleActive()
case "completed":
    handleCompleted()
default:
    handleUnknown()
}
```

### Type Switch

```go
switch v := value.(type) {
case string:
    processString(v)
case int:
    processInt(v)
case []byte:
    processBytes(v)
default:
    return fmt.Errorf("unsupported type: %T", value)
}
```

---

## Slice Patterns

### Clear a Slice (Go 1.21+)

```go
// ❌ Old style
slice = slice[:0]
// or
slice = nil

// ✅ Modern (Go 1.21+)
clear(slice)
```

### Clone a Slice (Go 1.21+)

```go
// ❌ Old style
clone := make([]T, len(original))
copy(clone, original)

// ✅ Modern (Go 1.21+)
clone := slices.Clone(original)
```

---

## Map Patterns

### Clear a Map (Go 1.21+)

```go
// ❌ Old style - iterate and delete
for k := range m {
    delete(m, k)
}

// ✅ Modern (Go 1.21+)
clear(m)
```

### Clone a Map (Go 1.21+)

```go
// ✅ Modern
clone := maps.Clone(original)
```

---

## Error Handling

### Errors.Join (Go 1.20+)

```go
// Combine multiple errors
var errs []error
if err1 != nil {
    errs = append(errs, err1)
}
if err2 != nil {
    errs = append(errs, err2)
}
return errors.Join(errs...)
```

### Wrapping with %w

```go
return fmt.Errorf("failed to process %s: %w", name, err)
```

---

## Context Patterns

### Context with Cause (Go 1.20+)

```go
ctx, cancel := context.WithCancelCause(parent)

// Cancel with specific reason
cancel(errors.New("user requested cancellation"))

// Check cause
if err := context.Cause(ctx); err != nil {
    log.Printf("cancelled because: %v", err)
}
```

### AfterFunc (Go 1.21+)

```go
stop := context.AfterFunc(ctx, func() {
    cleanup()
})
defer stop()
```

---

## Comparison Patterns

### cmp.Or (Go 1.22+)

```go
// ❌ Old style
value := x
if value == "" {
    value = y
}
if value == "" {
    value = "default"
}

// ✅ Modern (Go 1.22+)
value := cmp.Or(x, y, "default")
```

---

## Testing Patterns

### Subtests with Parallel

```go
func TestFeature(t *testing.T) {
    tests := []struct {
        name string
        input string
        want string
    }{
        {"empty", "", ""},
        {"simple", "foo", "FOO"},
    }
    
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            t.Parallel()
            got := Transform(tc.input)
            if got != tc.want {
                t.Errorf("got %q, want %q", got, tc.want)
            }
        })
    }
}
```

---

## Summary Table

| Pattern | Minimum Version |
|---------|-----------------|
| `for i := range n` | Go 1.22 |
| `for range n` | Go 1.22 |
| `for b.Loop()` | Go 1.24 |
| `clear(slice/map)` | Go 1.21 |
| `slices.Clone/maps.Clone` | Go 1.21 |
| `cmp.Or` | Go 1.22 |
| `context.WithCancelCause` | Go 1.20 |
| `context.AfterFunc` | Go 1.21 |
| `errors.Join` | Go 1.20 |

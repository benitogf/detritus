---
description: ko package - persistent LevelDB storage adapter for ooo
category: storage
triggers:
  - ko
  - leveldb
  - embedded storage
  - persistent
  - LayeredConfig
  - storage.New
  - EmbeddedStorage
  - database path
  - db/
when: Setting up LevelDB storage, layered storage configuration, debugging storage issues
related:
  - ooo-package
  - ooo-pivot
---

# ko Package Reference

**Repository:** https://github.com/benitogf/ko

ko provides persistent storage using LevelDB for the ooo ecosystem. It enables data to survive server restarts.

---

## Installation

```bash
go get github.com/benitogf/ko
```

---

## Usage Patterns

### Standalone Persistent Storage

```go
import "github.com/benitogf/ko"

store := &ko.Storage{Path: "/data/myapp"}
err := store.Start([]string{}, nil)
if err != nil {
    log.Fatal(err)
}
defer store.Close()
```

### With ooo Server (Direct)

```go
import (
    "github.com/benitogf/ko"
    "github.com/benitogf/ooo"
)

store := &ko.Storage{Path: "/data/myapp"}
err := store.Start([]string{}, nil)
if err != nil {
    log.Fatal(err)
}

server := ooo.Server{
    Storage: store,
}
server.Start("0.0.0.0:8800")
server.WaitClose()
```

### Layered Storage (Recommended for Production)

Combines in-memory speed with disk persistence:

```go
import (
    "github.com/benitogf/ko"
    "github.com/benitogf/ooo/storage"
)

// Create embedded storage (LevelDB backend)
embedded := &ko.EmbeddedStorage{Path: "db/data"}

// Create layered storage (memory + embedded)
store := storage.New(storage.LayeredConfig{
    Memory:   storage.NewMemoryLayer(),
    Embedded: embedded,
})

err := store.Start(storage.Options{})
if err != nil {
    log.Fatal(err)
}

// Required for proper shutdown
go storage.WatchStorageNoop(store)

// Use with ooo server
server := &ooo.Server{
    Storage: store,
}
```

---

## EmbeddedStorage vs Storage

| Type | Description | Use Case |
|------|-------------|----------|
| `ko.Storage` | Direct LevelDB access | Simple persistent storage |
| `ko.EmbeddedStorage` | Embedded adapter for layered storage | Production with memory layer |

---

## Storage Interface

ko implements the ooo storage interface:

```go
type Database interface {
    Start(options Options) error
    Close()
    Get(path string) (meta.Object, error)
    GetN(path string, limit int) ([]meta.Object, error)
    GetNAscending(path string, limit int) ([]meta.Object, error)
    Set(path string, data []byte) (meta.Object, error)
    Push(path string, data []byte) (string, error)
    Del(path string) error
    Keys(pattern string) ([]string, error)
    Watch(callback func(string))
    WatchWithCallback(callback func(string))
}
```

---

## Common Patterns

### Separate Auth and Data Storage

```go
// Auth storage
authEmbedded := &ko.EmbeddedStorage{Path: *authPath}
authStorage := storage.New(storage.LayeredConfig{
    Memory:   storage.NewMemoryLayer(),
    Embedded: authEmbedded,
})
err := authStorage.Start(storage.Options{})
go storage.WatchStorageNoop(authStorage)

// Data storage  
dataEmbedded := &ko.EmbeddedStorage{Path: *dataPath}
dataStorage := storage.New(storage.LayeredConfig{
    Memory:   storage.NewMemoryLayer(),
    Embedded: dataEmbedded,
})

// Use auth storage for auth, data storage for server
auth := auth.New(auth.NewJwtStore(*key, time.Minute*10), authStorage)
server := &ooo.Server{Storage: dataStorage}
```

---

## Storage Options

```go
store.Start(storage.Options{
    AfterWrite: func(key string) {
        // Called after every write operation
        log.Println("Written:", key)
    },
})
```

---

## Watch Patterns

```go
// No-op watch (required for clean shutdown)
go storage.WatchStorageNoop(store)

// Watch with callback (used by pivot for sync)
go store.WatchWithCallback(func(key string) {
    log.Println("Changed:", key)
})
```

---

## Related Packages

- [ooo](https://github.com/benitogf/ooo) - Core server
- [ooo/storage](https://github.com/benitogf/ooo) - Storage interfaces and layered storage
- [pivot](https://github.com/benitogf/pivot) - Multi-instance sync

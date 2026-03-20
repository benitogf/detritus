---
description: ooo package - core real-time state management system
category: core
triggers:
  - ooo
  - server setup
  - filters
  - ReadObjectFilter
  - WriteFilter
  - AfterWriteFilter
  - OpenFilter
  - LimitFilter
  - ooo.Get
  - ooo.Set
  - ooo.Push
  - ooo.Delete
  - meta.Object
  - WebSocket
  - subscription
  - endpoint
  - CRUD
when: Setting up ooo servers, adding filters, CRUD operations, WebSocket subscriptions, custom endpoints
related:
  - ooo-pivot
  - ooo-auth
  - ooo-nopog
  - ooo-client-js
---

# ooo Package Reference

**Repository:** https://github.com/benitogf/ooo

ooo is the core state management library providing real-time network access with WebSocket subscriptions, REST API, and JSON Patch updates.

## When to Use ooo

- **Application state/settings** that need real-time sync across clients
- **Prototyping** real-time features quickly
- **Small to medium datasets** where speed matters more than scale

For large-scale data storage (millions of records, complex queries), use [nopog](https://github.com/benitogf/nopog) instead.

---

## Ecosystem

| Package | Description |
|---------|-------------|
| [ooo](https://github.com/benitogf/ooo) | Core server - in-memory state with WebSocket/REST API |
| [ko](https://github.com/benitogf/ko) | Persistent storage adapter (LevelDB) |
| [ooo-client](https://github.com/benitogf/ooo-client) | JavaScript client with reconnecting WebSocket |
| [auth](https://github.com/benitogf/auth) | JWT authentication middleware |
| [pivot](https://github.com/benitogf/pivot) | Multi-instance synchronization (AP distributed) |
| [nopog](https://github.com/benitogf/nopog) | PostgreSQL adapter for large-scale storage |

---

## Server Setup

### Basic Server

```go
package main

import "github.com/benitogf/ooo"

func main() {
    server := ooo.Server{}
    server.Start("0.0.0.0:8800")
    server.WaitClose()
}
```

### Production Server with Layered Storage

```go
import (
    "github.com/benitogf/ko"
    "github.com/benitogf/ooo"
    "github.com/benitogf/ooo/storage"
    "github.com/gorilla/mux"
)

// Create layered storage (memory + embedded persistence)
dataEmbedded := &ko.EmbeddedStorage{Path: "db/data"}
dataStorage := storage.New(storage.LayeredConfig{
    Memory:   storage.NewMemoryLayer(),
    Embedded: dataEmbedded,
})

server := &ooo.Server{
    Silence: true,           // Suppress verbose output
    Static:  true,           // Only filtered routes are available
    Storage: dataStorage,
    Router:  mux.NewRouter(),
    OnClose: func() {
        // Cleanup on server close
    },
}

server.Start("0.0.0.0:8800")
server.WaitClose()
```

### Server Options

| Field | Type | Description |
|-------|------|-------------|
| `Silence` | `bool` | Suppress verbose logging |
| `Static` | `bool` | Only routes with filters are available |
| `Storage` | `storage.Database` | Storage backend |
| `Router` | `*mux.Router` | Gorilla mux router |
| `Client` | `*http.Client` | HTTP client for outgoing requests |
| `OnClose` | `func()` | Callback when server closes |
| `NoBroadcastKeys` | `[]string` | Keys that won't broadcast to subscribers |
| `Audit` | `func(*http.Request) bool` | Global request audit (return false = 401) |

---

## I/O Operations (Type-Safe Helpers)

**CRITICAL:** Always use these helper functions instead of direct storage access:

| ✅ Use | ❌ Avoid |
|--------|----------|
| `ooo.Get[T](server, key)` | `server.Storage.Get(key)` |
| `ooo.GetList[T](server, key)` | `server.Storage.GetList(key)` |
| `ooo.Set(server, key, data)` | `server.Storage.Set(key, data)` |
| `ooo.Push(server, key, data)` | Direct storage push |
| `ooo.Delete(server, key)` | `server.Storage.Del(key)` |

The helpers provide type safety, consistent JSON handling, and proper error patterns.

### Get Single Item

```go
item, err := ooo.Get[YourType](server, "path/to/item")
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Item: %+v, Created: %d\n", item.Data, item.Created)
```

### Get List of Items

```go
// Path must end with "/*" for lists
items, err := ooo.GetList[YourType](server, "path/to/items/*")
if err != nil {
    log.Fatal(err)
}
for _, item := range items {
    fmt.Printf("Item: %+v (index: %s)\n", item.Data, item.Index)
}
```

### Set Item

```go
err := ooo.Set(server, "path/to/item", YourType{
    Field1: "value1",
    Field2: "value2",
})
```

### Push to List

```go
// Path must end with "/*" - auto-generates index
index, err := ooo.Push(server, "path/to/items/*", YourType{
    Field1: "new item",
})
fmt.Println("Created at:", index) // e.g., "path/to/items/1234567890"
```

### Delete Item

```go
err := ooo.Delete(server, "path/to/item")
```

### Patch Item (Partial Update)

```go
// Merges with existing data
err := ooo.Patch(server, "path/to/item", PartialUpdate{
    Field1: "updated value",
})
```

---

## Filters

Filters control access and transform data. When `Static=true`, only filtered routes are available.

### OpenFilter (Full CRUD Access)

```go
// Enable read, write, delete for a path
server.OpenFilter("books/*")
server.OpenFilter("settings")
```

### ReadObjectFilter (Single Object)

```go
server.ReadObjectFilter("config", func(index string, data meta.Object) (meta.Object, error) {
    // Transform or validate on read
    return data, nil
})

// Use NoopObjectFilter for read-only access without transformation
server.ReadObjectFilter("status", ooo.NoopObjectFilter)
```

### ReadListFilter (List of Objects)

```go
server.ReadListFilter("items/*", func(index string, items []meta.Object) ([]meta.Object, error) {
    // Filter, sort, or transform list
    return items, nil
})
```

### WriteFilter (Before Write)

```go
server.WriteFilter("books/*", func(index string, data json.RawMessage) (json.RawMessage, error) {
    // Validate or transform before write
    // Return error to deny write
    var book Book
    if err := json.Unmarshal(data, &book); err != nil {
        return nil, err
    }
    if book.Title == "" {
        return nil, errors.New("title required")
    }
    return data, nil
})
```

### AfterWriteFilter (After Write Callback)

```go
server.AfterWriteFilter("books/*", func(index string) {
    log.Println("wrote:", index)
    // Trigger side effects, notifications, etc.
})
```

### DeleteFilter

```go
server.DeleteFilter("books/protected", func(key string) error {
    return errors.New("cannot delete protected books")
})
```

### LimitFilter (Auto-Cleanup Old Entries)

```go
// Keep only the N most recent entries
server.LimitFilter("logs/*", ooo.LimitFilterConfig{
    Limit: 100,
    Order: ooo.OrderDesc, // Most recent first (default)
})

// Dynamic limit based on runtime state
server.LimitFilter("games/*", ooo.LimitFilterConfig{
    LimitFunc: func() int {
        device, err := getDevice()
        if err == nil && device.Cap > 100 {
            return device.Cap
        }
        return 100
    },
    Order: ooo.OrderAsc, // Oldest first
})
```

### Filter Configuration Options

```go
server.WriteFilter("items/*", myFilter, ooo.FilterConfig{
    Description: "Validates item data",
    Schema:      ItemSchema{}, // For UI display
})
```

---

## Custom Endpoints

Register custom HTTP endpoints with typed schemas visible in the UI.

```go
server.Endpoint(ooo.EndpointConfig{
    Path:        "/policies/{id}",
    Description: "Manage access control policies",
    Vars:        ooo.Vars{"id": "Policy ID"},
    Methods: ooo.Methods{
        "GET": ooo.MethodSpec{
            Response: PolicyResponse{},
            Params:   ooo.Params{"filter": "Optional filter value"},
        },
        "PUT": ooo.MethodSpec{
            Request:  Policy{},
            Response: PolicyResponse{},
        },
    },
    Handler: func(w http.ResponseWriter, r *http.Request) {
        id := mux.Vars(r)["id"]
        filter := r.URL.Query().Get("filter")
        // Handle request...
    },
})
```

---

## WebSocket Subscriptions (Go Client)

### Subscribe to Single Object

```go
import "github.com/benitogf/ooo/client"

ctx, cancel := context.WithCancel(context.Background())
defer cancel()

serverCfg := client.Server{Protocol: "ws", Host: "localhost:8800"}
cfg := client.SubscribeConfig{Ctx: ctx, Server: serverCfg}

go client.Subscribe(cfg, "config", client.SubscribeEvents[Config]{
    OnMessage: func(item client.Meta[Config]) {
        fmt.Printf("Config updated: %+v\n", item.Data)
    },
    OnError: func(err error) {
        log.Printf("Error: %v\n", err)
    },
})
```

### Subscribe to List

```go
go client.SubscribeList(cfg, "items/*", client.SubscribeListEvents[Item]{
    OnMessage: func(items []client.Meta[Item]) {
        for _, item := range items {
            fmt.Printf("Item: %+v (created: %d)\n", item.Data, item.Created)
        }
    },
    OnError: func(err error) {
        log.Printf("Error: %v\n", err)
    },
})
```

---

## Remote Operations (HTTP Client)

Perform operations on remote ooo servers via HTTP.

**Import:** `import "github.com/benitogf/ooo/io"`

### RemoteConfig

```go
cfg := io.RemoteConfig{
    Host:   "localhost:8800",        // REQUIRED: host:port
    Client: httpClient,              // Optional: *http.Client (default: http.DefaultClient)
    SSL:    false,                   // Optional: use https (default: false)
    Header: http.Header{},           // Optional: custom headers (e.g., auth)
    Retry:  io.RetryConfig{          // Optional: retry configuration
        MaxRetries: 3,               // Default: 0 (no retries)
        RetryDelay: 100*time.Millisecond, // Initial delay, doubles each retry
    },
    MaxResponseSize: 10 * 1024 * 1024, // Optional: max response size (default: 10MB)
}
```

**Note:** If `Client` is nil, `http.DefaultClient` is used automatically.

### Remote Operations

```go
// RemoteGet - single object (path must NOT be glob)
item, err := io.RemoteGet[YourType](cfg, "path/to/item")

// RemoteGetList - list of objects (path must be glob ending in /*)
items, err := io.RemoteGetList[YourType](cfg, "path/to/items/*")

// RemoteSet - create/update single object (path must NOT be glob)
err := io.RemoteSet(cfg, "path/to/item", YourType{Field: "value"})

// RemotePush - add to list (path must be glob ending in /*)
err := io.RemotePush(cfg, "path/to/items/*", YourType{Field: "new"})

// RemotePatch - partial update (merges with existing)
err := io.RemotePatch(cfg, "path/to/item", PartialType{Field: "updated"})

// RemoteDelete - delete object
err := io.RemoteDelete(cfg, "path/to/item")

// With response (returns index)
resp, err := io.RemoteSetWithResponse(cfg, "path", data)
fmt.Println(resp.Index) // "path"

resp, err := io.RemotePushWithResponse(cfg, "items/*", data)
fmt.Println(resp.Index) // "items/1234567890"
```

### Context Support

All operations have `*WithContext` variants for cancellation:

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

item, err := io.RemoteGetWithContext[T](ctx, cfg, "path")
err := io.RemoteSetWithContext(ctx, cfg, "path", data)
```

### Error Types

| Error | Cause |
|-------|-------|
| `io.ErrHostRequired` | `RemoteConfig.Host` is empty |
| `io.ErrPathGlobRequired` | Used non-glob path with `RemotePush`/`RemoteGetList` |
| `io.ErrPathGlobNotAllowed` | Used glob path with `RemoteGet`/`RemoteSet`/`RemotePatch` |
| `io.ErrEmptyKey` | GET returned 404 (key doesn't exist) |
| `io.ErrRequestFailed` | HTTP request returned 4xx/5xx |

---

## meta.Object Structure

The core data wrapper used by ooo:

```go
type Object struct {
    Created int64           `json:"created"` // Monotonic timestamp (nanoseconds)
    Updated int64           `json:"updated"` // Last update timestamp
    Index   string          `json:"index"`   // Full path (e.g., "items/123")
    Path    string          `json:"path"`    // Optional path info
    Data    json.RawMessage `json:"data"`    // Your actual data
}
```

### Sorting

```go
import "github.com/benitogf/ooo/meta"

// Sort by created descending (most recent first)
sort.Slice(objects, meta.SortDesc(objects))

// Sort by created ascending (oldest first)
sort.Slice(objects, meta.SortAsc(objects))
```

---

## Pivot (Multi-Instance Sync)

Pivot enables AP (Available + Partition-tolerant) distributed synchronization.

### Cluster Leader Setup

```go
import "github.com/benitogf/pivot"

config := pivot.Config{
    Keys: []pivot.Key{
        {Path: "users/*", Database: authStorage}, // External storage
        {Path: "settings"},                        // nil = server.Storage
    },
    NodesKey:   "devices/*", // Node discovery path (entries need "ip" field)
    ClusterURL: "",          // Empty = cluster leader
}

pivot.Setup(server, config)
pivot.GetInstance(server).Attach(authStorage) // Attach external storage
server.Start("0.0.0.0:8800")
```

### Node (Follower) Setup

```go
config := pivot.Config{
    Keys: []pivot.Key{
        {Path: "users/*"},
        {Path: "settings"},
    },
    NodesKey:   "devices/*",
    ClusterURL: "192.168.1.100:8800", // Cluster leader address
}

pivot.Setup(server, config)
server.Start("0.0.0.0:8801")
```

### Node Discovery

Nodes register themselves by creating entries with `IP` and `Port` fields:

```go
ooo.Push(server, "devices/*", Device{
    IP:   "192.168.1.101",
    Port: 8801,
    // ... other fields
})
```

---

## API Reference

### REST Endpoints

| Method | URL | Description |
|--------|-----|-------------|
| `GET` | `/` | Web UI |
| `GET` | `/?api=keys` | List all keys (paginated) |
| `GET` | `/?api=info` | Server info |
| `GET` | `/?api=filters` | List filters |
| `GET` | `/?api=state` | Connection state |
| `GET` | `/{key}` | Read single object or list |
| `POST` | `/{key}` | Create/Update |
| `PATCH` | `/{key}` | Partial update (JSON Patch) |
| `DELETE` | `/{key}` | Delete |

### WebSocket

| URL | Description |
|-----|-------------|
| `ws://{host}:{port}` | Server clock |
| `ws://{host}:{port}/{key}` | Subscribe to path |

### Keys API Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `page` | Page number (1-indexed) | 1 |
| `limit` | Items per page (max 500) | 50 |
| `filter` | Prefix or glob pattern | none |

---

## Storage Interface

### Layered Storage (Recommended)

```go
import (
    "github.com/benitogf/ko"
    "github.com/benitogf/ooo/storage"
)

embedded := &ko.EmbeddedStorage{Path: "db/data"}
store := storage.New(storage.LayeredConfig{
    Memory:   storage.NewMemoryLayer(),
    Embedded: embedded,
})

err := store.Start(storage.Options{})
if err != nil {
    log.Fatal(err)
}
```

### Storage Options

```go
store.Start(storage.Options{
    AfterWrite: func(key string) {
        // Called after every write
    },
})
```

### Watch for Changes

```go
// Watch with callback (used by pivot)
go store.WatchWithCallback(func(key string) {
    log.Println("Changed:", key)
})

// No-op watch (required for proper shutdown)
go storage.WatchStorageNoop(store)
```

---

## Common Patterns

### Subscription Handler Pattern

```go
func (h *Handler) subscribeTo(itemID string) {
    serverCfg := client.Server{Protocol: "ws", Host: h.serverURL}
    cfg := client.SubscribeConfig{Ctx: context.Background(), Server: serverCfg}
    
    client.Subscribe(cfg, "items/"+itemID, client.SubscribeEvents[Item]{
        OnMessage: func(m client.Meta[Item]) {
            h.handleUpdate(m.Data)
        },
        OnError: func(err error) {
            log.Printf("Subscription error: %v", err)
        },
    })
}
```

---

## Noop Filters Reference

| Filter | Usage |
|--------|-------|
| `ooo.NoopFilter` | Write filter that accepts all writes |
| `ooo.NoopObjectFilter` | Read filter for single objects (pass-through) |
| `ooo.NoopListFilter` | Read filter for lists (pass-through) |
| `ooo.NoopHook` | Delete filter that allows all deletes |
| `ooo.NoopNotify` | AfterWrite callback that does nothing |

---

## Key Path Patterns

### Path Types

- **Single object:** `settings`, `config`, `devices/123`
- **Glob list:** `devices/*`, `games/*`
- **Multi-glob (hierarchical):** `items/*/*/*`

> **`*` matches exactly ONE path segment.** It does NOT match across `/` separators.

### Multi-Glob Patterns

Filters and pivot config support multiple `*` segments for hierarchical data.
Each `*` represents one variable level in your path hierarchy.

When you say `"items/{category}/{subcategory}/{itemID}"`, implement it as:

```go
// Register the filter with one * per variable segment
server.OpenFilter("items/*/*/*")

// Pivot sync config (same pattern)
pivot.Config{
    Keys: []pivot.Key{
        {Path: "items/*/*/*"},
    },
}
```

**Writing:** POST to the parent glob, filling in the known segments:

```go
// Create an item under electronics/phones
ooo.Push(server, "items/electronics/phones/*", item)
// Stores at: items/electronics/phones/{autoID}

// Create an item under electronics/laptops
ooo.Push(server, "items/electronics/laptops/*", item)
// Stores at: items/electronics/laptops/{autoID}
```

**Reading:** GET at the same depth you wrote to:

```go
// List all items under electronics/phones
items, _ := ooo.GetList[Item](server, "items/electronics/phones/*")

// Get a specific item by ID
item, _ := ooo.Get[Item](server, "items/electronics/phones/abc123")
```

**WebSocket:** Subscribe at any concrete sub-path:

```go
client.SubscribeList(cfg, "items/electronics/phones/*", events)
```

> **Important:** You cannot read at intermediate depths. `items/electronics/*` will NOT return items stored at `items/electronics/phones/{id}` because the slash count differs. If you need reads at multiple depths, register separate filters for each level.

### REST Key Validation

REST API request keys are stricter than filter registration:

| Context | Multi-glob | Example |
|---------|------------|----------|
| **Filter registration** (`OpenFilter`, etc.) | ✅ Allowed | `server.OpenFilter("items/*/*/*")` |
| **Pivot config** (`pivot.Key.Path`) | ✅ Allowed | `{Path: "items/*/*/*"}` |
| **REST request key** (HTTP POST/GET) | ❌ Single trailing `*` only | POST to `items/cat/sub/*` |
| **`key.Match`** (internal matching) | ✅ Allowed | Matches `items/*/*/*` against `items/a/b/c` |

---

## Common Pitfalls

### Confusing Filter Paths with REST Keys

```go
// ✅ Filter registration — multi-glob is valid
server.OpenFilter("items/*/*/*")

// ✅ Write — fill in known segments, glob the last
ooo.Push(server, "items/electronics/phones/*", data)

// ❌ WRONG — REST rejects multi-glob in request keys
// POST to "items/*/*/*" will fail (ErrInvalidGlobCount)

// ❌ WRONG — reading at wrong depth (slash count mismatch)
ooo.GetList[T](server, "items/electronics/*")
// This does NOT return items at items/electronics/phones/{id}
// because * matches one segment, not two

// ✅ CORRECT — read at the depth you wrote to
ooo.GetList[T](server, "items/electronics/phones/*")
```

### Glob Path Mismatches

```go
// ❌ WRONG - RemoteSet requires non-glob path
io.RemoteSet(cfg, "items/*", data)  // Returns io.ErrPathGlobNotAllowed

// ❌ WRONG - RemotePush requires glob path
io.RemotePush(cfg, "items/123", data)  // Returns io.ErrPathGlobRequired

// ✅ CORRECT
io.RemoteSet(cfg, "items/123", data)  // Single item
io.RemotePush(cfg, "items/*", data)   // Add to list
```

### Silent Error Ignoring

```go
// ❌ WRONG - Error silently ignored
func SendData(host string, data MyType) {
    io.RemoteSet(io.RemoteConfig{Host: host}, "path", data)
}

// ✅ CORRECT - Log or handle errors
func SendData(host string, data MyType) error {
    err := io.RemoteSet(io.RemoteConfig{Client: client, Host: host}, "path", data)
    if err != nil {
        log.Printf("SendData failed: %v", err)
    }
    return err
}
```

### Server Must Have Filters

Remote operations only work if the target server has appropriate filters configured:

```go
// Server side - must have filter for path
server.OpenFilter("baccarat/burn/starter")  // Enables read/write/delete

// Client side - now RemoteSet will work
io.RemoteSet(cfg, "baccarat/burn/starter", card)
```

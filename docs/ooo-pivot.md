---
description: pivot package - AP distributed multi-instance synchronization for ooo
category: sync
triggers:
  - pivot
  - cluster
  - node
  - sync
  - distributed
  - multi-instance
  - ClusterURL
  - NodesKey
  - pivot.Setup
  - pivot.Config
  - AP system
  - partition
  - leader
  - follower
when: Distributed systems, multi-node setup, cluster configuration, sync issues between instances
related:
  - ooo-package
  - ooo-ko
---

# Pivot Package Reference

**Repository:** https://github.com/benitogf/pivot

Pivot enables synchronization across multiple ooo server instances using an AP (Available + Partition-tolerant) distributed architecture.

---

## CAP Theorem Context

Pivot prioritizes **Availability** and **Partition Tolerance** over Consistency:
- Nodes accept writes even when cluster leader is unreachable
- Data synchronizes when connectivity is restored
- Uses **last-write-wins** conflict resolution

---

## Installation

```bash
go get github.com/benitogf/pivot
```

---

## Architecture

```
┌─────────────────┐
│ Cluster Leader  │  (ClusterURL = "")
│   ooo Server    │
└────────┬────────┘
         │ sync
    ┌────┴────┐
    ▼         ▼
┌───────┐ ┌───────┐
│ Node  │ │ Node  │  (ClusterURL = "leader:8800")
│   A   │ │   B   │
└───────┘ └───────┘
```

---

## Cluster Leader Setup

```go
import (
    "github.com/benitogf/ooo"
    "github.com/benitogf/ooo/storage"
    "github.com/benitogf/pivot"
)

server := &ooo.Server{}
server.Storage = storage.New(storage.LayeredConfig{
    Memory: storage.NewMemoryLayer(),
})

// External storage (e.g., for auth)
authStorage := storage.New(storage.LayeredConfig{
    Memory: storage.NewMemoryLayer(),
})

config := pivot.Config{
    Keys: []pivot.Key{
        {Path: "users/*", Database: authStorage}, // External storage
        {Path: "settings"},                        // nil = server.Storage
        {Path: "devices/*"},                       // Sync device data
    },
    NodesKey:   "devices/*",  // Node discovery path (entries need IP/Port)
    ClusterURL: "",           // Empty string = this is the cluster leader
}

pivot.Setup(server, config)
pivot.GetInstance(server).Attach(authStorage)

server.Start("0.0.0.0:8800")
```

---

## Node (Follower) Setup

```go
config := pivot.Config{
    Keys: []pivot.Key{
        {Path: "users/*"},
        {Path: "settings"},
        {Path: "devices/*"},
    },
    NodesKey:   "devices/*",
    ClusterURL: "192.168.1.100:8800", // Cluster leader address
}

pivot.Setup(server, config)
server.Start("0.0.0.0:8801")
```

---

## Config Options

| Field | Type | Description |
|-------|------|-------------|
| `Keys` | `[]pivot.Key` | Paths to synchronize |
| `NodesKey` | `string` | Path for node discovery (entries need `IP` field) |
| `ClusterURL` | `string` | Empty = leader, non-empty = leader's address |
| `Client` | `*http.Client` | Custom HTTP client for sync requests |

### pivot.Key Structure

```go
type Key struct {
    Path     string           // Glob pattern to sync (e.g., "users/*")
    Database storage.Database // nil = use server.Storage
}
```

---

## Node Discovery

Nodes register themselves by creating entries with `IP` and `Port` fields:

```go
// On cluster leader, register a node
ooo.Push(server, "devices/*", Device{
    IP:   "192.168.1.101",
    Port: 8801,
    Name: "Game Table 1",
    // ... other fields
})
```

**Important:** 
- Entries with `Port: 0` are treated as data, not node servers
- The `IP` and `Port` fields are extracted to construct sync addresses

---

## External Storage (Attach)

For storages other than `server.Storage`:

```go
// After pivot.Setup()
instance := pivot.GetInstance(server)

// Attach handles: Start + BeforeRead + WatchWithCallback
instance.Attach(authStorage)

// With additional options
instance.Attach(authStorage, storage.Options{
    AfterWrite: func(key string) {
        log.Println("Auth changed:", key)
    },
})
```

---

## Pivot Routes

All pivot routes are prefixed with `/_pivot`:

| Route | Description |
|-------|-------------|
| `/_pivot/pivot` | Sync status |
| `/_pivot/health/nodes` | Node health status |
| `/_pivot/activity/{key}` | Activity for a key |
| `/_pivot/pivot/{key}` | Sync data for key |
| `/_pivot/pivot/{key}/{index}` | Sync specific item |
| `/_pivot/pivot/{key}/{index}/{time}` | Sync with timestamp |

---

## Node Health Monitoring

Cluster leaders track node health automatically:

```bash
GET /_pivot/health/nodes
```

Response:
```json
[
    {"address": "192.168.1.10:8080", "healthy": true, "lastCheck": "2026-01-05T16:43:00+08:00"},
    {"address": "192.168.1.11:8080", "healthy": false, "lastCheck": "2026-01-05T16:42:30+08:00"}
]
```

- Unhealthy nodes are skipped during sync
- Re-checked every 30 seconds
- Automatically marked healthy when back online

---

## HTTP Client Configuration

Default client settings:
- **500ms dial timeout** - Quick detection of unreachable nodes
- **30s overall timeout** - Handles large data transfers
- **Connection pooling** - Efficient connection reuse

Custom client:
```go
config := pivot.Config{
    Keys:       []pivot.Key{{Path: "settings"}},
    ClusterURL: clusterURL,
    Client:     &http.Client{Timeout: 10 * time.Second},
}
```

---

## Common Patterns

### Node Server Setup

```go
pivotURL := *pivot + ":" + strconv.Itoa(*pivotPort)

config := pivot.Config{
    Keys: []pivot.Key{
        {Path: "devices/*"},
        {Path: "settings"},
    },
    NodesKey:   "devices/*",
    ClusterURL: pivotURL,
}

pivot.Setup(server, config)
```

### Auth Sync with Pivot

```go
// Auth storage needs to be synced separately
authStorage := storage.New(storage.LayeredConfig{...})

config := pivot.Config{
    Keys: []pivot.Key{
        {Path: "accounts/*", Database: authStorage},
        {Path: "devices/*"},
    },
    NodesKey:   "devices/*",
    ClusterURL: pivotURL,
}

pivot.Setup(server, config)
pivot.GetInstance(server).Attach(authStorage)
```

---

## Sync Behavior

1. **Leader writes** → Triggers sync to all healthy nodes
2. **Node writes** → Local only (syncs when leader pushes)
3. **Conflict resolution** → Last-write-wins based on timestamp
4. **Partition recovery** → Full sync when connectivity restored

---

## Related Packages

- [ooo](https://github.com/benitogf/ooo) - Core server
- [ko](https://github.com/benitogf/ko) - Persistent storage
- [ooo/storage](https://github.com/benitogf/ooo) - Storage interfaces

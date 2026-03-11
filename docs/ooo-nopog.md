---
description: nopog package - PostgreSQL key-value storage adapter for large-scale data
category: storage
triggers:
  - nopog
  - postgres
  - postgresql
  - large scale
  - millions
  - database
  - sql
  - GetN
  - GetNRange
when: Large-scale data storage, millions of records, complex queries, when ooo in-memory isn't enough
related:
  - ooo-package
---

# nopog Package Reference

**Repository:** https://github.com/benitogf/nopog

Key-value abstraction using PostgreSQL JSON column type. Use for large-scale data storage when ooo's in-memory storage isn't suitable.

---

## When to Use nopog

- **Large datasets** - Millions of records
- **Complex queries** - SQL-based filtering
- **Persistence requirements** - PostgreSQL durability
- **Bulk data** - Historical records, logs, analytics

**Note:** For real-time state/settings, use ooo. Combine both: ooo for real-time, nopog for bulk data.

---

## Installation

```bash
go get github.com/benitogf/nopog
```

---

## Database Setup

Create a database and run the SQL script:

```sql
-- From: https://github.com/benitogf/nopog/blob/master/nopog.sql
CREATE TABLE IF NOT EXISTS objects (
    key TEXT PRIMARY KEY,
    created BIGINT NOT NULL,
    updated BIGINT NOT NULL,
    value JSONB NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_objects_created ON objects (created);
CREATE INDEX IF NOT EXISTS idx_objects_key_pattern ON objects (key text_pattern_ops);
```

---

## Basic Usage

```go
import "github.com/benitogf/nopog"

storage := &nopog.Storage{
    Name: "nopog",      // Database name
    IP:   "10.0.1.249", // PostgreSQL server IP
}
storage.Start()
defer storage.Close()

// Set
_, err := storage.Set("test/1", `{"name": "test item"}`)

// Get
dataList, err := storage.Get("test/1")
if len(dataList) > 0 {
    log.Println(dataList[0])
}

// Get with glob pattern
items, err := storage.Get("test/*")
for _, item := range items {
    log.Println(item.Key, item.Value)
}

// Delete
err = storage.Del("test/1")
```

---

## Storage Interface

```go
type Object struct {
    Created int64           `json:"created"`
    Updated int64           `json:"updated"`
    Key     string          `json:"key"`
    Value   json.RawMessage `json:"value"`
}

// Core methods
Start() error
Close()
Clear() error

// Key operations
Keys() ([]string, error)
KeysRange(path string, from, to int64, limit int) ([]string, error)

// CRUD
Get(path string) ([]Object, error)
GetN(path string, limit int) ([]Object, error)
GetNRange(path string, from, to int64, limit int) ([]Object, error)
Set(key string, value string) (string, error)
Del(path string) error
```

---

## Configuration

```go
storage := &nopog.Storage{
    Name:     "mydb",           // Database name
    IP:       "localhost",      // PostgreSQL host
    Port:     5432,             // PostgreSQL port (default: 5432)
    User:     "postgres",       // Username
    Password: "password",       // Password
    SSLMode:  "disable",        // SSL mode
}
```

---

## Query Patterns

### Get Single Item

```go
items, err := storage.Get("users/123")
if len(items) > 0 {
    user := items[0]
    log.Printf("User: %s, Created: %d", user.Value, user.Created)
}
```

### Get List with Glob

```go
// All users
users, err := storage.Get("users/*")

// All items under a specific path
items, err := storage.Get("games/123/rounds/*")
```

### Get with Limit

```go
// Get latest 100 items
items, err := storage.GetN("logs/*", 100)
```

### Get with Time Range

```go
// Get items created between timestamps
from := time.Now().Add(-24 * time.Hour).UnixNano()
to := time.Now().UnixNano()
items, err := storage.GetNRange("events/*", from, to, 1000)
```

### Keys in Time Range

```go
// Get keys (not full objects) in time range
keys, err := storage.KeysRange("events/*", from, to, 1000)
```

---

## Combining with ooo

```go
// Real-time state with ooo
server := &ooo.Server{
    Storage: oooStorage, // In-memory + LevelDB
}
server.OpenFilter("game")
server.OpenFilter("settings")

// Bulk data with nopog
historyStorage := &nopog.Storage{
    Name: "history",
    IP:   "localhost",
}
historyStorage.Start()

// Save historical data to nopog
server.AfterWriteFilter("games/*", func(index string) {
    game, _ := ooo.Get[Game](server, index)
    data, _ := json.Marshal(game.Data)
    historyStorage.Set(index, string(data))
})
```

---

## Troubleshooting

### PostgreSQL 10 Collation Error

Error:
```
collation "pg_catalog.C.UTF-8" for encoding "UTF8" does not exist
```

Fix: Change collation in SQL script to:
```sql
COLLATE pg_catalog."und-x-icu"
```

Or find available collations:
```sql
SELECT * FROM pg_collation;
```

---

## Common Patterns

### History Storage

```go
// In history service
historyDB := &nopog.Storage{
    Name: "history",
    IP:   *postgresIP,
}
historyDB.Start()

// Query historical games
games, err := historyDB.GetNRange(
    fmt.Sprintf("games/%s/*", tableID),
    fromTime.UnixNano(),
    toTime.UnixNano(),
    1000,
)
```

### Bulk Export

```go
// Export all data for a table
keys, err := storage.Keys()
for _, key := range keys {
    if strings.HasPrefix(key, "table/123/") {
        items, _ := storage.Get(key)
        // Process items...
    }
}
```

---

## Related Packages

- [ooo](https://github.com/benitogf/ooo) - Real-time state (use for small/medium data)
- [ko](https://github.com/benitogf/ko) - LevelDB persistence (alternative to nopog)

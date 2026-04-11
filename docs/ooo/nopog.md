---
description: nopog package - long-term historical data storage for the ooo ecosystem
category: storage
triggers:
  - nopog
  - history
  - historical data
  - long term storage
  - millions of records
  - time range query
  - GetN
  - GetNRange
  - KeysRange
  - analytics
  - logs
  - audit trail
  - store old data
  - keep records over time
  - query by date range
  - large dataset
  - data retention policy
  - archive data
when: Long-term historical data, millions of records, time-range queries, analytics, audit trails — used alongside ooo, not as a replacement
related:
  - ooo/package
---

# nopog — Long-Term Historical Data Storage

**Repository:** https://github.com/benitogf/nopog

Key-value storage for long-term historical data. Use alongside ooo when you need to retain and query millions of records over time.

---

## When to Use nopog

- **Historical records** — Game rounds, transactions, audit trails
- **Analytics data** — Logs, metrics, event streams
- **Time-range queries** — "Show me records from the last 24 hours"
- **Large retention** — Millions of records that don't belong in real-time state

nopog is **not** a replacement for ooo. Use ooo for real-time state and settings. Use nopog alongside ooo for long-term data retention.

---

## Setup

```bash
go get github.com/benitogf/nopog
```

nopog requires a running database instance. Initialize the schema:

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
    IP:       "localhost",      // Database host
    Port:     5432,             // Database port (default: 5432)
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

## Using with ooo

The typical pattern: ooo manages real-time state, nopog stores historical data alongside it.

```go
// Real-time state with ooo
server := &ooo.Server{
    Storage: oooStorage,
}
server.OpenFilter("game")
server.OpenFilter("settings")

// Historical data with nopog
historyStorage := &nopog.Storage{
    Name: "history",
    IP:   "localhost",
}
historyStorage.Start()

// Archive data after every write
server.AfterWriteFilter("games/*", func(index string) {
    game, _ := ooo.Get[Game](server, index)
    data, _ := json.Marshal(game.Data)
    historyStorage.Set(index, string(data))
})
```

---

## Troubleshooting

### Collation Error

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

## Related Docs

- `ooo/package` — Real-time state management (use for app state, settings, small/medium data)
- `ooo/auth` — JWT authentication (if storing user-related history)

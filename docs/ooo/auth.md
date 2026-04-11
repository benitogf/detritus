---
description: Simple JWT auth - github.com/benitogf/auth for lightweight services
category: auth
triggers:
  - github.com/benitogf/auth
  - simple auth
  - jwt auth
  - register endpoint
  - /register
  - /authorize
  - /verify
  - token
  - tokenAuth
  - Bearer
  - login
  - sign in
  - protect endpoint
  - require authentication
  - user access control
  - password
  - secure API
  - who can access
when: Simple JWT auth, basic token auth without policies
related:
  - ooo/package
  - ooo/client-js
---

# Simple Auth (`github.com/benitogf/auth`)

**Repository:** https://github.com/benitogf/auth

Lightweight JWT authentication for the ooo ecosystem. Used by simple services that don't need password policies, session management, or alternate auth methods.

---

## When to Use

- Lightweight services that need basic JWT auth
- Services without password policy requirements
- Services without session management or alternate auth methods

---

## Installation

```bash
go get github.com/benitogf/auth
```

---

## Basic Setup

```go
import (
    "net/http"
    "time"

    "github.com/benitogf/auth"
    "github.com/benitogf/ko"
    "github.com/benitogf/ooo"
    "github.com/benitogf/ooo/storage"
    "github.com/gorilla/mux"
)

// Auth storage (for users)
authEmbedded := &ko.EmbeddedStorage{Path: "db/auth"}
authStorage := storage.New(storage.LayeredConfig{
    Memory:   storage.NewMemoryLayer(),
    Embedded: authEmbedded,
})
err := authStorage.Start(storage.Options{})
go storage.WatchStorageNoop(authStorage)

// Create auth with JWT token expiry
key := "your-secret-key"
tokenAuth := auth.New(
    auth.NewJwtStore(key, time.Minute*10), // 10-minute token expiry
    authStorage,
)

// Create server
server := &ooo.Server{
    Static: true,
    Router: mux.NewRouter(),
}

// Add audit middleware
server.Audit = func(r *http.Request) bool {
    if r.URL.Path == "/open" {
        return true
    }
    return tokenAuth.Verify(r)
}

// Add auth routes
tokenAuth.Router(server)

server.Start("0.0.0.0:8800")
```

---

## Routes

| Route | Method | Description |
|-------|--------|-------------|
| `/register` | POST | Register new user |
| `/authorize` | POST | Login and get token |
| `/verify` | GET | Verify token validity |

### Register

```bash
POST /register
Content-Type: application/json

{
    "account": "username",
    "password": "password123"
}
```

### Authorize (Login)

```bash
POST /authorize
Content-Type: application/json

{
    "account": "username",
    "password": "password123"
}

# Response
{
    "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

### Verify Token

```bash
GET /verify
Authorization: Bearer <token>

# Response: 200 OK if valid, 401 if invalid
```

---

## Token Configuration

```go
auth.NewJwtStore(key, time.Minute*10)  // 10 minutes
auth.NewJwtStore(key, time.Hour*24)    // 24 hours
auth.NewJwtStore(key, time.Hour*24*7)  // 7 days
```

---

## Audit Middleware

```go
server.Audit = func(r *http.Request) bool {
    // Public paths
    publicPaths := []string{"/open", "/health", "/status"}
    for _, path := range publicPaths {
        if strings.HasPrefix(r.URL.Path, path) {
            return true
        }
    }
    
    // OPTIONS for CORS
    if r.Method == http.MethodOptions {
        return true
    }
    
    return tokenAuth.Verify(r)
}
```

---

## Using Token in Requests

```bash
GET /protected/resource
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

---

## TokenAuth Methods

| Method | Description |
|--------|-------------|
| `Verify(r *http.Request) bool` | Verify token from Authorization header |
| `VerifyWS(r *http.Request) bool` | Verify token for WebSocket (query param) |
| `Router(server *ooo.Server, remoteURL ...string)` | Add auth routes |
| `New(store JwtStore, storage Database) *TokenAuth` | Create new auth instance |

---

## Remote Auth Verification

For services that verify tokens against a central auth server:

```go
authURL := *authIP + ":" + strconv.Itoa(*authPort)

tokenAuth := auth.New(
    auth.NewJwtStore(*key, time.Minute*10),
    authStorage,
)

// Router with remote auth URL for verification
tokenAuth.Router(server, authURL)
```

---

## Related

- `ooo/package` - Core ooo server

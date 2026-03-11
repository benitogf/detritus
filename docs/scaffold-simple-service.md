---
description: Scaffold a new simple backend service using ooo + ko
category: scaffold
triggers:
  - new service
  - create service
  - scaffold service
  - add service
  - new backend
  - service template
when: Creating a new backend service from scratch
related:
  - ooo-package
  - ooo-ko
---

# Scaffold Simple Service

Create a new backend service using the ooo ecosystem. This scaffold uses:
- `ooo` - Core real-time server
- `ko` - LevelDB persistent storage

For more complex services, see specialized scaffolds (pivot sync, kafka publishing, auth) when available.

---

## Directory Structure

```
servicename/
├── dockerfile
├── main.go
└── router/
    ├── opt/
    │   └── opt.go
    ├── routes.go
    └── startup.go
```

---

## Step 1: Create Directory Structure

```bash
mkdir -p servicename/router/opt
```

---

## Step 2: Create `router/opt/opt.go`

```go
package opt

type Opt struct {
	// Add service-specific configuration here
}
```

---

## Step 3: Create `router/routes.go`

```go
package router

import (
	routerOpt "<module>/servicename/router/opt"
	"github.com/benitogf/ooo"
)

func Routes(server *ooo.Server, opt routerOpt.Opt) {
	// Define filters and routes here
	// See /ooo-package for filter patterns
}
```

> **Note:** Replace `<module>` with the module path from your `go.mod` file.

---

## Step 4: Create `router/startup.go`

```go
package router

import (
	routerOpt "<module>/servicename/router/opt"
	"github.com/benitogf/ooo"
)

func OnStartup(server *ooo.Server, opt routerOpt.Opt) {
	// Add startup tasks and background goroutines here
}
```

---

## Step 5: Create `main.go`

```go
package main

import (
	"flag"
	"strconv"

	"<module>/servicename/router"
	routerOpt "<module>/servicename/router/opt"
	"github.com/benitogf/ko"
	"github.com/benitogf/network"
	"github.com/benitogf/ooo"
	"github.com/benitogf/ooo/storage"
	"github.com/gorilla/mux"
)

var dataPath = flag.String("dataPath", "db/data", "data storage path")
var port = flag.Int("port", 3XXX, "service port")
var silence = flag.Bool("silence", true, "silence output")

func main() {
	flag.Parse()

	// Server
	server := &ooo.Server{
		Silence: *silence,
		Storage: storage.New(storage.LayeredConfig{
			Embedded: &ko.EmbeddedStorage{Path: *dataPath},
		}),
		Router: mux.NewRouter(),
		Client: network.NewHttpClient(),
	}

	// Options
	opt := routerOpt.Opt{}

	// Routes
	router.Routes(server, opt)
	server.Start("0.0.0.0:" + strconv.Itoa(*port))

	// Startup
	router.OnStartup(server, opt)

	server.WaitClose()
}
```

---

## Step 6: Create `dockerfile`

```dockerfile
# build stage
FROM golang:alpine AS build-env
RUN apk --no-cache add build-base git gcc
ADD go.mod /src/go.mod
ADD go.sum /src/go.sum
ADD servicename /src/servicename
RUN cd /src/servicename && go build

# final stage
FROM alpine
RUN apk --no-cache add tzdata
WORKDIR /app
COPY --from=build-env /src/servicename/servicename /app/
ENV SILENCE true
ENTRYPOINT ./servicename -silence=$SILENCE
EXPOSE 3XXX
```

**Notes:**
- Add additional `ADD` lines for any packages your service imports from the monorepo
- Add `ENV` and flag mappings to `ENTRYPOINT` for each flag your service uses
- The binary name matches the directory name by default

---

## Step 7: Assign Port Number

Check existing services in the project to avoid port conflicts. Pick an unused port (e.g., in the 3000-9000 range).

---

## Checklist

- [ ] Replace `servicename` with actual service name in all files
- [ ] Set unique port number in `main.go` and `dockerfile`
- [ ] Add service-specific flags to `main.go` if needed
- [ ] Add service-specific options to `opt/opt.go`
- [ ] Implement filters in `routes.go` (see `/ooo-package`)
- [ ] Implement startup tasks in `startup.go`
- [ ] Update `dockerfile` with any additional project dependencies
- [ ] Update `dockerfile` ENV vars and ENTRYPOINT flags
- [ ] Test locally: `go run . -port=XXXX`
- [ ] Test docker build: `docker build -f servicename/dockerfile -t servicename .` (from project root)

---

## Common Additions

### Add Flag with Validation

```go
var requiredIP = flag.String("requiredIP", "", "IP of required service")

func main() {
	flag.Parse()
	if *requiredIP == "" {
		panic("can't have an empty requiredIP")
	}
	// ...
}
```

### Add Custom Timeouts

```go
server := &ooo.Server{
	// ...
	ReadTimeout:  50 * time.Minute,
	WriteTimeout: 50 * time.Minute,
	IdleTimeout:  50 * time.Minute,
	Deadline:     50 * time.Minute,
}
```

### Add Static File Serving

```go
func Routes(server *ooo.Server, opt routerOpt.Opt) {
	// ...
	static := http.StripPrefix("/static/", http.FileServer(http.Dir("./static/")))
	server.Router.PathPrefix("/static/").Handler(static)
}
```

---

## Next Steps

After basic scaffold is working, consider adding:
- **Pivot sync** - For distributed data (see `/ooo-pivot`)
- **Kafka publishing** - For event streaming
- **Authentication** - For secured endpoints (see `/ooo-auth`)

---

## Related Workflows

- `/ooo-package` - Server setup, filters, CRUD operations
- `/ooo-ko` - Storage configuration
- `/ooo-pivot` - Multi-instance synchronization (when needed)

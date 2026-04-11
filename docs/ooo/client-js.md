---
description: ooo-client - JavaScript WebSocket client for ooo servers
category: frontend
triggers:
  - ooo-client
  - javascript
  - react
  - frontend
  - browser
  - npm
  - websocket client
  - subscribe
  - onMessage
  - TypeScript
  - useSubscribe
  - usePublish
  - useOoo
  - publish
  - unpublish
  - JSON Patch
  - show data in browser
  - display data from server in UI
  - data updates automatically in frontend
  - connect frontend to backend
  - react component with live data
  - listen for changes
when: Frontend integration, React apps, JavaScript WebSocket client, browser-side subscriptions
related:
  - ooo/package
  - ooo/auth
---

# ooo-client Package Reference

**Repository:** https://github.com/benitogf/ooo-client  
**NPM:** https://www.npmjs.com/package/ooo-client

JavaScript client for ooo with reconnecting WebSocket, JSON Patch support, and automatic state caching.

---

## Features

- Reconnecting WebSocket abstraction
- Automatic JSON Patch handling for efficient updates
- State caching (keeps latest snapshot)
- CRUD operations via WebSocket or HTTP
- TypeScript support

---

## Installation

```bash
npm i ooo-client
```

---

## Basic Usage

### Single Object Subscription

```javascript
import ooo from 'ooo-client'

const client = ooo('localhost:8800/settings')

client.onopen = async () => {
    // Create or update
    await client.publish('settings', { theme: 'dark', language: 'en' })
    
    // Update
    await client.publish('settings', { theme: 'light' })
    
    // Delete
    await client.unpublish('settings')
}

client.onmessage = (msg) => {
    console.log('Settings updated:', msg)
    // msg contains the full object state
}

client.onerror = (err) => {
    console.error('Connection error:', err)
    client.close()
}
```

### List Subscription

```javascript
import ooo from 'ooo-client'

const client = ooo('localhost:8800/items/*')

client.onopen = async () => {
    // Create new item (returns ID)
    const id = await client.publish('items/*', { name: 'Item 1' })
    console.log('Created:', id)
    
    // Update specific item
    await client.publish('items/' + id, { name: 'Updated Item 1' })
    
    // Create with custom ID
    await client.publish('items/custom-id', { name: 'Custom Item' })
    
    // Delete all items
    await client.unpublish('items/*')
}

client.onmessage = (msg) => {
    // msg is an array of items for list subscriptions
    console.log('Items:', msg)
}

client.onerror = (err) => {
    console.error('Error:', err)
}
```

---

## API Reference

### Constructor

```javascript
const client = ooo(url, options)
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `url` | `string` | WebSocket URL (e.g., `localhost:8800/path`) |
| `options` | `object` | Optional configuration |

### Options

```javascript
const client = ooo('localhost:8800/items/*', {
    headers: {
        Authorization: 'Bearer <token>'
    },
    protocol: 'wss', // Use secure WebSocket
})
```

### Event Handlers

| Handler | Description |
|---------|-------------|
| `onopen` | Called when connection established |
| `onmessage` | Called when data received (snapshot or patch applied) |
| `onerror` | Called on connection error |
| `onclose` | Called when connection closed |

### Methods

| Method | Description |
|--------|-------------|
| `publish(path, data)` | Create or update data at path |
| `unpublish(path)` | Delete data at path |
| `close()` | Close the connection |

---

## Message Format

Messages arrive as either:
1. **Snapshot** - Full data on initial connection or reconnection
2. **Patch** - JSON Patch operations for incremental updates

The client automatically applies patches and maintains the current state.

```javascript
client.onmessage = (msg) => {
    // msg is always the current state (patches already applied)
    // For objects: { field1: 'value', field2: 'value' }
    // For lists: [{ index: '123', created: 1234567890, data: {...} }, ...]
}
```

---

## HTTP Fallback

You can also use standard HTTP/fetch with ooo servers:

```javascript
// Create/Update
await fetch('http://localhost:8800/settings', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ theme: 'dark' })
})

// Read
const response = await fetch('http://localhost:8800/settings')
const data = await response.json()

// Delete
await fetch('http://localhost:8800/settings', { method: 'DELETE' })

// Read list
const listResponse = await fetch('http://localhost:8800/items/*')
const items = await listResponse.json()
```

---

## React Integration Example

```jsx
import { useState, useEffect, useCallback } from 'react'
import ooo from 'ooo-client'

function useOoo(path) {
    const [data, setData] = useState(null)
    const [client, setClient] = useState(null)
    const [connected, setConnected] = useState(false)

    useEffect(() => {
        const c = ooo(`localhost:8800/${path}`)
        
        c.onopen = () => setConnected(true)
        c.onmessage = (msg) => setData(msg)
        c.onerror = () => setConnected(false)
        c.onclose = () => setConnected(false)
        
        setClient(c)
        
        return () => c.close()
    }, [path])

    const publish = useCallback(async (data) => {
        if (client) await client.publish(path, data)
    }, [client, path])

    return { data, publish, connected }
}

// Usage
function Settings() {
    const { data, publish, connected } = useOoo('settings')
    
    return (
        <div>
            <p>Status: {connected ? 'Connected' : 'Disconnected'}</p>
            <pre>{JSON.stringify(data, null, 2)}</pre>
            <button onClick={() => publish({ theme: 'dark' })}>
                Set Dark Theme
            </button>
        </div>
    )
}
```

---

## Authentication

```javascript
// Get token first
const authResponse = await fetch('http://localhost:8800/authorize', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ account: 'user', password: 'pass' })
})
const { token } = await authResponse.json()

// Use token with WebSocket
const client = ooo('localhost:8800/protected/*', {
    headers: { Authorization: `Bearer ${token}` }
})
```

---

## Common Patterns

### Subscribe to Single Item

```javascript
const configClient = ooo(`${serverUrl}/config`)
configClient.onmessage = (config) => {
    updateDisplay(config)
}
```

### Subscribe to List

```javascript
const itemsClient = ooo(`${serverUrl}/items/*`)
itemsClient.onmessage = (items) => {
    setItems(items.map(d => d.data))
}
```

---

## Related Packages

- [ooo](https://github.com/benitogf/ooo) - Core server (Go)
- [auth](https://github.com/benitogf/auth) - JWT authentication

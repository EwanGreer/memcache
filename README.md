# memcache

A thread-safe in-memory LRU cache for Go with TTL support.

## Features

- LRU eviction by item count or byte size
- TTL expiration with lazy deletion
- Atomic GetOrSet with singleflight (dedupes concurrent fetches)
- Context support for cancellation/timeouts
- Hit/miss/eviction statistics
- Eviction callbacks
- Optional file persistence (survives restarts)
- Real-time event stream and HTTP debug viewer (opt-in)

## Installation

```bash
go get github.com/EwanGreer/memcache
```

## Usage

```go
package main

import (
    "time"
    "github.com/EwanGreer/memcache"
)

func main() {
    // Basic cache
    c := memcache.New()
    c.Set("key", []byte("value"))
    c.Get("key")

    // With limits
    c = memcache.NewWithOptions(memcache.Options{
        MaxItems: 1000,
        MaxBytes: 64 * 1024 * 1024, // 64MB
        OnEvict: func(key string, value []byte) {
            // cleanup callback
        },
    })

    // TTL
    c.SetWithTTL("session", []byte("data"), 30*time.Minute)

    // Atomic get-or-compute (concurrent calls share single fetch)
    val := c.GetOrSet("user:1", func() []byte {
        return fetchFromDB("user:1")
    })

    // With context for cancellation/timeout
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    val, err := c.GetOrSetWithContext(ctx, "user:1", func(ctx context.Context) ([]byte, error) {
        return fetchFromDBWithContext(ctx, "user:1")
    })

    // Set if not exists
    ok := c.SetNX("lock", []byte("holder"))

    // Stats
    stats := c.Stats()
    fmt.Printf("hits=%d misses=%d\n", stats.Hits, stats.Misses)

    // Bulk cleanup
    c.DeleteExpired()
}
```

### Persistence

Add file persistence to survive restarts:

```go
// Basic persistence - data auto-syncs every 30s
c := memcache.NewWithOptions(memcache.Options{
    FilePath: "/var/cache/myapp.db",
})
defer c.Close() // Final flush on shutdown

// Full configuration
c := memcache.NewWithOptions(memcache.Options{
    MaxItems:     1000,
    FilePath:     "/var/cache/myapp.db",
    SyncStrategy: memcache.SyncPeriodic, // SyncNone, SyncImmediate, or SyncPeriodic
    SyncInterval: time.Minute,           // For SyncPeriodic
    OnError: func(op string, err error) {
        log.Printf("cache %s error: %v", op, err)
    },
})
defer c.Close()

// Manual flush
c.Flush()

// Reload from disk
c.Load()
```

Sync strategies:
- `SyncNone` - Manual `Flush()` only
- `SyncImmediate` - Write on every mutation (debounced)
- `SyncPeriodic` - Background sync at intervals (default when FilePath is set)

Note: LRU order is not preserved across restarts. Loaded items are treated as recently accessed.

### Real-Time Debug Viewer

Observe cache mutations in real time via a web-based dashboard:

```go
c := memcache.NewWithOptions(memcache.Options{MaxItems: 1000})

// Standalone server on separate port
go memcache.ListenAndServeDebug(c, ":9090")

// Or mount into existing server
mux.Handle("/debug/cache/", http.StripPrefix("/debug/cache", memcache.DebugHandler(c)))
```

Open http://localhost:9090 to see:
- **Live stats** (items, bytes, hits, misses, evictions) refreshed every 2s
- **Items table** (key, size, TTL) in LRU order with search filter
- **Event stream** (set, delete, evict, expire, clear) via SSE

The debug server is **zero-cost when not used** — no goroutines, channels, or overhead unless you explicitly call `ListenAndServeDebug()` or `DebugHandler()`.

### Event Subscriptions

Subscribe to cache events programmatically:

```go
// Subscribe to all events
sub := c.Subscribe()
defer sub.Close()

for event := range sub.C {
    fmt.Printf("%s: key=%s size=%d\n", event.Type, event.Key, event.Size)
}

// Subscribe to specific event types
sub := c.Subscribe(memcache.EventSet, memcache.EventEvict)
```

Event types: `EventSet`, `EventDelete`, `EventEvict`, `EventExpire`, `EventClear`

Events are delivered via buffered channel (256). Slow consumers drop events (non-blocking). The subscription system is **zero-cost with no subscribers** — a fast-path `RLock` check short-circuits immediately with no allocations.

## API

| Method                        | Description                      |
| ----------------------------- | -------------------------------- |
| `New()`                       | Create unlimited cache           |
| `NewWithMaxSize(n)`           | Create cache with max item count |
| `NewWithOptions(opts)`        | Create cache with full options   |
| `Get(key)`                    | Get value or nil                 |
| `Set(key, value)`             | Store value                      |
| `SetWithTTL(key, value, ttl)` | Store with expiration            |
| `SetNX(key, value)`           | Set if not exists                |
| `GetOrSet(key, fn)`           | Get or compute and store         |
| `GetOrSetWithContext(ctx, key, fn)` | Same with context support  |
| `Delete(key)`                 | Remove key                       |
| `Has(key)`                    | Check existence                  |
| `Keys()`                      | List all keys                    |
| `Len()`                       | Count of items                   |
| `Bytes()`                     | Current memory usage             |
| `Clear()`                     | Remove all items                 |
| `DeleteExpired()`             | Bulk remove expired              |
| `Stats()`                     | Get hit/miss/eviction counts     |
| `Flush()`                     | Write to disk immediately        |
| `Load()`                      | Read from disk, replace contents |
| `Close()`                     | Stop sync, final flush           |
| `Subscribe(eventTypes...)`    | Subscribe to cache events        |
| `DebugHandler(c)`             | HTTP handler for debug dashboard |
| `ListenAndServeDebug(c, addr)`| Standalone debug server          |

## Testing

```bash
# Run tests
go test ./...

# With race detector
go test -race ./...

# With coverage
go test -coverprofile=coverage.out ./...
```

Or using mise:

```bash
mise run test
mise run test:race
mise run test:cover
mise run test:bench
```

## Demos

```bash
go run ./scripts/main.go            # TTL expiration
go run ./scripts/basic/main.go      # CRUD operations
go run ./scripts/lru/main.go        # LRU eviction
go run ./scripts/cleanup/main.go    # DeleteExpired
go run ./scripts/stats/main.go      # Hit/miss tracking
go run ./scripts/getorset/main.go   # Atomic get-or-compute
go run ./scripts/onevict/main.go    # Eviction callbacks
go run ./scripts/maxbytes/main.go   # Byte-based limits
go run ./scripts/debug/main.go      # Live debug viewer (opens :9090)
```

## License

MIT

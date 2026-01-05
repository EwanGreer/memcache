# memcache

A thread-safe in-memory LRU cache for Go with TTL support.

## Features

- LRU eviction by item count or byte size
- TTL expiration with lazy deletion
- Atomic GetOrSet for cache-aside pattern
- Hit/miss/eviction statistics
- Eviction callbacks

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

    // Atomic get-or-compute
    val := c.GetOrSet("user:1", func() []byte {
        return fetchFromDB("user:1")
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
| `Delete(key)`                 | Remove key                       |
| `Has(key)`                    | Check existence                  |
| `Keys()`                      | List all keys                    |
| `Len()`                       | Count of items                   |
| `Bytes()`                     | Current memory usage             |
| `Clear()`                     | Remove all items                 |
| `DeleteExpired()`             | Bulk remove expired              |
| `Stats()`                     | Get hit/miss/eviction counts     |

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
```

## Demos

```bash
go run ./scripts/main.go        # TTL expiration
go run ./scripts/basic/main.go  # CRUD operations
go run ./scripts/lru/main.go    # LRU eviction
go run ./scripts/cleanup/main.go    # DeleteExpired
go run ./scripts/stats/main.go      # Hit/miss tracking
go run ./scripts/getorset/main.go   # Atomic get-or-compute
go run ./scripts/onevict/main.go    # Eviction callbacks
go run ./scripts/maxbytes/main.go   # Byte-based limits
```

## License

MIT

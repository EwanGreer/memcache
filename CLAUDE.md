# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Run tests
go test ./...
go test -race ./...                  # With race detector (preferred for concurrent code)
go test -run TestName ./...          # Run a single test
go test -bench=. -benchmem ./...     # Benchmarks

# Using mise task runner
mise run test
mise run test:race
mise run test:cover
mise run test:bench

# Demo scripts
go run ./scripts/debug/main.go       # Launch live debug viewer on :9090
go run ./scripts/basic/main.go       # CRUD operations demo
```

## Architecture

This is a single-package Go library (`package memcache`) — all files are in the root directory.

**Core files:**

- `cache.go` — Primary data structure. `Cache` holds a `map[string]*list.Element` for O(1) lookup and a `container/list` where `front = MRU, back = LRU`. LRU eviction removes from back; all mutations call `MoveToFront`. Uses `sync.RWMutex` (write lock on `Get` due to `MoveToFront`). `singleflight.Group` deduplicates concurrent `GetOrSet` calls.

- `events.go` — Pub/sub for cache mutations. `EventType` is a bitmask. `emitEvent` is called while the cache mutex is held but `emit` acquires its own `subMu` RLock — no lock nesting issues. Slow consumers drop events (non-blocking send). Zero-cost when no subscribers via `len(c.subscribers) == 0` fast path.

- `file.go` — Persistence via gob encoding. Atomic writes use temp-file + rename. `SyncStrategy`: `SyncPeriodic` (default, background ticker), `SyncImmediate` (debounced signal channel), `SyncNone` (manual only). `dirty` and `flushing` are `atomic.Bool` to prevent concurrent flushes.

- `http.go` — Debug HTTP server. Embeds `dashboard.html` and `partials/` via `//go:embed`. The dashboard uses HTMX for partial refreshes — `wantsHTML()` checks `HX-Request` header. `/api/events` streams SSE using `Subscribe()`. Item values are base64-encoded in API responses.

**Key design patterns:**

- `item.size` = `len(key) + len(value)` — both key and value bytes count toward `MaxBytes`
- `emitEvent` is called while holding `c.mu` (write lock), so event handlers must not re-acquire it
- `Close()` uses `sync.Once` to prevent double-close of the `stopSync` channel
- File persistence: `Load()` on init (unless `SkipLoadOnInit`), `Flush()` on `Close()`; LRU order is not preserved across restarts

## Declaration order

Within each file, declarations must follow this order:

1. `const`
2. `var`
3. types
4. public methods
5. private methods
6. public functions
7. private functions

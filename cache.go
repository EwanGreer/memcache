package memcache

import (
	"container/list"
	"context"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

type item struct {
	key       string
	value     []byte
	size      int
	expiresAt time.Time
}

type Stats struct {
	Hits      int64
	Misses    int64
	Evictions int64
}

type Options struct {
	MaxItems int // 0 means unlimited
	MaxBytes int // 0 means unlimited
	OnEvict  func(key string, value []byte)

	// Persistence options
	FilePath       string        // Path to cache file (empty = no persistence)
	SyncStrategy   SyncStrategy  // When to sync (default: SyncPeriodic if FilePath set)
	SyncInterval   time.Duration // Periodic sync interval (default: 30s)
	SkipLoadOnInit bool          // Skip loading from file on cache creation (default: false)
}

type Cache struct {
	mu sync.RWMutex
	// items maps keys to their corresponding element in the order list.
	// The actual data lives in order; this map just provides O(1) lookup.
	items    map[string]*list.Element
	order    *list.List // front = most recent, back = least recent
	maxItems int        // 0 means unlimited
	maxBytes int        // 0 means unlimited
	curBytes int
	onEvict  func(key string, value []byte)
	stats    Stats
	group    singleflight.Group // dedupes concurrent GetOrSet calls

	// Persistence fields
	filePath     string
	syncStrategy SyncStrategy
	syncInterval time.Duration
	dirty        atomic.Bool   // Has unsaved changes
	flushing     atomic.Bool   // Prevents concurrent flushes
	stopSync     chan struct{} // Stops periodic sync goroutine
	closeOnce    sync.Once     // Ensures Close is called only once
}

func New() *Cache {
	return NewWithOptions(Options{})
}

// NewWithMaxSize creates a cache. A maxItems of 0 means unlimited.
func NewWithMaxSize(maxItems int) *Cache {
	return NewWithOptions(Options{MaxItems: maxItems})
}

func NewWithOptions(opts Options) *Cache {
	c := &Cache{
		items:    make(map[string]*list.Element),
		order:    list.New(),
		maxItems: opts.MaxItems,
		maxBytes: opts.MaxBytes,
		onEvict:  opts.OnEvict,
	}

	// Configure persistence if FilePath is set
	if opts.FilePath != "" {
		c.filePath = opts.FilePath
		c.syncStrategy = opts.SyncStrategy
		c.syncInterval = opts.SyncInterval

		// Default to SyncPeriodic if not specified
		if c.syncStrategy == SyncNone && opts.SyncStrategy == 0 {
			c.syncStrategy = SyncPeriodic
		}

		// Default interval
		if c.syncInterval == 0 {
			c.syncInterval = defaultInterval
		}

		// Load from file on init (default behavior when FilePath is set)
		if !opts.SkipLoadOnInit {
			_ = c.Load() // Ignore error on startup - file may not exist
		}

		// Start periodic sync goroutine
		if c.syncStrategy == SyncPeriodic {
			c.stopSync = make(chan struct{})
			go c.periodicSync()
		}
	}

	return c
}

func (c *Cache) periodicSync() {
	ticker := time.NewTicker(c.syncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopSync:
			return
		case <-ticker.C:
			if c.dirty.Load() {
				_ = c.Flush() // Best effort
			}
		}
	}
}

// Get returns the value for key. Expired items are deleted lazily.
func (c *Cache) Get(key string) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.items[key]
	if !ok {
		c.stats.Misses++
		return nil
	}

	it := elem.Value.(*item)
	if it.isExpired() {
		c.removeElement(elem)
		c.stats.Misses++
		return nil
	}

	c.stats.Hits++
	c.order.MoveToFront(elem)
	return it.value
}

func (c *Cache) Set(key string, value []byte) {
	c.SetWithTTL(key, value, 0)
}

// SetWithTTL stores a value. A TTL of 0 means no expiration.
// If at max capacity, the least recently used item is evicted.
func (c *Cache) SetWithTTL(key string, value []byte, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.setInternal(key, value, ttl)
}

// Must hold lock.
func (c *Cache) setInternal(key string, value []byte, ttl time.Duration) {
	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	}

	size := len(key) + len(value)

	if elem, ok := c.items[key]; ok {
		it := elem.Value.(*item)
		c.curBytes -= it.size
		it.value = value
		it.size = size
		it.expiresAt = expiresAt
		c.curBytes += size
		c.order.MoveToFront(elem)
		c.markDirty()
		return
	}

	for c.shouldEvict(size) {
		c.evict()
	}

	it := &item{
		key:       key,
		value:     value,
		size:      size,
		expiresAt: expiresAt,
	}
	elem := c.order.PushFront(it)

	c.items[key] = elem
	c.curBytes += size
	c.markDirty()
}

func (c *Cache) shouldEvict(incoming int) bool {
	if c.order.Len() == 0 {
		return false
	}
	if c.maxItems > 0 && c.order.Len() >= c.maxItems {
		return true
	}
	if c.maxBytes > 0 && c.curBytes+incoming > c.maxBytes {
		return true
	}
	return false
}

// SetNX sets the value only if the key does not exist. Returns true if set.
func (c *Cache) SetNX(key string, value []byte) bool {
	return c.SetNXWithTTL(key, value, 0)
}

// SetNXWithTTL sets the value only if the key does not exist. Returns true if set.
func (c *Cache) SetNXWithTTL(key string, value []byte, ttl time.Duration) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		it := elem.Value.(*item)
		if !it.isExpired() {
			return false
		}
		c.removeElement(elem)
	}

	c.setInternal(key, value, ttl)
	return true
}

// GetOrSet returns the value for key if it exists, otherwise calls fn and stores the result.
// Concurrent calls for the same key will share a single fn invocation.
func (c *Cache) GetOrSet(key string, fn func() []byte) []byte {
	return c.GetOrSetWithTTL(key, fn, 0)
}

// GetOrSetWithTTL returns the value for key if it exists, otherwise calls fn and stores the result.
// Concurrent calls for the same key will share a single fn invocation.
func (c *Cache) GetOrSetWithTTL(key string, fn func() []byte, ttl time.Duration) []byte {
	c.mu.Lock()

	if elem, ok := c.items[key]; ok {
		it := elem.Value.(*item)

		if !it.isExpired() {
			c.stats.Hits++
			c.order.MoveToFront(elem)
			c.mu.Unlock()

			return it.value
		}

		c.removeElement(elem)
	}
	c.mu.Unlock()

	val, _, _ := c.group.Do(key, func() (any, error) {
		return fn(), nil
	})
	value := val.([]byte)

	c.mu.Lock()
	c.stats.Misses++
	c.setInternal(key, value, ttl)
	c.mu.Unlock()

	return value
}

// GetOrSetWithContext returns the value for key if it exists, otherwise calls fn and stores the result.
// Supports cancellation via context. Concurrent calls for the same key share a single fn invocation.
func (c *Cache) GetOrSetWithContext(ctx context.Context, key string, fn func(context.Context) ([]byte, error)) ([]byte, error) {
	return c.GetOrSetWithTTLAndContext(ctx, key, fn, 0)
}

// GetOrSetWithTTLAndContext returns the value for key if it exists, otherwise calls fn and stores the result.
// Supports cancellation via context. Concurrent calls for the same key share a single fn invocation.
func (c *Cache) GetOrSetWithTTLAndContext(ctx context.Context, key string, fn func(context.Context) ([]byte, error), ttl time.Duration) ([]byte, error) {
	// Fast path: check cache
	c.mu.Lock()
	if elem, ok := c.items[key]; ok {
		it := elem.Value.(*item)
		if !it.isExpired() {
			c.stats.Hits++
			c.order.MoveToFront(elem)
			c.mu.Unlock()
			return it.value, nil
		}
		c.removeElement(elem)
	}
	c.mu.Unlock()

	// Slow path: singleflight fetch with context support
	ch := c.group.DoChan(key, func() (any, error) {
		return fn(ctx)
	})

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		if res.Err != nil {
			return nil, res.Err
		}
		value := res.Val.([]byte)

		// Store result
		c.mu.Lock()
		c.stats.Misses++
		c.setInternal(key, value, ttl)
		c.mu.Unlock()

		return value, nil
	}
}

func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.removeElement(elem)
	}
}

// Has reports whether key exists. Expired items are deleted lazily.
func (c *Cache) Has(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.items[key]
	if !ok {
		return false
	}

	it := elem.Value.(*item)
	if it.isExpired() {
		c.removeElement(elem)
		return false
	}

	return true
}

func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*list.Element)
	c.order.Init()
	c.curBytes = 0
	c.markDirty()
}

func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	count := 0
	for _, elem := range c.items {
		it := elem.Value.(*item)
		if !it.isExpired() {
			count++
		}
	}
	return count
}

func (c *Cache) DeleteExpired() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	deleted := 0
	for _, elem := range c.items {
		it := elem.Value.(*item)
		if it.isExpired() {
			c.removeElement(elem)
			deleted++
		}
	}
	return deleted
}

func (c *Cache) Keys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	keys := make([]string, 0, len(c.items))
	for _, elem := range c.items {
		it := elem.Value.(*item)
		if !it.isExpired() {
			keys = append(keys, it.key)
		}
	}
	return keys
}

func (c *Cache) Stats() Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stats
}

func (c *Cache) Bytes() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.curBytes
}

func (i *item) isExpired() bool {
	if i.expiresAt.IsZero() {
		return false
	}
	return time.Now().After(i.expiresAt)
}

// Must hold lock.
func (c *Cache) removeElement(elem *list.Element) {
	c.order.Remove(elem)
	it := elem.Value.(*item)
	delete(c.items, it.key)
	c.curBytes -= it.size
	c.markDirty()
}

// Must hold lock.
func (c *Cache) evict() {
	elem := c.order.Back()
	if elem == nil {
		return
	}

	it := elem.Value.(*item)

	c.removeElement(elem)
	c.stats.Evictions++

	if c.onEvict != nil {
		c.onEvict(it.key, it.value)
	}
}

// markDirty sets the dirty flag and triggers immediate sync if configured.
func (c *Cache) markDirty() {
	if c.filePath == "" {
		return
	}
	c.dirty.Store(true)
	if c.syncStrategy == SyncImmediate {
		go c.Flush() // Non-blocking
	}
}

// Flush writes the cache contents to disk immediately.
// Returns nil if no file path is configured.
func (c *Cache) Flush() error {
	if c.filePath == "" {
		return nil
	}

	// Prevent concurrent flushes
	if !c.flushing.CompareAndSwap(false, true) {
		return nil
	}
	defer c.flushing.Store(false)

	c.mu.RLock()
	items := make([]*fileItem, 0, len(c.items))
	for _, elem := range c.items {
		it := elem.Value.(*item)
		if !it.isExpired() {
			items = append(items, &fileItem{
				Key:       it.key,
				Value:     it.value,
				ExpiresAt: it.expiresAt,
			})
		}
	}
	c.mu.RUnlock()

	if err := saveToFile(c.filePath, items); err != nil {
		return err
	}

	c.dirty.Store(false)
	return nil
}

// Load reads cache contents from disk, replacing current contents.
// Returns nil if no file path is configured or file doesn't exist.
func (c *Cache) Load() error {
	if c.filePath == "" {
		return nil
	}

	items, err := loadFromFile(c.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Clear existing data
	c.items = make(map[string]*list.Element)
	c.order.Init()
	c.curBytes = 0

	// Load items from file
	for _, fi := range items {
		size := len(fi.Key) + len(fi.Value)
		it := &item{
			key:       fi.Key,
			value:     fi.Value,
			size:      size,
			expiresAt: fi.ExpiresAt,
		}
		elem := c.order.PushFront(it)
		c.items[fi.Key] = elem
		c.curBytes += size
	}

	c.dirty.Store(false)
	return nil
}

// Close stops the periodic sync goroutine (if running) and performs a final flush.
// Always call Close when done with a cache that has persistence enabled.
func (c *Cache) Close() error {
	c.closeOnce.Do(func() {
		if c.stopSync != nil {
			close(c.stopSync)
		}
	})

	return c.Flush()
}

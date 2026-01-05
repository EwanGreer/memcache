package memcache

import (
	"container/list"
	"sync"
	"time"
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
	MaxItems int                        // 0 means unlimited
	MaxBytes int                        // 0 means unlimited
	OnEvict  func(key string, value []byte)
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
}

func New() *Cache {
	return NewWithOptions(Options{})
}

// NewWithMaxSize creates a cache. A maxItems of 0 means unlimited.
func NewWithMaxSize(maxItems int) *Cache {
	return NewWithOptions(Options{MaxItems: maxItems})
}

func NewWithOptions(opts Options) *Cache {
	return &Cache{
		items:    make(map[string]*list.Element),
		order:    list.New(),
		maxItems: opts.MaxItems,
		maxBytes: opts.MaxBytes,
		onEvict:  opts.OnEvict,
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
func (c *Cache) GetOrSet(key string, fn func() []byte) []byte {
	return c.GetOrSetWithTTL(key, fn, 0)
}

// GetOrSetWithTTL returns the value for key if it exists, otherwise calls fn and stores the result.
func (c *Cache) GetOrSetWithTTL(key string, fn func() []byte, ttl time.Duration) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		it := elem.Value.(*item)
		if !it.isExpired() {
			c.stats.Hits++
			c.order.MoveToFront(elem)
			return it.value
		}
		c.removeElement(elem)
	}

	c.stats.Misses++
	value := fn()
	c.setInternal(key, value, ttl)
	return value
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

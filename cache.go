package memcache

import (
	"container/list"
	"sync"
	"time"
)

type item struct {
	key       string
	value     []byte
	expiresAt time.Time
}

type Cache struct {
	mu sync.RWMutex
	// items maps keys to their corresponding element in the order list.
	// The actual data lives in order; this map just provides O(1) lookup.
	items   map[string]*list.Element
	order   *list.List // front = most recent, back = least recent
	maxSize int        // 0 means unlimited
}

func New() *Cache {
	return NewWithMaxSize(0)
}

// NewWithMaxSize creates a cache. A maxSize of 0 means unlimited.
func NewWithMaxSize(maxSize int) *Cache {
	return &Cache{
		items:   make(map[string]*list.Element),
		order:   list.New(),
		maxSize: maxSize,
	}
}

// Get returns the value for key. Expired items are deleted lazily.
func (c *Cache) Get(key string) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.items[key]
	if !ok {
		return nil
	}

	it := elem.Value.(*item)
	if it.isExpired() {
		c.order.Remove(elem)
		delete(c.items, key)
		return nil
	}

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

	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	}

	if elem, ok := c.items[key]; ok {
		it := elem.Value.(*item)
		it.value = value
		it.expiresAt = expiresAt
		c.order.MoveToFront(elem)
		return
	}

	if c.maxSize > 0 && c.order.Len() >= c.maxSize {
		c.evict()
	}

	it := &item{
		key:       key,
		value:     value,
		expiresAt: expiresAt,
	}
	elem := c.order.PushFront(it)

	c.items[key] = elem
}

func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.order.Remove(elem)
		delete(c.items, key)
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
		c.order.Remove(elem)
		delete(c.items, key)
		return false
	}

	return true
}

func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*list.Element)
	c.order.Init()
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
	for key, elem := range c.items {
		it := elem.Value.(*item)
		if it.isExpired() {
			c.order.Remove(elem)
			delete(c.items, key)
			deleted++
		}
	}
	return deleted
}

func (i *item) isExpired() bool {
	if i.expiresAt.IsZero() {
		return false
	}
	return time.Now().After(i.expiresAt)
}

func (c *Cache) evict() {
	elem := c.order.Back()
	if elem == nil {
		return
	}
	c.order.Remove(elem)
	it := elem.Value.(*item)
	delete(c.items, it.key)
}

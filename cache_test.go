package memcache

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	c := New()
	if c == nil {
		t.Fatal("expected non-nil cache")
	}
	if c.Len() != 0 {
		t.Errorf("expected empty cache, got %d items", c.Len())
	}
}

func TestSetAndGet(t *testing.T) {
	c := New()
	key := "foo"
	value := []byte("bar")

	c.Set(key, value)

	got := c.Get(key)
	if !bytes.Equal(got, value) {
		t.Errorf("Get(%q) = %q, want %q", key, got, value)
	}
}

func TestGetMissing(t *testing.T) {
	c := New()

	got := c.Get("nonexistent")
	if got != nil {
		t.Errorf("Get(nonexistent) = %q, want nil", got)
	}
}

func TestSetOverwrite(t *testing.T) {
	c := New()
	key := "foo"

	c.Set(key, []byte("first"))
	c.Set(key, []byte("second"))

	got := c.Get(key)
	if !bytes.Equal(got, []byte("second")) {
		t.Errorf("Get(%q) = %q, want %q", key, got, "second")
	}
}

func TestDelete(t *testing.T) {
	c := New()
	key := "foo"

	c.Set(key, []byte("bar"))
	c.Delete(key)

	if c.Has(key) {
		t.Error("expected key to be deleted")
	}
	if got := c.Get(key); got != nil {
		t.Errorf("Get after delete = %q, want nil", got)
	}
}

func TestDeleteNonexistent(t *testing.T) {
	c := New()
	c.Delete("nonexistent") // should not panic
}

func TestHas(t *testing.T) {
	c := New()

	if c.Has("foo") {
		t.Error("Has(foo) = true for empty cache")
	}

	c.Set("foo", []byte("bar"))

	if !c.Has("foo") {
		t.Error("Has(foo) = false after Set")
	}
}

func TestClear(t *testing.T) {
	c := New()

	c.Set("a", []byte("1"))
	c.Set("b", []byte("2"))
	c.Set("c", []byte("3"))

	c.Clear()

	if c.Len() != 0 {
		t.Errorf("Len after Clear = %d, want 0", c.Len())
	}
	if c.Has("a") || c.Has("b") || c.Has("c") {
		t.Error("expected all keys to be cleared")
	}
}

func TestLen(t *testing.T) {
	c := New()

	if c.Len() != 0 {
		t.Errorf("Len = %d, want 0", c.Len())
	}

	c.Set("a", []byte("1"))
	if c.Len() != 1 {
		t.Errorf("Len = %d, want 1", c.Len())
	}

	c.Set("b", []byte("2"))
	if c.Len() != 2 {
		t.Errorf("Len = %d, want 2", c.Len())
	}

	c.Delete("a")
	if c.Len() != 1 {
		t.Errorf("Len = %d, want 1", c.Len())
	}
}

func TestSetWithTTL(t *testing.T) {
	c := New()

	c.SetWithTTL("foo", []byte("bar"), 50*time.Millisecond)

	if got := c.Get("foo"); !bytes.Equal(got, []byte("bar")) {
		t.Errorf("Get before expiry = %q, want %q", got, "bar")
	}

	time.Sleep(60 * time.Millisecond)

	if got := c.Get("foo"); got != nil {
		t.Errorf("Get after expiry = %q, want nil", got)
	}
}

func TestExpiredItemDeletedOnGet(t *testing.T) {
	c := New()

	c.SetWithTTL("foo", []byte("bar"), 10*time.Millisecond)
	time.Sleep(20 * time.Millisecond)

	// Access should delete the expired item
	c.Get("foo")

	c.mu.Lock()
	_, exists := c.items["foo"]
	c.mu.Unlock()

	if exists {
		t.Error("expired item should be deleted on Get")
	}
}

func TestExpiredItemDeletedOnHas(t *testing.T) {
	c := New()

	c.SetWithTTL("foo", []byte("bar"), 10*time.Millisecond)
	time.Sleep(20 * time.Millisecond)

	if c.Has("foo") {
		t.Error("Has should return false for expired item")
	}

	c.mu.Lock()
	_, exists := c.items["foo"]
	c.mu.Unlock()

	if exists {
		t.Error("expired item should be deleted on Has")
	}
}

func TestLenExcludesExpired(t *testing.T) {
	c := New()

	c.Set("permanent", []byte("value"))
	c.SetWithTTL("temporary", []byte("value"), 10*time.Millisecond)

	if c.Len() != 2 {
		t.Errorf("Len = %d, want 2", c.Len())
	}

	time.Sleep(20 * time.Millisecond)

	if c.Len() != 1 {
		t.Errorf("Len after expiry = %d, want 1", c.Len())
	}
}

func TestDeleteExpired(t *testing.T) {
	c := New()

	c.Set("permanent", []byte("value"))
	c.SetWithTTL("temp1", []byte("value"), 10*time.Millisecond)
	c.SetWithTTL("temp2", []byte("value"), 10*time.Millisecond)

	time.Sleep(20 * time.Millisecond)

	deleted := c.DeleteExpired()

	if deleted != 2 {
		t.Errorf("DeleteExpired = %d, want 2", deleted)
	}

	c.mu.Lock()
	itemCount := len(c.items)
	c.mu.Unlock()

	if itemCount != 1 {
		t.Errorf("items remaining = %d, want 1", itemCount)
	}
}

func TestConcurrentAccess(t *testing.T) {
	c := New()
	var wg sync.WaitGroup

	// Concurrent writes
	for i := range 100 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := string(rune('a' + n%26))
			c.Set(key, []byte{byte(n)})
		}(i)
	}

	// Concurrent reads
	for i := range 100 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := string(rune('a' + n%26))
			c.Get(key)
			c.Has(key)
		}(i)
	}

	wg.Wait()
}

func TestNewWithMaxSize(t *testing.T) {
	c := NewWithMaxSize(5)
	if c == nil {
		t.Fatal("expected non-nil cache")
	}
	if c.maxItems != 5 {
		t.Errorf("maxItems = %d, want 5", c.maxItems)
	}
}

func TestMaxSizeEviction(t *testing.T) {
	c := NewWithMaxSize(3)

	c.Set("a", []byte("1"))
	c.Set("b", []byte("2"))
	c.Set("c", []byte("3"))

	// Cache is now full, adding "d" should evict "a" (least recently used)
	c.Set("d", []byte("4"))

	if c.Has("a") {
		t.Error("expected 'a' to be evicted")
	}
	if !c.Has("b") || !c.Has("c") || !c.Has("d") {
		t.Error("expected 'b', 'c', 'd' to still exist")
	}

	c.mu.Lock()
	itemCount := len(c.items)
	c.mu.Unlock()

	if itemCount != 3 {
		t.Errorf("item count = %d, want 3", itemCount)
	}
}

func TestLRUOrderUpdatedOnGet(t *testing.T) {
	c := NewWithMaxSize(3)

	c.Set("a", []byte("1"))
	c.Set("b", []byte("2"))
	c.Set("c", []byte("3"))

	// Access "a" to make it most recently used
	c.Get("a")

	// Adding "d" should now evict "b" (least recently used)
	c.Set("d", []byte("4"))

	if c.Has("b") {
		t.Error("expected 'b' to be evicted")
	}
	if !c.Has("a") || !c.Has("c") || !c.Has("d") {
		t.Error("expected 'a', 'c', 'd' to still exist")
	}
}

func TestLRUOrderUpdatedOnSet(t *testing.T) {
	c := NewWithMaxSize(3)

	c.Set("a", []byte("1"))
	c.Set("b", []byte("2"))
	c.Set("c", []byte("3"))

	// Update "a" to make it most recently used
	c.Set("a", []byte("updated"))

	// Adding "d" should now evict "b" (least recently used)
	c.Set("d", []byte("4"))

	if c.Has("b") {
		t.Error("expected 'b' to be evicted")
	}
	if !c.Has("a") {
		t.Error("expected 'a' to still exist")
	}
	if got := c.Get("a"); !bytes.Equal(got, []byte("updated")) {
		t.Errorf("Get(a) = %q, want %q", got, "updated")
	}
}

func TestMaxSizeZeroMeansUnlimited(t *testing.T) {
	c := NewWithMaxSize(0)

	for i := range 1000 {
		c.Set(string(rune(i)), []byte{byte(i)})
	}

	c.mu.Lock()
	itemCount := len(c.items)
	c.mu.Unlock()

	if itemCount != 1000 {
		t.Errorf("item count = %d, want 1000", itemCount)
	}
}

func TestMaxSizeWithConcurrentAccess(t *testing.T) {
	c := NewWithMaxSize(100)
	var wg sync.WaitGroup

	for i := range 500 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := string(rune(n))
			c.Set(key, []byte{byte(n)})
			c.Get(key)
		}(i)
	}

	wg.Wait()

	c.mu.Lock()
	itemCount := len(c.items)
	c.mu.Unlock()

	if itemCount > 100 {
		t.Errorf("item count = %d, want <= 100", itemCount)
	}
}

func TestSetWithTTLOverwrite(t *testing.T) {
	tests := []struct {
		name        string
		initialTTL  time.Duration
		newTTL      time.Duration
		waitBetween time.Duration
		waitAfter   time.Duration
		expectValue bool
		description string
	}{
		{
			name:        "extend TTL",
			initialTTL:  30 * time.Millisecond,
			newTTL:      100 * time.Millisecond,
			waitBetween: 0,
			waitAfter:   50 * time.Millisecond,
			expectValue: true,
			description: "overwriting with longer TTL should extend expiry",
		},
		{
			name:        "shorten TTL",
			initialTTL:  100 * time.Millisecond,
			newTTL:      20 * time.Millisecond,
			waitBetween: 0,
			waitAfter:   30 * time.Millisecond,
			expectValue: false,
			description: "overwriting with shorter TTL should shorten expiry",
		},
		{
			name:        "refresh TTL",
			initialTTL:  50 * time.Millisecond,
			newTTL:      50 * time.Millisecond,
			waitBetween: 30 * time.Millisecond,
			waitAfter:   30 * time.Millisecond,
			expectValue: true,
			description: "overwriting should reset TTL from current time",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New()

			c.SetWithTTL("key", []byte("initial"), tt.initialTTL)

			if tt.waitBetween > 0 {
				time.Sleep(tt.waitBetween)
			}

			c.SetWithTTL("key", []byte("updated"), tt.newTTL)

			time.Sleep(tt.waitAfter)

			got := c.Get("key")
			if tt.expectValue && got == nil {
				t.Errorf("%s: expected value, got nil", tt.description)
			}
			if !tt.expectValue && got != nil {
				t.Errorf("%s: expected nil, got %q", tt.description, got)
			}
			if tt.expectValue && !bytes.Equal(got, []byte("updated")) {
				t.Errorf("%s: expected 'updated', got %q", tt.description, got)
			}
		})
	}
}

func TestSetWithTTLZeroMeansNoExpiration(t *testing.T) {
	c := New()

	c.SetWithTTL("key", []byte("value"), 0)

	// Wait longer than any reasonable TTL
	time.Sleep(50 * time.Millisecond)

	if got := c.Get("key"); !bytes.Equal(got, []byte("value")) {
		t.Errorf("TTL=0 should not expire, got %v", got)
	}

	// Verify expiresAt is zero
	c.mu.Lock()
	elem := c.items["key"]
	it := elem.Value.(*item)
	isZero := it.expiresAt.IsZero()
	c.mu.Unlock()

	if !isZero {
		t.Error("TTL=0 should result in zero expiresAt")
	}
}

func TestTTLTransitions(t *testing.T) {
	tests := []struct {
		name        string
		firstTTL    time.Duration
		secondTTL   time.Duration
		wait        time.Duration
		expectValue bool
	}{
		{
			name:        "non-TTL to TTL",
			firstTTL:    0,
			secondTTL:   20 * time.Millisecond,
			wait:        30 * time.Millisecond,
			expectValue: false,
		},
		{
			name:        "TTL to non-TTL",
			firstTTL:    20 * time.Millisecond,
			secondTTL:   0,
			wait:        30 * time.Millisecond,
			expectValue: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New()

			c.SetWithTTL("key", []byte("first"), tt.firstTTL)
			c.SetWithTTL("key", []byte("second"), tt.secondTTL)

			time.Sleep(tt.wait)

			got := c.Get("key")
			if tt.expectValue && got == nil {
				t.Errorf("expected value to exist")
			}
			if !tt.expectValue && got != nil {
				t.Errorf("expected value to be expired, got %q", got)
			}
		})
	}
}

func TestHasDoesNotUpdateLRUOrder(t *testing.T) {
	c := NewWithMaxSize(3)

	c.Set("a", []byte("1"))
	c.Set("b", []byte("2"))
	c.Set("c", []byte("3"))

	// Has() on "a" should NOT make it recently used
	if !c.Has("a") {
		t.Fatal("expected 'a' to exist")
	}

	// Adding "d" should evict "a" since Has() didn't update LRU order
	c.Set("d", []byte("4"))

	if c.Has("a") {
		t.Error("expected 'a' to be evicted - Has() should not update LRU order")
	}
	if !c.Has("b") || !c.Has("c") || !c.Has("d") {
		t.Error("expected 'b', 'c', 'd' to still exist")
	}
}

func TestDeleteExpiredWhenNothingExpired(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*Cache)
		expected int
	}{
		{
			name:     "empty cache",
			setup:    func(c *Cache) {},
			expected: 0,
		},
		{
			name: "only non-TTL items",
			setup: func(c *Cache) {
				c.Set("a", []byte("1"))
				c.Set("b", []byte("2"))
			},
			expected: 0,
		},
		{
			name: "TTL items not yet expired",
			setup: func(c *Cache) {
				c.SetWithTTL("a", []byte("1"), time.Hour)
				c.SetWithTTL("b", []byte("2"), time.Hour)
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New()
			tt.setup(c)

			deleted := c.DeleteExpired()
			if deleted != tt.expected {
				t.Errorf("DeleteExpired() = %d, want %d", deleted, tt.expected)
			}
		})
	}
}

func TestConcurrentDelete(t *testing.T) {
	c := New()
	var wg sync.WaitGroup

	// Pre-populate
	for i := range 100 {
		c.Set(string(rune(i)), []byte{byte(i)})
	}

	// Concurrent deletes
	for i := range 100 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			c.Delete(string(rune(n)))
		}(i)
	}

	// Concurrent sets (some will be re-adding deleted keys)
	for i := range 100 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			c.Set(string(rune(n)), []byte{byte(n)})
		}(i)
	}

	wg.Wait()
	// No panic or race = success
}

func TestConcurrentDeleteExpired(t *testing.T) {
	c := New()
	var wg sync.WaitGroup

	// Concurrent operations mixing Set, SetWithTTL, and DeleteExpired
	for i := range 50 {
		wg.Add(3)

		go func(n int) {
			defer wg.Done()
			c.SetWithTTL(string(rune(n)), []byte{byte(n)}, time.Millisecond)
		}(i)

		go func(n int) {
			defer wg.Done()
			c.Set(string(rune(n+1000)), []byte{byte(n)})
		}(i)

		go func() {
			defer wg.Done()
			c.DeleteExpired()
		}()
	}

	wg.Wait()
	// No panic or race = success
}

func TestOnEvictCallback(t *testing.T) {
	var evicted []string
	c := NewWithOptions(Options{
		MaxItems: 2,
		OnEvict: func(key string, value []byte) {
			evicted = append(evicted, key)
		},
	})

	c.Set("a", []byte("1"))
	c.Set("b", []byte("2"))
	c.Set("c", []byte("3")) // evicts "a"

	if len(evicted) != 1 || evicted[0] != "a" {
		t.Errorf("expected evicted=[a], got %v", evicted)
	}
}

func TestStats(t *testing.T) {
	c := NewWithMaxSize(2)

	c.Set("a", []byte("1"))
	c.Get("a")       // hit
	c.Get("a")       // hit
	c.Get("missing") // miss
	c.Set("b", []byte("2"))
	c.Set("c", []byte("3")) // evicts "a"

	stats := c.Stats()
	if stats.Hits != 2 {
		t.Errorf("Hits = %d, want 2", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("Misses = %d, want 1", stats.Misses)
	}
	if stats.Evictions != 1 {
		t.Errorf("Evictions = %d, want 1", stats.Evictions)
	}
}

func TestKeys(t *testing.T) {
	c := New()

	c.Set("a", []byte("1"))
	c.Set("b", []byte("2"))
	c.SetWithTTL("c", []byte("3"), 10*time.Millisecond)

	keys := c.Keys()
	if len(keys) != 3 {
		t.Errorf("len(Keys()) = %d, want 3", len(keys))
	}

	time.Sleep(20 * time.Millisecond)

	keys = c.Keys()
	if len(keys) != 2 {
		t.Errorf("len(Keys()) after expiry = %d, want 2", len(keys))
	}
}

func TestSetNX(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*Cache)
		key      string
		value    []byte
		expected bool
	}{
		{
			name:     "set on empty cache",
			setup:    func(c *Cache) {},
			key:      "a",
			value:    []byte("1"),
			expected: true,
		},
		{
			name: "key exists",
			setup: func(c *Cache) {
				c.Set("a", []byte("existing"))
			},
			key:      "a",
			value:    []byte("new"),
			expected: false,
		},
		{
			name: "key expired",
			setup: func(c *Cache) {
				c.SetWithTTL("a", []byte("old"), time.Millisecond)
				time.Sleep(5 * time.Millisecond)
			},
			key:      "a",
			value:    []byte("new"),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New()
			tt.setup(c)

			got := c.SetNX(tt.key, tt.value)
			if got != tt.expected {
				t.Errorf("SetNX() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetOrSet(t *testing.T) {
	c := New()
	calls := 0

	fn := func() []byte {
		calls++
		return []byte("computed")
	}

	// First call computes
	v1 := c.GetOrSet("key", fn)
	if !bytes.Equal(v1, []byte("computed")) {
		t.Errorf("first GetOrSet = %q, want 'computed'", v1)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}

	// Second call returns cached
	v2 := c.GetOrSet("key", fn)
	if !bytes.Equal(v2, []byte("computed")) {
		t.Errorf("second GetOrSet = %q, want 'computed'", v2)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (should not call fn again)", calls)
	}

	stats := c.Stats()
	if stats.Hits != 1 || stats.Misses != 1 {
		t.Errorf("Stats = %+v, want Hits=1 Misses=1", stats)
	}
}

func TestGetOrSetWithExpiry(t *testing.T) {
	c := New()
	calls := 0

	fn := func() []byte {
		calls++
		return []byte("computed")
	}

	c.GetOrSetWithTTL("key", fn, 10*time.Millisecond)
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}

	time.Sleep(20 * time.Millisecond)

	c.GetOrSetWithTTL("key", fn, 10*time.Millisecond)
	if calls != 2 {
		t.Errorf("calls = %d, want 2 (should recompute after expiry)", calls)
	}
}

func TestMaxBytes(t *testing.T) {
	c := NewWithOptions(Options{MaxBytes: 20})

	c.Set("a", []byte("12345")) // 1 + 5 = 6 bytes
	c.Set("b", []byte("12345")) // 1 + 5 = 6 bytes, total 12
	c.Set("c", []byte("12345")) // 1 + 5 = 6 bytes, total 18
	c.Set("d", []byte("12345")) // would be 24, evicts "a", total 18

	if c.Has("a") {
		t.Error("expected 'a' to be evicted")
	}
	if !c.Has("b") || !c.Has("c") || !c.Has("d") {
		t.Error("expected 'b', 'c', 'd' to exist")
	}

	if c.Bytes() > 20 {
		t.Errorf("Bytes() = %d, want <= 20", c.Bytes())
	}
}

func TestBytesTracking(t *testing.T) {
	c := New()

	c.Set("key", []byte("value")) // 3 + 5 = 8
	if c.Bytes() != 8 {
		t.Errorf("Bytes() = %d, want 8", c.Bytes())
	}

	c.Set("key", []byte("newvalue")) // 3 + 8 = 11
	if c.Bytes() != 11 {
		t.Errorf("Bytes() after update = %d, want 11", c.Bytes())
	}

	c.Delete("key")
	if c.Bytes() != 0 {
		t.Errorf("Bytes() after delete = %d, want 0", c.Bytes())
	}
}

func TestNewWithOptions(t *testing.T) {
	evicted := false
	c := NewWithOptions(Options{
		MaxItems: 10,
		MaxBytes: 1000,
		OnEvict: func(key string, value []byte) {
			evicted = true
		},
	})

	if c.maxItems != 10 {
		t.Errorf("maxItems = %d, want 10", c.maxItems)
	}
	if c.maxBytes != 1000 {
		t.Errorf("maxBytes = %d, want 1000", c.maxBytes)
	}

	c.Set("a", []byte("1"))
	c.Set("b", []byte("2"))
	c.maxItems = 1 // force eviction on next set
	c.Set("c", []byte("3"))

	if !evicted {
		t.Error("expected OnEvict to be called")
	}
}

func TestGetOrSet_Singleflight(t *testing.T) {
	c := New()
	var calls int64
	var wg sync.WaitGroup

	fn := func() []byte {
		atomic.AddInt64(&calls, 1)
		time.Sleep(50 * time.Millisecond)
		return []byte("computed")
	}

	// Launch 10 concurrent requests for the same key
	for range 10 {
		wg.Go(func() {
			c.GetOrSet("key", fn)
		})
	}
	wg.Wait()

	// Singleflight should dedupe to 1 call
	if calls != 1 {
		t.Errorf("expected 1 call, got %d (singleflight not working)", calls)
	}

	// All should get the same value
	if v := c.Get("key"); !bytes.Equal(v, []byte("computed")) {
		t.Errorf("expected 'computed', got %q", v)
	}
}

func TestGetOrSetWithContext_Success(t *testing.T) {
	c := New()
	ctx := context.Background()

	fn := func(ctx context.Context) ([]byte, error) {
		return []byte("result"), nil
	}

	val, err := c.GetOrSetWithContext(ctx, "key", fn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(val, []byte("result")) {
		t.Errorf("expected 'result', got %q", val)
	}
}

func TestGetOrSetWithContext_Cancellation(t *testing.T) {
	c := New()
	ctx, cancel := context.WithCancel(context.Background())

	fn := func(ctx context.Context) ([]byte, error) {
		time.Sleep(100 * time.Millisecond)
		return []byte("result"), nil
	}

	// Cancel context before fn completes
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	_, err := c.GetOrSetWithContext(ctx, "key", fn)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestGetOrSetWithContext_Timeout(t *testing.T) {
	c := New()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	fn := func(ctx context.Context) ([]byte, error) {
		time.Sleep(100 * time.Millisecond)
		return []byte("result"), nil
	}

	_, err := c.GetOrSetWithContext(ctx, "key", fn)
	if err != context.DeadlineExceeded {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestGetOrSetWithContext_FnError(t *testing.T) {
	c := New()
	ctx := context.Background()
	expectedErr := fmt.Errorf("fetch failed")

	fn := func(ctx context.Context) ([]byte, error) {
		return nil, expectedErr
	}

	_, err := c.GetOrSetWithContext(ctx, "key", fn)
	if err != expectedErr {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
}

func TestGetOrSetWithContext_CacheHit(t *testing.T) {
	c := New()
	c.Set("key", []byte("cached"))
	ctx := context.Background()

	calls := 0
	fn := func(ctx context.Context) ([]byte, error) {
		calls++
		return []byte("computed"), nil
	}

	val, err := c.GetOrSetWithContext(ctx, "key", fn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(val, []byte("cached")) {
		t.Errorf("expected 'cached', got %q", val)
	}
	if calls != 0 {
		t.Errorf("expected 0 calls (cache hit), got %d", calls)
	}
}

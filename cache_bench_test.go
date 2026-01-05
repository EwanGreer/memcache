package memcache

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func BenchmarkGet_Hit(b *testing.B) {
	c := New()
	c.Set("key", []byte("value"))

	for b.Loop() {
		c.Get("key")
	}
}

func BenchmarkGet_Miss(b *testing.B) {
	c := New()

	for b.Loop() {
		c.Get("missing")
	}
}

func BenchmarkSet(b *testing.B) {
	c := New()
	value := []byte("value")

	for b.Loop() {
		c.Set("key", value)
	}
}

func BenchmarkSet_Unique(b *testing.B) {
	c := New()
	value := []byte("value")

	for i := 0; b.Loop(); i++ {
		c.Set(fmt.Sprintf("key%d", i), value)
	}
}

func BenchmarkSetWithTTL(b *testing.B) {
	c := New()
	value := []byte("value")
	ttl := time.Hour

	for b.Loop() {
		c.SetWithTTL("key", value, ttl)
	}
}

func BenchmarkGetOrSet_Hit(b *testing.B) {
	c := New()
	c.Set("key", []byte("value"))
	fn := func() []byte { return []byte("computed") }

	for b.Loop() {
		c.GetOrSet("key", fn)
	}
}

func BenchmarkGetOrSet_Miss(b *testing.B) {
	c := New()
	fn := func() []byte { return []byte("computed") }

	for i := 0; b.Loop(); i++ {
		c.GetOrSet(fmt.Sprintf("key%d", i), fn)
	}
}

func BenchmarkGet_Parallel(b *testing.B) {
	c := New()
	c.Set("key", []byte("value"))

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			c.Get("key")
		}
	})
}

func BenchmarkSet_Parallel(b *testing.B) {
	c := New()
	value := []byte("value")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			c.Set(fmt.Sprintf("key%d", i), value)
			i++
		}
	})
}

func BenchmarkGetOrSet_Parallel_SameKey(b *testing.B) {
	c := New()
	fn := func() []byte {
		time.Sleep(time.Microsecond) // simulate work
		return []byte("computed")
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			c.GetOrSet("key", fn)
		}
	})
}

func BenchmarkGetOrSet_Parallel_UniqueKeys(b *testing.B) {
	c := New()
	fn := func() []byte { return []byte("computed") }
	var counter atomic.Int64

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			key := fmt.Sprintf("key%d", counter.Add(1))
			c.GetOrSet(key, fn)
		}
	})
}

// BenchmarkGetOrSet_Contention tests singleflight behavior:
// many goroutines request the same uncached key simultaneously.
// Without singleflight, fn would be called N times.
// With singleflight, fn should be called only once.
func BenchmarkGetOrSet_Contention(b *testing.B) {
	for _, numGoroutines := range []int{10, 100, 1000} {
		b.Run(fmt.Sprintf("goroutines=%d", numGoroutines), func(b *testing.B) {
			for b.Loop() {
				c := New()
				var calls atomic.Int64
				fn := func() []byte {
					calls.Add(1)
					time.Sleep(time.Millisecond)
					return []byte("computed")
				}

				var wg sync.WaitGroup
				wg.Add(numGoroutines)
				for range numGoroutines {
					go func() {
						defer wg.Done()
						c.GetOrSet("contested-key", fn)
					}()
				}
				wg.Wait()

				// Verify singleflight worked
				if calls.Load() != 1 {
					b.Errorf("expected 1 call, got %d", calls.Load())
				}
			}
		})
	}
}

func BenchmarkMixed_ReadHeavy(b *testing.B) {
	c := New()
	for i := range 1000 {
		c.Set(fmt.Sprintf("key%d", i), []byte("value"))
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%10 == 0 {
				c.Set(fmt.Sprintf("key%d", i%1000), []byte("newvalue"))
			} else {
				c.Get(fmt.Sprintf("key%d", i%1000))
			}
			i++
		}
	})
}

func BenchmarkMixed_WriteHeavy(b *testing.B) {
	c := New()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%10 == 0 {
				c.Get(fmt.Sprintf("key%d", i%1000))
			} else {
				c.Set(fmt.Sprintf("key%d", i%1000), []byte("value"))
			}
			i++
		}
	})
}

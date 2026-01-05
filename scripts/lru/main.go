package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/EwanGreer/memcache"
)

func show(c *memcache.Cache) {
	keys := c.Keys()
	fmt.Printf("  cache: [%s]\n", strings.Join(keys, ", "))
}

func main() {
	fmt.Println("LRU Eviction Demo")
	fmt.Println("=================")
	fmt.Println("cache max size: 3")

	c := memcache.NewWithOptions(memcache.Options{
		MaxItems: 3,
		OnEvict: func(key string, _ []byte) {
			fmt.Printf("  ! evicted %q\n", key)
		},
	})

	for _, k := range []string{"a", "b", "c"} {
		time.Sleep(300 * time.Millisecond)
		fmt.Printf("\n> Set(%q)\n", k)
		c.Set(k, []byte(k))
		show(c)
	}

	time.Sleep(500 * time.Millisecond)
	fmt.Println("\n> Get(\"a\")  -- moves 'a' to front")
	c.Get("a")
	fmt.Println("  order is now: a (recent) -> c -> b (oldest)")

	time.Sleep(500 * time.Millisecond)
	fmt.Println("\n> Set(\"d\")  -- cache full, must evict")
	c.Set("d", []byte("d"))
	show(c)
}

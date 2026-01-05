package main

import (
	"fmt"
	"time"

	"github.com/EwanGreer/memcache"
)

func stats(c *memcache.Cache) {
	s := c.Stats()
	fmt.Printf("  stats: %d hits, %d misses, %d evictions\n", s.Hits, s.Misses, s.Evictions)
}

func main() {
	fmt.Println("Stats Demo")
	fmt.Println("==========")
	fmt.Println("cache max size: 2")

	c := memcache.NewWithMaxSize(2)

	time.Sleep(300 * time.Millisecond)
	fmt.Println("\n> Set(\"a\")")
	c.Set("a", []byte("1"))
	stats(c)

	time.Sleep(300 * time.Millisecond)
	fmt.Println("\n> Get(\"a\")  -- hit")
	c.Get("a")
	stats(c)

	time.Sleep(300 * time.Millisecond)
	fmt.Println("\n> Get(\"a\")  -- hit")
	c.Get("a")
	stats(c)

	time.Sleep(300 * time.Millisecond)
	fmt.Println("\n> Get(\"x\")  -- miss (doesn't exist)")
	c.Get("x")
	stats(c)

	time.Sleep(300 * time.Millisecond)
	fmt.Println("\n> Set(\"b\")")
	c.Set("b", []byte("2"))
	stats(c)

	time.Sleep(300 * time.Millisecond)
	fmt.Println("\n> Set(\"c\")  -- evicts \"a\"")
	c.Set("c", []byte("3"))
	stats(c)
}

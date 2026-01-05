package main

import (
	"fmt"
	"time"

	"github.com/EwanGreer/memcache"
)

func main() {
	fmt.Println("OnEvict Callback Demo")
	fmt.Println("=====================")
	fmt.Println("log evictions for monitoring/cleanup")

	evictLog := []string{}

	c := memcache.NewWithOptions(memcache.Options{
		MaxItems: 3,
		OnEvict: func(key string, value []byte) {
			evictLog = append(evictLog, key)
			fmt.Printf("  >> evicted %q (was: %s)\n", key, value)
		},
	})

	fmt.Println("\nfilling cache (max 3 items):")
	for _, k := range []string{"a", "b", "c"} {
		time.Sleep(300 * time.Millisecond)
		fmt.Printf("> Set(%q)\n", k)
		c.Set(k, []byte(fmt.Sprintf("value_%s", k)))
	}

	fmt.Println("\nadding more items (triggers evictions):")
	for _, k := range []string{"d", "e", "f"} {
		time.Sleep(400 * time.Millisecond)
		fmt.Printf("> Set(%q)\n", k)
		c.Set(k, []byte(fmt.Sprintf("value_%s", k)))
	}

	time.Sleep(300 * time.Millisecond)
	fmt.Printf("\neviction log: %v\n", evictLog)
	fmt.Printf("final keys:   %v\n", c.Keys())
}

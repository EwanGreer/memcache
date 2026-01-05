package main

import (
	"fmt"
	"time"

	"github.com/EwanGreer/memcache"
)

func main() {
	fmt.Println("GetOrSet Demo")
	fmt.Println("=============")
	fmt.Println("simulates fetching user data from a slow database")

	c := memcache.New()

	fetch := func() []byte {
		fmt.Print("  [db] fetching")
		for i := 0; i < 3; i++ {
			time.Sleep(200 * time.Millisecond)
			fmt.Print(".")
		}
		fmt.Println(" done")
		return []byte(`{"name":"alice","id":1}`)
	}

	time.Sleep(300 * time.Millisecond)
	fmt.Println("\n> GetOrSet(\"user:1\", fetch)")
	v1 := c.GetOrSet("user:1", fetch)
	fmt.Printf("  result: %s\n", v1)

	time.Sleep(500 * time.Millisecond)
	fmt.Println("\n> GetOrSet(\"user:1\", fetch)  -- cached, no fetch")
	v2 := c.GetOrSet("user:1", fetch)
	fmt.Printf("  result: %s\n", v2)

	time.Sleep(500 * time.Millisecond)
	fmt.Println("\n> GetOrSet(\"user:2\", fetch)  -- different key")
	v3 := c.GetOrSet("user:2", fetch)
	fmt.Printf("  result: %s\n", v3)

	s := c.Stats()
	fmt.Printf("\nfinal stats: %d hits, %d misses\n", s.Hits, s.Misses)
}

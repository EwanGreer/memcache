package main

import (
	"fmt"
	"time"

	"github.com/EwanGreer/memcache"
)

func main() {
	fmt.Println("DeleteExpired Demo")
	fmt.Println("==================")
	fmt.Println("bulk cleanup of expired items")

	c := memcache.New()

	fmt.Println("\n> adding 5 items with 1s TTL")
	for _, k := range []string{"a", "b", "c", "d", "e"} {
		c.SetWithTTL(k, []byte(k), 1*time.Second)
		fmt.Printf("  Set(%q)\n", k)
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Printf("\n> Len() = %d\n", c.Len())

	fmt.Print("\nwaiting for expiry")
	for range 5 {
		time.Sleep(400 * time.Millisecond)
		fmt.Print(".")
	}
	fmt.Println()

	fmt.Printf("\n> Len() = %d  (expired but not yet cleaned)\n", c.Len())

	fmt.Println("\n> DeleteExpired()")
	deleted := c.DeleteExpired()
	fmt.Printf("  cleaned up %d items\n", deleted)
}

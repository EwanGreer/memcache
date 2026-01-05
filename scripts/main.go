package main

import (
	"fmt"
	"time"

	"github.com/EwanGreer/memcache"
)

func main() {
	fmt.Println("TTL Expiration Demo")
	fmt.Println("===================")

	c := memcache.New()

	fmt.Println("\n> SetWithTTL(\"token\", \"abc123\", 3s)")
	c.SetWithTTL("token", []byte("abc123"), 3*time.Second)

	fmt.Printf("\n> Get(\"token\") = %q\n", c.Get("token"))

	fmt.Print("\nwaiting")
	for i := 0; i < 4; i++ {
		time.Sleep(time.Second)
		fmt.Print(".")
	}
	fmt.Println()

	fmt.Printf("\n> Get(\"token\") = %v  (expired)\n", c.Get("token"))
}

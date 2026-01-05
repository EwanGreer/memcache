package main

import (
	"fmt"
	"time"

	"github.com/EwanGreer/memcache"
)

func main() {
	c := memcache.New()

	c.SetWithTTL("token", []byte("abc123"), 3*time.Second)
	fmt.Println("stored token with 3s TTL")

	fmt.Println("get:", string(c.Get("token")))

	time.Sleep(4 * time.Second)

	fmt.Println("get after 4s:", string(c.Get("token")))
}

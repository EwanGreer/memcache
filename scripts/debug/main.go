package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/EwanGreer/memcache"
)

func main() {
	c := memcache.NewWithOptions(memcache.Options{
		MaxItems: 50,
		MaxBytes: 4096,
	})

	// Populate with some initial data
	for i := range 20 {
		key := fmt.Sprintf("key:%03d", i)
		val := fmt.Sprintf("value-%d-%s", i, time.Now().Format(time.RFC3339))
		c.Set(key, []byte(val))
	}

	// Add some items with TTLs
	for i := range 10 {
		key := fmt.Sprintf("ttl:%03d", i)
		val := fmt.Sprintf("expires-soon-%d", i)
		c.SetWithTTL(key, []byte(val), time.Duration(10+i*5)*time.Second)
	}

	fmt.Println("Cache populated with 30 items")
	fmt.Println("Starting debug server on :9090")
	fmt.Println("Open http://localhost:9090 in your browser")

	// Background goroutine to simulate cache activity
	go func() {
		for {
			time.Sleep(time.Duration(500+rand.Intn(2000)) * time.Millisecond)
			op := rand.Intn(4)
			switch op {
			case 0: // set
				key := fmt.Sprintf("key:%03d", rand.Intn(100))
				val := fmt.Sprintf("updated-%d", time.Now().UnixMilli())
				c.Set(key, []byte(val))
			case 1: // set with TTL
				key := fmt.Sprintf("ttl:%03d", rand.Intn(50))
				val := fmt.Sprintf("temp-%d", time.Now().UnixMilli())
				c.SetWithTTL(key, []byte(val), time.Duration(5+rand.Intn(30))*time.Second)
			case 2: // get
				key := fmt.Sprintf("key:%03d", rand.Intn(100))
				c.Get(key)
			case 3: // delete
				key := fmt.Sprintf("key:%03d", rand.Intn(100))
				c.Delete(key)
			}
		}
	}()

	if err := memcache.ListenAndServeDebug(c, ":9090"); err != nil {
		fmt.Printf("server error: %v\n", err)
	}
}

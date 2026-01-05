package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/EwanGreer/memcache"
)

func main() {
	fmt.Println("MaxBytes Demo")
	fmt.Println("=============")
	fmt.Println("cache limit: 50 bytes")

	c := memcache.NewWithOptions(memcache.Options{
		MaxBytes: 50,
		OnEvict: func(key string, _ []byte) {
			fmt.Printf("  ! evicted %q to free space\n", key)
		},
	})

	items := []struct {
		key   string
		value string
	}{
		{"a", "hello world"},
		{"b", "foo bar baz"},
		{"c", "this is a longer string value"},
	}

	for _, item := range items {
		time.Sleep(400 * time.Millisecond)
		size := len(item.key) + len(item.value)
		fmt.Printf("\n> Set(%q, %q)  -- %d bytes\n", item.key, item.value, size)
		c.Set(item.key, []byte(item.value))

		bar := strings.Repeat("█", c.Bytes()/2) + strings.Repeat("░", (50-c.Bytes())/2)
		fmt.Printf("  [%s] %d/50 bytes\n", bar, c.Bytes())
		fmt.Printf("  keys: %v\n", c.Keys())
	}
}

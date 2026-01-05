package main

import (
	"fmt"
	"time"

	"github.com/EwanGreer/memcache"
)

func main() {
	fmt.Println("Basic Operations Demo")
	fmt.Println("=====================")

	c := memcache.New()

	time.Sleep(300 * time.Millisecond)
	fmt.Println("\n> Set(\"user:1\", {\"name\":\"alice\"})")
	c.Set("user:1", []byte(`{"name":"alice"}`))

	time.Sleep(300 * time.Millisecond)
	fmt.Printf("\n> Get(\"user:1\") = %s\n", c.Get("user:1"))

	time.Sleep(300 * time.Millisecond)
	fmt.Printf("\n> Has(\"user:1\") = %v\n", c.Has("user:1"))

	time.Sleep(300 * time.Millisecond)
	fmt.Printf("\n> Len() = %d\n", c.Len())

	time.Sleep(300 * time.Millisecond)
	fmt.Println("\n> Delete(\"user:1\")")
	c.Delete("user:1")

	time.Sleep(300 * time.Millisecond)
	fmt.Printf("\n> Get(\"user:1\") = %v\n", c.Get("user:1"))
	fmt.Printf("> Has(\"user:1\") = %v\n", c.Has("user:1"))
}

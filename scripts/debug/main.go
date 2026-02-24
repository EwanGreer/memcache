package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/EwanGreer/memcache"
)

var adjectives = []string{"swift", "lazy", "eager", "quiet", "bold", "calm", "bright", "dark"}
var nouns = []string{"falcon", "panda", "otter", "lynx", "crane", "viper", "gecko", "bison"}

func randWord(list []string) string { return list[rand.Intn(len(list))] }

func userJSON(i int) []byte {
	type Address struct {
		Street string `json:"street"`
		City   string `json:"city"`
		Zip    string `json:"zip"`
	}
	type User struct {
		ID        int      `json:"id"`
		Username  string   `json:"username"`
		Email     string   `json:"email"`
		Score     float64  `json:"score"`
		Active    bool     `json:"active"`
		Tags      []string `json:"tags"`
		Address   Address  `json:"address"`
		CreatedAt string   `json:"created_at"`
	}
	u := User{
		ID:       i,
		Username: randWord(adjectives) + "_" + randWord(nouns),
		Email:    fmt.Sprintf("user%d@example.com", i),
		Score:    float64(rand.Intn(10000)) / 100.0,
		Active:   rand.Intn(2) == 1,
		Tags:     []string{randWord(adjectives), randWord(nouns)},
		Address: Address{
			Street: fmt.Sprintf("%d %s Ave", 100+rand.Intn(900), randWord(nouns)),
			City:   "Memville",
			Zip:    fmt.Sprintf("%05d", rand.Intn(99999)),
		},
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	b, _ := json.Marshal(u)
	return b
}

func sessionJSON(i int) []byte {
	type Session struct {
		Token     string `json:"token"`
		UserID    int    `json:"user_id"`
		ExpiresAt string `json:"expires_at"`
		IP        string `json:"ip"`
		UserAgent string `json:"user_agent"`
	}
	s := Session{
		Token:     fmt.Sprintf("%x", rand.Int63()),
		UserID:    rand.Intn(500),
		ExpiresAt: time.Now().Add(time.Duration(5+rand.Intn(55)) * time.Minute).Format(time.RFC3339),
		IP:        fmt.Sprintf("10.%d.%d.%d", rand.Intn(256), rand.Intn(256), rand.Intn(256)),
		UserAgent: "Mozilla/5.0 (compatible; debug-client/" + fmt.Sprintf("%d", i) + ")",
	}
	b, _ := json.Marshal(s)
	return b
}

func main() {
	c := memcache.NewWithOptions(memcache.Options{
		MaxItems: 50,
		MaxBytes: 4096,
	})

	// Populate with user objects
	for i := range 20 {
		key := fmt.Sprintf("user:%03d", i)
		c.Set(key, userJSON(i))
	}

	// Add session objects with TTLs
	for i := range 10 {
		key := fmt.Sprintf("session:%03d", i)
		c.SetWithTTL(key, sessionJSON(i), time.Duration(10+i*5)*time.Second)
	}

	fmt.Println("Starting memcache debug server")
	fmt.Println("  cache:  50 items  /  4 KB")
	fmt.Println("  url:    http://localhost:9090")

	// Background goroutine to simulate cache activity
	go func() {
		for {
			time.Sleep(time.Duration(500+rand.Intn(2000)) * time.Millisecond)
			op := rand.Intn(4)
			switch op {
			case 0: // set user
				i := rand.Intn(100)
				c.Set(fmt.Sprintf("user:%03d", i), userJSON(i))
			case 1: // set session with TTL
				i := rand.Intn(50)
				c.SetWithTTL(fmt.Sprintf("session:%03d", i), sessionJSON(i), time.Duration(5+rand.Intn(30))*time.Second)
			case 2: // get
				c.Get(fmt.Sprintf("user:%03d", rand.Intn(100)))
			case 3: // delete
				c.Delete(fmt.Sprintf("user:%03d", rand.Intn(100)))
			}
		}
	}()

	if err := memcache.ListenAndServeDebug(c, ":9090"); err != nil {
		fmt.Printf("server error: %v\n", err)
	}
}

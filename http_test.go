package memcache

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDebugStatsJSON(t *testing.T) {
	c := NewWithOptions(Options{MaxItems: 100, MaxBytes: 4096})
	c.Set("key1", []byte("val1"))
	c.Get("key1")
	c.Get("miss")

	handler := DebugHandler(c)

	req := httptest.NewRequest("GET", "/api/stats", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected application/json, got %s", ct)
	}

	var stats statsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if stats.Hits != 1 {
		t.Fatalf("expected 1 hit, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Fatalf("expected 1 miss, got %d", stats.Misses)
	}
	if stats.Items != 1 {
		t.Fatalf("expected 1 item, got %d", stats.Items)
	}
	if stats.MaxItems != 100 {
		t.Fatalf("expected max_items 100, got %d", stats.MaxItems)
	}
	if stats.MaxBytes != 4096 {
		t.Fatalf("expected max_bytes 4096, got %d", stats.MaxBytes)
	}
}

func TestDebugStatsHTML(t *testing.T) {
	c := New()
	handler := DebugHandler(c)

	req := httptest.NewRequest("GET", "/api/stats", nil)
	req.Header.Set("Accept", "text/html")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("expected text/html content type, got %s", w.Header().Get("Content-Type"))
	}
	if !strings.Contains(w.Body.String(), "stats-grid") {
		t.Fatal("expected HTML partial with stats-grid class")
	}
}

func TestDebugItemsJSON(t *testing.T) {
	c := New()
	c.Set("a", []byte("1"))
	c.Set("b", []byte("2"))
	c.Set("c", []byte("3"))

	handler := DebugHandler(c)

	req := httptest.NewRequest("GET", "/api/items", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var items []ItemInfo
	if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	// LRU order: most recent first
	if items[0].Key != "c" {
		t.Fatalf("expected first item 'c', got %s", items[0].Key)
	}
	if items[1].Key != "b" {
		t.Fatalf("expected second item 'b', got %s", items[1].Key)
	}
	if items[2].Key != "a" {
		t.Fatalf("expected third item 'a', got %s", items[2].Key)
	}
}

func TestDebugItemsHTML(t *testing.T) {
	c := New()
	c.Set("key1", []byte("val1"))

	handler := DebugHandler(c)

	req := httptest.NewRequest("GET", "/api/items", nil)
	req.Header.Set("Accept", "text/html")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "<table>") {
		t.Fatal("expected HTML table")
	}
	if !strings.Contains(body, "key1") {
		t.Fatal("expected key1 in table")
	}
}

func TestDebugItemDetail(t *testing.T) {
	c := New()
	c.Set("mykey", []byte("myvalue"))

	handler := DebugHandler(c)

	req := httptest.NewRequest("GET", "/api/items/mykey", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var detail ItemDetail
	if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if detail.Key != "mykey" {
		t.Fatalf("expected key 'mykey', got %s", detail.Key)
	}
	if detail.Value == "" {
		t.Fatal("expected base64-encoded value")
	}
}

func TestDebugItemDetailNotFound(t *testing.T) {
	c := New()
	handler := DebugHandler(c)

	req := httptest.NewRequest("GET", "/api/items/nonexistent", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestDebugItemsWithTTL(t *testing.T) {
	c := New()
	c.SetWithTTL("expiring", []byte("val"), 1*time.Hour)

	handler := DebugHandler(c)

	req := httptest.NewRequest("GET", "/api/items", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	var items []ItemInfo
	json.Unmarshal(w.Body.Bytes(), &items)

	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].ExpiresAt == "" {
		t.Fatal("expected expires_at to be set")
	}
	if items[0].TTLRemaining == "" {
		t.Fatal("expected ttl_remaining to be set")
	}
}

func TestDebugSSE(t *testing.T) {
	c := New()
	handler := DebugHandler(c)

	server := httptest.NewServer(handler)
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/events")
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %s", ct)
	}

	// Set a key to generate an event
	c.Set("test", []byte("data"))

	// Read the SSE data
	buf := make([]byte, 4096)
	n, err := resp.Body.Read(buf)
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}
	body := string(buf[:n])
	if !strings.Contains(body, "data: ") {
		t.Fatalf("expected SSE data line, got: %s", body)
	}
	if !strings.Contains(body, "test") {
		t.Fatalf("expected event with key 'test', got: %s", body)
	}
}

func TestDebugDashboard(t *testing.T) {
	c := New()
	handler := DebugHandler(c)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "memcache debug") {
		t.Fatal("expected dashboard HTML")
	}
	if !strings.Contains(body, "htmx") {
		t.Fatal("expected HTMX reference")
	}
}

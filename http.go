package memcache

import (
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"
)

//go:embed dashboard.html partials
var debugFS embed.FS

// ItemInfo represents cache item metadata for the debug API.
type ItemInfo struct {
	Key          string `json:"key"`
	Size         int    `json:"size"`
	ExpiresAt    string `json:"expires_at,omitempty"`
	TTLRemaining string `json:"ttl_remaining,omitempty"`
}

// ItemDetail includes the value (base64-encoded) along with metadata.
type ItemDetail struct {
	ItemInfo
	Value string `json:"value"` // base64-encoded
}

// snapshot walks the LRU list front-to-back and returns item metadata.
func (c *Cache) snapshot() []ItemInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()

	items := make([]ItemInfo, 0, c.order.Len())
	for e := c.order.Front(); e != nil; e = e.Next() {
		it := e.Value.(*item)
		if it.isExpired() {
			continue
		}

		info := ItemInfo{
			Key:  it.key,
			Size: it.size,
		}

		if !it.expiresAt.IsZero() {
			info.ExpiresAt = it.expiresAt.Format(time.RFC3339)
			remaining := time.Until(it.expiresAt)

			if remaining > 0 {
				info.TTLRemaining = remaining.Truncate(time.Second).String()
			}
		}
		items = append(items, info)
	}
	return items
}

// getItem returns detail for a single cache key.
func (c *Cache) getItem(key string) (*ItemDetail, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	elem, ok := c.items[key]
	if !ok {
		return nil, false
	}

	it := elem.Value.(*item)
	if it.isExpired() {
		return nil, false
	}

	detail := &ItemDetail{
		ItemInfo: ItemInfo{
			Key:  it.key,
			Size: it.size,
		},
		Value: base64.StdEncoding.EncodeToString(it.value),
	}

	if !it.expiresAt.IsZero() {
		detail.ExpiresAt = it.expiresAt.Format(time.RFC3339)
		remaining := time.Until(it.expiresAt)
		if remaining > 0 {
			detail.TTLRemaining = remaining.Truncate(time.Second).String()
		}
	}
	return detail, true
}

type statsResponse struct {
	Hits      int64 `json:"hits"`
	Misses    int64 `json:"misses"`
	Evictions int64 `json:"evictions"`
	Items     int   `json:"items"`
	Bytes     int   `json:"bytes"`
	MaxItems  int   `json:"max_items"`
	MaxBytes  int   `json:"max_bytes"`
}

func (c *Cache) debugStats() statsResponse {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return statsResponse{
		Hits:      c.stats.Hits,
		Misses:    c.stats.Misses,
		Evictions: c.stats.Evictions,
		Items:     c.order.Len(),
		Bytes:     c.curBytes,
		MaxItems:  c.maxItems,
		MaxBytes:  c.maxBytes,
	}
}

func formatBytes(b int) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

var (
	statsPartialTmpl *template.Template
	itemsPartialTmpl *template.Template
)

func init() {
	statsPartialTmpl = template.Must(
		template.New("stats.html").
			Funcs(template.FuncMap{"formatBytes": formatBytes}).
			ParseFS(debugFS, "partials/stats.html"),
	)
	itemsPartialTmpl = template.Must(
		template.New("items.html").ParseFS(debugFS, "partials/items.html"),
	)
}

// DebugHandler returns an http.Handler that serves cache debug endpoints.
func DebugHandler(c *Cache) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/stats", func(w http.ResponseWriter, r *http.Request) {
		stats := c.debugStats()
		if wantsHTML(r) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_ = statsPartialTmpl.Execute(w, stats)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(stats)
	})

	mux.HandleFunc("GET /api/items", func(w http.ResponseWriter, r *http.Request) {
		items := c.snapshot()
		if wantsHTML(r) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_ = itemsPartialTmpl.Execute(w, items)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(items)
	})

	mux.HandleFunc("GET /api/items/{key}", func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("key")
		detail, ok := c.getItem(key)
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(detail)
	})

	mux.HandleFunc("GET /api/events", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		sub := c.Subscribe()
		defer sub.Close()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher.Flush()

		for {
			select {
			case <-r.Context().Done():
				return
			case e, ok := <-sub.C:
				if !ok {
					return
				}
				data, _ := json.Marshal(e)
				_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			}
		}
	})

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		data, err := debugFS.ReadFile("dashboard.html")
		if err != nil {
			http.Error(w, "dashboard not found", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})

	return mux
}

// ListenAndServeDebug starts a standalone HTTP debug server for the cache.
func ListenAndServeDebug(c *Cache, addr string) error {
	return http.ListenAndServe(addr, DebugHandler(c))
}

func wantsHTML(r *http.Request) bool {
	// HTMX always sends HX-Request: true; fall back to Accept header inspection.
	if r.Header.Get("HX-Request") == "true" {
		return true
	}
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

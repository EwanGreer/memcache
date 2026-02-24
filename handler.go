package memcache

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
)

//go:embed dashboard.html partials
var debugFS embed.FS

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

type statsResponse struct {
	Hits      int64 `json:"hits"`
	Misses    int64 `json:"misses"`
	Evictions int64 `json:"evictions"`
	Items     int   `json:"items"`
	Bytes     int   `json:"bytes"`
	MaxItems  int   `json:"max_items"`
	MaxBytes  int   `json:"max_bytes"`
}

type handler struct {
	c *Cache
}

func newHandler(c *Cache) *handler {
	return &handler{
		c: c,
	}
}

func (h *handler) stats(w http.ResponseWriter, r *http.Request) {
	stats := h.c.debugStats()
	if wantsHTML(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = statsPartialTmpl.Execute(w, stats)
		return
	}
	render.JSON(w, r, stats)
}

func (h *handler) items(w http.ResponseWriter, r *http.Request) {
	items := h.c.snapshot()
	if wantsHTML(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = itemsPartialTmpl.Execute(w, items)
		return
	}
	render.JSON(w, r, items)
}

func (h *handler) item(w http.ResponseWriter, r *http.Request) {
	detail, ok := h.c.getItem(chi.URLParam(r, "key"))
	if !ok {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, render.M{"error": "not found"})
		return
	}
	render.JSON(w, r, detail)
}

func (h *handler) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"error": "streaming not supported"})
		return
	}

	sub := h.c.Subscribe()
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
}

func (h *handler) dashboard(w http.ResponseWriter, r *http.Request) {
	data, err := debugFS.ReadFile("dashboard.html")
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"error": "dashboard not found"})
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
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

func wantsHTML(r *http.Request) bool {
	// HTMX always sends HX-Request: true; fall back to Accept header inspection.
	if r.Header.Get("HX-Request") == "true" {
		return true
	}
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

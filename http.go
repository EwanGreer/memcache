package memcache

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// DebugHandler returns an http.Handler that serves cache debug endpoints.
func DebugHandler(c *Cache) http.Handler {
	h := newHandler(c)
	r := chi.NewRouter()

	r.Get("/api/stats", h.stats)
	r.Get("/api/items", h.items)
	r.Get("/api/items/{key}", h.item)
	r.Get("/api/events", h.events)
	r.Get("/", h.dashboard)

	return r
}

// ListenAndServeDebug starts a standalone HTTP debug server for the cache.
func ListenAndServeDebug(c *Cache, addr string) error {
	return http.ListenAndServe(addr, DebugHandler(c))
}

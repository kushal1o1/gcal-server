package server

import (
	"sync"
	"time"
)

// listCache is a tiny TTL cache for GET /api/events responses.
//
// Building the upcoming list expands Google's recurring instances across
// several API pages (each hundreds of ms), so read-heavy flows (opening the
// events screen, refresh, load-more, widget updates) hit the cache instead of
// Google. All event mutations clear it via Clear, so a create/edit/delete is
// visible on the next read.
type listCache struct {
	mu    sync.Mutex
	ttl   time.Duration
	items map[string]cachedResponse
}

type cachedResponse struct {
	body   []byte
	status int
	expiry time.Time
}

func newListCache(ttl time.Duration) *listCache {
	return &listCache{ttl: ttl, items: make(map[string]cachedResponse)}
}

func (c *listCache) Get(key string) ([]byte, int, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.items[key]
	if !ok {
		return nil, 0, false
	}
	if time.Now().After(e.expiry) {
		delete(c.items, key)
		return nil, 0, false
	}
	return e.body, e.status, true
}

func (c *listCache) Put(key string, body []byte, status int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Cap the map so a pathological mix of query params can't grow it unbounded.
	if len(c.items) > 64 {
		for k := range c.items {
			delete(c.items, k)
			break
		}
	}
	c.items[key] = cachedResponse{body: body, status: status, expiry: time.Now().Add(c.ttl)}
}

func (c *listCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]cachedResponse)
}

// defaultListCache powers GET /api/events with a 15s freshness window.
var defaultListCache = newListCache(15 * time.Second)

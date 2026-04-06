// Package webfetch implements the WebFetchTool — the Go port of Claude Code's
// WebFetchTool. It fetches URLs, converts HTML to markdown, and caches results.
package webfetch

import (
	"container/list"
	"sync"
	"time"
)

const (
	cacheTTL      = 15 * time.Minute
	cacheMaxBytes = 50 * 1024 * 1024 // 50MB
)

// cacheEntry is a single cached fetch result.
type cacheEntry struct {
	key       string
	content   string
	expiresAt time.Time
	size      int64
}

// fetchCache is an LRU cache with TTL and total size limit.
// Expired entries are evicted lazily on access, plus proactively on set.
// Thread-safe — all methods are guarded by a mutex.
type fetchCache struct {
	mu       sync.Mutex
	items    map[string]*list.Element
	order    *list.List // front = most recent, back = LRU
	curBytes int64
	maxBytes int64
	ttl      time.Duration
}

// newFetchCache creates a cache with the given byte limit and TTL.
func newFetchCache(maxBytes int64, ttl time.Duration) *fetchCache {
	return &fetchCache{
		items:    make(map[string]*list.Element),
		order:    list.New(),
		maxBytes: maxBytes,
		ttl:      ttl,
	}
}

// get retrieves a cached result. Returns ("", false) on miss or TTL expiry.
func (c *fetchCache) get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.items[key]
	if !ok {
		return "", false
	}
	entry := elem.Value.(*cacheEntry)
	if time.Now().After(entry.expiresAt) {
		c.removeLocked(elem)
		return "", false
	}
	c.order.MoveToFront(elem)
	return entry.content, true
}

// set stores a result, replacing any existing entry for the same key.
// May evict LRU entries to stay within the byte limit.
func (c *fetchCache) set(key, content string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	size := int64(len(content))
	if size < 1 {
		size = 1
	}

	if elem, ok := c.items[key]; ok {
		entry := elem.Value.(*cacheEntry)
		c.curBytes -= entry.size
		entry.content = content
		entry.size = size
		entry.expiresAt = time.Now().Add(c.ttl)
		c.curBytes += size
		c.order.MoveToFront(elem)
		c.evictLocked()
		return
	}

	entry := &cacheEntry{
		key:       key,
		content:   content,
		expiresAt: time.Now().Add(c.ttl),
		size:      size,
	}
	elem := c.order.PushFront(entry)
	c.items[key] = elem
	c.curBytes += size
	c.evictLocked()
}

// evictLocked removes LRU entries until total byte size is within limit.
// Must be called with c.mu held.
func (c *fetchCache) evictLocked() {
	for c.curBytes > c.maxBytes {
		back := c.order.Back()
		if back == nil {
			break
		}
		c.removeLocked(back)
	}
}

// removeLocked removes a single element from the cache.
// Must be called with c.mu held.
func (c *fetchCache) removeLocked(elem *list.Element) {
	entry := elem.Value.(*cacheEntry)
	c.order.Remove(elem)
	delete(c.items, entry.key)
	c.curBytes -= entry.size
}

// len returns the number of live (non-expired) entries.
func (c *fetchCache) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

// byteSize returns the current total cached byte size.
func (c *fetchCache) byteSize() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.curBytes
}

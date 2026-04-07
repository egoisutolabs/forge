package tools

import (
	"container/list"
	"path/filepath"
	"sync"
)

// FileState represents the cached state of a file after it has been read.
// This is the Go equivalent of Claude Code's FileState type in fileStateCache.ts.
//
// Key behaviors:
//   - Offset/Limit nil means full file read
//   - IsPartialView true means auto-injected content (e.g. CLAUDE.md stripped)
//     where the model saw a modified version — Edit/Write must require explicit Read
//   - Content always holds RAW disk bytes, even for partial views
type FileState struct {
	Content       string // raw file bytes from disk
	Timestamp     int64  // file mtime in unix milliseconds
	Offset        *int   // line offset if partial read, nil if full
	Limit         *int   // line limit if partial read, nil if full
	IsPartialView *bool  // true only when auto-injected content differs from disk
}

// fileStateEntry is an internal cache entry wrapping FileState with its size.
type fileStateEntry struct {
	key   string
	state FileState
	size  int64 // byte size of content (min 1)
}

// FileStateCache is an LRU cache that tracks file read state.
// Evicts when either entry count or total byte size exceeds limits.
//
// This is the Go equivalent of Claude Code's FileStateCache which wraps
// Node's lru-cache with both max entries and maxSize constraints.
//
// Thread-safe — all methods are guarded by a mutex.
type FileStateCache struct {
	mu       sync.Mutex
	items    map[string]*list.Element // normalized path → list element
	order    *list.List               // front = most recent, back = LRU
	maxCount int
	maxBytes int64
	curBytes int64
}

// NewFileStateCache creates a new cache with the given limits.
// Claude Code defaults: maxCount=100, maxBytes=25*1024*1024 (25MB).
func NewFileStateCache(maxCount int, maxBytes int64) *FileStateCache {
	return &FileStateCache{
		items:    make(map[string]*list.Element),
		order:    list.New(),
		maxCount: maxCount,
		maxBytes: maxBytes,
	}
}

// Get retrieves a file state from the cache. Returns (state, true) on hit,
// (zero, false) on miss. Accessing an entry promotes it to most-recently-used.
func (c *FileStateCache) Get(path string) (FileState, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := normalizePath(path)
	elem, ok := c.items[key]
	if !ok {
		return FileState{}, false
	}
	c.order.MoveToFront(elem)
	return elem.Value.(*fileStateEntry).state, true
}

// Set adds or updates a file state in the cache.
// May evict LRU entries to stay within count and size limits.
func (c *FileStateCache) Set(path string, state FileState) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := normalizePath(path)
	size := entrySize(state.Content)

	// Update existing
	if elem, ok := c.items[key]; ok {
		entry := elem.Value.(*fileStateEntry)
		c.curBytes -= entry.size
		entry.state = state
		entry.size = size
		c.curBytes += size
		c.order.MoveToFront(elem)
		c.evictLocked()
		return
	}

	// Insert new
	entry := &fileStateEntry{key: key, state: state, size: size}
	elem := c.order.PushFront(entry)
	c.items[key] = elem
	c.curBytes += size
	c.evictLocked()
}

// Has returns true if the path exists in the cache.
func (c *FileStateCache) Has(path string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.items[normalizePath(path)]
	return ok
}

// Delete removes a path from the cache.
func (c *FileStateCache) Delete(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := normalizePath(path)
	if elem, ok := c.items[key]; ok {
		c.removeLocked(elem)
	}
}

// Clear removes all entries from the cache.
func (c *FileStateCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]*list.Element)
	c.order.Init()
	c.curBytes = 0
}

// Len returns the number of entries in the cache.
func (c *FileStateCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

// ByteSize returns the current total byte size of cached content.
func (c *FileStateCache) ByteSize() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.curBytes
}

// evictLocked removes LRU entries until both count and size limits are met.
// Must be called with c.mu held.
func (c *FileStateCache) evictLocked() {
	for len(c.items) > c.maxCount || c.curBytes > c.maxBytes {
		back := c.order.Back()
		if back == nil {
			break
		}
		c.removeLocked(back)
	}
}

// removeLocked removes a single element from the cache.
// Must be called with c.mu held.
func (c *FileStateCache) removeLocked(elem *list.Element) {
	entry := elem.Value.(*fileStateEntry)
	c.order.Remove(elem)
	delete(c.items, entry.key)
	c.curBytes -= entry.size
}

// entrySize returns the byte size for a content string.
// Minimum 1 byte (matches Claude Code's Math.max(1, Buffer.byteLength(...))).
func entrySize(content string) int64 {
	n := int64(len(content))
	if n < 1 {
		return 1
	}
	return n
}

// normalizePath cleans a file path so equivalent paths hit the same cache entry.
// It resolves symlinks (if possible) so that a path and its symlink-resolved
// equivalent hit the same cache slot. This prevents cache misses caused by
// paths like /var/folders/... vs /private/var/folders/... on macOS.
func normalizePath(p string) string {
	// Try to resolve symlinks for consistent cache keys.
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	// Fallback to Clean if the path doesn't exist yet.
	return filepath.Clean(p)
}

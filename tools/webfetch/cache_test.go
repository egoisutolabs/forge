package webfetch

import (
	"testing"
	"time"
)

func TestCacheGetSet(t *testing.T) {
	c := newFetchCache(1024*1024, 5*time.Minute)

	// Miss on empty cache
	if _, ok := c.get("http://example.com"); ok {
		t.Fatal("expected miss on empty cache")
	}

	c.set("http://example.com", "hello world")

	got, ok := c.get("http://example.com")
	if !ok {
		t.Fatal("expected hit after set")
	}
	if got != "hello world" {
		t.Fatalf("got %q, want %q", got, "hello world")
	}
}

func TestCacheTTLExpiry(t *testing.T) {
	// Very short TTL for testing
	c := newFetchCache(1024*1024, 50*time.Millisecond)
	c.set("http://example.com", "content")

	// Immediate read should hit
	if _, ok := c.get("http://example.com"); !ok {
		t.Fatal("expected hit before TTL expiry")
	}

	time.Sleep(100 * time.Millisecond)

	// After TTL expiry, should miss
	if _, ok := c.get("http://example.com"); ok {
		t.Fatal("expected miss after TTL expiry")
	}
}

func TestCacheSizeEviction(t *testing.T) {
	// Allow only ~10 bytes
	c := newFetchCache(10, time.Minute)

	c.set("a", "12345") // 5 bytes
	c.set("b", "67890") // 5 bytes — fills cache

	if c.byteSize() > 10 {
		t.Fatalf("cache over limit: %d bytes", c.byteSize())
	}

	// Adding a third entry should evict the LRU
	c.set("c", "abcde") // should evict "a" (LRU)

	// "a" or "b" should be gone (LRU evicted)
	_, aOk := c.get("a")
	_, cOk := c.get("c")
	if aOk {
		t.Error("expected 'a' to be evicted as LRU")
	}
	if !cOk {
		t.Error("expected 'c' to be present after set")
	}
}

func TestCacheLRUPromotion(t *testing.T) {
	// Allow only 10 bytes (~2 entries of 5 bytes each)
	c := newFetchCache(10, time.Minute)

	c.set("a", "11111") // 5 bytes
	c.set("b", "22222") // 5 bytes

	// Access "a" to promote it to MRU
	c.get("a")

	// Add "c" — should evict "b" (now LRU), not "a"
	c.set("c", "33333")

	_, aOk := c.get("a")
	_, bOk := c.get("b")
	if !aOk {
		t.Error("expected 'a' to survive (was recently accessed)")
	}
	if bOk {
		t.Error("expected 'b' to be evicted (was LRU)")
	}
}

func TestCacheUpdate(t *testing.T) {
	c := newFetchCache(1024, time.Minute)
	c.set("key", "old")
	c.set("key", "new")

	got, ok := c.get("key")
	if !ok || got != "new" {
		t.Fatalf("got %q ok=%v, want %q true", got, ok, "new")
	}

	if c.len() != 1 {
		t.Fatalf("expected 1 entry, got %d", c.len())
	}
}

func TestCacheByteSize(t *testing.T) {
	c := newFetchCache(1024*1024, time.Minute)
	c.set("a", "hello") // 5 bytes
	c.set("b", "world") // 5 bytes

	if c.byteSize() != 10 {
		t.Fatalf("expected 10 bytes, got %d", c.byteSize())
	}

	c.set("a", "x") // replace with 1 byte
	if c.byteSize() != 6 {
		t.Fatalf("expected 6 bytes after update, got %d", c.byteSize())
	}
}

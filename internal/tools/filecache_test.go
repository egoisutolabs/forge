package tools

import (
	"strings"
	"testing"
)

func TestFileStateCache_GetSet(t *testing.T) {
	c := NewFileStateCache(100, 25*1024*1024)

	c.Set("/tmp/foo.go", FileState{
		Content:   "package main",
		Timestamp: 1000,
	})

	got, ok := c.Get("/tmp/foo.go")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.Content != "package main" {
		t.Errorf("content = %q, want %q", got.Content, "package main")
	}
	if got.Timestamp != 1000 {
		t.Errorf("timestamp = %d, want 1000", got.Timestamp)
	}
}

func TestFileStateCache_Miss(t *testing.T) {
	c := NewFileStateCache(100, 25*1024*1024)

	_, ok := c.Get("/tmp/nope.go")
	if ok {
		t.Fatal("expected cache miss")
	}
}

func TestFileStateCache_Has(t *testing.T) {
	c := NewFileStateCache(100, 25*1024*1024)

	if c.Has("/tmp/foo.go") {
		t.Fatal("expected Has=false before Set")
	}
	c.Set("/tmp/foo.go", FileState{Content: "x", Timestamp: 1})
	if !c.Has("/tmp/foo.go") {
		t.Fatal("expected Has=true after Set")
	}
}

func TestFileStateCache_Delete(t *testing.T) {
	c := NewFileStateCache(100, 25*1024*1024)

	c.Set("/tmp/foo.go", FileState{Content: "x", Timestamp: 1})
	c.Delete("/tmp/foo.go")

	if c.Has("/tmp/foo.go") {
		t.Fatal("expected deleted")
	}
}

func TestFileStateCache_OffsetLimit(t *testing.T) {
	c := NewFileStateCache(100, 25*1024*1024)

	offset := 10
	limit := 50
	c.Set("/tmp/partial.go", FileState{
		Content:   "partial content",
		Timestamp: 2000,
		Offset:    &offset,
		Limit:     &limit,
	})

	got, ok := c.Get("/tmp/partial.go")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.Offset == nil || *got.Offset != 10 {
		t.Errorf("offset = %v, want 10", got.Offset)
	}
	if got.Limit == nil || *got.Limit != 50 {
		t.Errorf("limit = %v, want 50", got.Limit)
	}
}

func TestFileStateCache_IsPartialView(t *testing.T) {
	c := NewFileStateCache(100, 25*1024*1024)

	partial := true
	c.Set("/tmp/claude.md", FileState{
		Content:       "raw disk content",
		Timestamp:     3000,
		IsPartialView: &partial,
	})

	got, _ := c.Get("/tmp/claude.md")
	if got.IsPartialView == nil || !*got.IsPartialView {
		t.Error("expected IsPartialView=true")
	}
}

func TestFileStateCache_EvictsByCount(t *testing.T) {
	c := NewFileStateCache(3, 25*1024*1024) // max 3 entries

	c.Set("/a", FileState{Content: "a", Timestamp: 1})
	c.Set("/b", FileState{Content: "b", Timestamp: 2})
	c.Set("/c", FileState{Content: "c", Timestamp: 3})
	c.Set("/d", FileState{Content: "d", Timestamp: 4}) // should evict /a (LRU)

	if c.Has("/a") {
		t.Error("expected /a evicted")
	}
	if !c.Has("/b") || !c.Has("/c") || !c.Has("/d") {
		t.Error("expected /b, /c, /d to survive")
	}
	if c.Len() != 3 {
		t.Errorf("len = %d, want 3", c.Len())
	}
}

func TestFileStateCache_EvictsBySize(t *testing.T) {
	// Max 50 bytes. Each entry's size = len(content) bytes.
	c := NewFileStateCache(100, 50)

	c.Set("/a", FileState{Content: strings.Repeat("x", 20), Timestamp: 1}) // 20 bytes
	c.Set("/b", FileState{Content: strings.Repeat("y", 20), Timestamp: 2}) // 20 bytes → total 40
	c.Set("/c", FileState{Content: strings.Repeat("z", 20), Timestamp: 3}) // 20 bytes → total 60, evict /a

	if c.Has("/a") {
		t.Error("expected /a evicted by size")
	}
	if !c.Has("/b") || !c.Has("/c") {
		t.Error("expected /b, /c to survive")
	}
}

func TestFileStateCache_LRUOrder(t *testing.T) {
	c := NewFileStateCache(3, 25*1024*1024)

	c.Set("/a", FileState{Content: "a", Timestamp: 1})
	c.Set("/b", FileState{Content: "b", Timestamp: 2})
	c.Set("/c", FileState{Content: "c", Timestamp: 3})

	// Access /a to make it recently used
	c.Get("/a")

	// Add /d — should evict /b (least recently used), not /a
	c.Set("/d", FileState{Content: "d", Timestamp: 4})

	if !c.Has("/a") {
		t.Error("expected /a to survive (was accessed recently)")
	}
	if c.Has("/b") {
		t.Error("expected /b evicted (LRU)")
	}
}

func TestFileStateCache_PathNormalization(t *testing.T) {
	c := NewFileStateCache(100, 25*1024*1024)

	c.Set("/tmp/foo/../bar/baz.go", FileState{Content: "x", Timestamp: 1})

	// Should hit with normalized path
	if !c.Has("/tmp/bar/baz.go") {
		t.Error("expected cache hit with normalized path")
	}
	got, ok := c.Get("/tmp/bar/baz.go")
	if !ok || got.Content != "x" {
		t.Error("expected content match with normalized path")
	}
}

func TestFileStateCache_EmptyContentMinSize(t *testing.T) {
	// Empty content should count as 1 byte (min size), not 0
	c := NewFileStateCache(100, 3) // 3 byte max

	c.Set("/a", FileState{Content: "", Timestamp: 1}) // 1 byte (min)
	c.Set("/b", FileState{Content: "", Timestamp: 2}) // 1 byte
	c.Set("/c", FileState{Content: "", Timestamp: 3}) // 1 byte → total 3

	if c.Len() != 3 {
		t.Errorf("len = %d, want 3", c.Len())
	}

	c.Set("/d", FileState{Content: "", Timestamp: 4}) // total 4 → evict 1
	if c.Len() != 3 {
		t.Errorf("after eviction: len = %d, want 3", c.Len())
	}
}

func TestFileStateCache_Clear(t *testing.T) {
	c := NewFileStateCache(100, 25*1024*1024)

	c.Set("/a", FileState{Content: "a", Timestamp: 1})
	c.Set("/b", FileState{Content: "b", Timestamp: 2})
	c.Clear()

	if c.Len() != 0 {
		t.Errorf("len = %d after Clear, want 0", c.Len())
	}
	if c.Has("/a") || c.Has("/b") {
		t.Error("expected empty after Clear")
	}
}

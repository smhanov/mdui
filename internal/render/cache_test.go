package render

import (
	"fmt"
	"testing"
)

func testKey(i int) Key {
	return Key{
		Path:    fmt.Sprintf("/notes/%d.md", i),
		ModTime: 100,
		Width:   80,
		Style:   Dark,
	}
}

func TestCacheEvictsOldest(t *testing.T) {
	cache := NewCache(3)
	for i := 0; i < 10; i++ {
		cache.Put(testKey(i), fmt.Sprintf("document %d", i))
	}

	if got := cache.Len(); got != 3 {
		t.Fatalf("cache length = %d, want 3", got)
	}
	for i := 7; i < 10; i++ {
		if _, ok := cache.Get(testKey(i)); !ok {
			t.Errorf("recent entry %d was evicted", i)
		}
	}
	for i := 0; i < 7; i++ {
		if _, ok := cache.Get(testKey(i)); ok {
			t.Errorf("old entry %d should have been evicted", i)
		}
	}
}

func TestCacheGetRefreshesRecency(t *testing.T) {
	cache := NewCache(3)
	for i := 0; i < 3; i++ {
		cache.Put(testKey(i), "doc")
	}

	if _, ok := cache.Get(testKey(0)); !ok {
		t.Fatal("entry 0 missing before eviction")
	}
	cache.Put(testKey(3), "doc")

	if _, ok := cache.Get(testKey(0)); !ok {
		t.Error("entry 0 should survive because it was just read")
	}
	if _, ok := cache.Get(testKey(1)); ok {
		t.Error("entry 1 should be the least recently used and evicted")
	}
}

func TestCacheKeyDimensions(t *testing.T) {
	cache := NewCache(4)
	base := Key{Path: "/notes/a.md", ModTime: 100, Width: 80, Style: Dark}
	cache.Put(base, "rendered")

	tests := []struct {
		name string
		key  Key
	}{
		{"same key", base},
		{"mtime changed", Key{Path: base.Path, ModTime: 101, Width: 80, Style: Dark}},
		{"width changed", Key{Path: base.Path, ModTime: 100, Width: 81, Style: Dark}},
		{"style changed", Key{Path: base.Path, ModTime: 100, Width: 80, Style: Light}},
		{"path changed", Key{Path: "/notes/b.md", ModTime: 100, Width: 80, Style: Dark}},
	}

	for _, tt := range tests {
		_, ok := cache.Get(tt.key)
		if want := tt.key == base; ok != want {
			t.Errorf("%s: cache hit = %v, want %v", tt.name, ok, want)
		}
	}
}

func TestCachePutReplacesValue(t *testing.T) {
	cache := NewCache(2)
	key := testKey(0)
	cache.Put(key, "first")
	cache.Put(key, "second")

	if got := cache.Len(); got != 1 {
		t.Fatalf("cache length = %d, want 1", got)
	}
	if value, ok := cache.Get(key); !ok || value != "second" {
		t.Fatalf("cache value = %q (hit %v), want second", value, ok)
	}
}

package render

import (
	"container/list"
	"sync"
)

// Key identifies one rendered variant of a document. Two requests with the
// same key can safely reuse the rendered string.
type Key struct {
	Path    string
	ModTime int64
	Width   int
	Style   Style
}

// Cache is a small, concurrency-safe LRU of rendered markdown. Render jobs run
// in goroutines, so every operation takes the lock.
type Cache struct {
	mu       sync.Mutex
	capacity int
	order    *list.List
	items    map[Key]*list.Element
}

type cacheEntry struct {
	key   Key
	value string
}

// NewCache returns an LRU cache holding at most capacity entries.
func NewCache(capacity int) *Cache {
	if capacity < 1 {
		capacity = 1
	}
	return &Cache{
		capacity: capacity,
		order:    list.New(),
		items:    make(map[Key]*list.Element, capacity),
	}
}

// Get returns the cached render for key and marks it as recently used.
func (c *Cache) Get(key Key) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	element, ok := c.items[key]
	if !ok {
		return "", false
	}
	c.order.MoveToFront(element)
	return element.Value.(*cacheEntry).value, true
}

// Put stores value for key, evicting the least recently used entry when the
// cache is full.
func (c *Cache) Put(key Key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if element, ok := c.items[key]; ok {
		element.Value.(*cacheEntry).value = value
		c.order.MoveToFront(element)
		return
	}

	element := c.order.PushFront(&cacheEntry{key: key, value: value})
	c.items[key] = element
	for c.order.Len() > c.capacity {
		oldest := c.order.Back()
		if oldest == nil {
			break
		}
		c.order.Remove(oldest)
		delete(c.items, oldest.Value.(*cacheEntry).key)
	}
}

// Len reports how many entries the cache currently holds.
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

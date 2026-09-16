package cache

import (
	"strings"
	"sync"
	"time"
)

type entry struct {
	value     string
	expiresAt time.Time
}

// Cache is an in-memory, TTL-expiring key-value store. It never persists
// to disk; a process restart cold-starts the cache by design (spec:
// Non-goals — persisted cache is future work).
type Cache struct {
	mu      sync.Mutex
	entries map[string]entry
	now     func() time.Time
}

// New creates a Cache. now is injected so tests can control expiry without
// real sleeps; production callers pass time.Now.
func New(now func() time.Time) *Cache {
	return &Cache{
		entries: make(map[string]entry),
		now:     now,
	}
}

func (c *Cache) Get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	e, ok := c.entries[key]
	if !ok {
		return "", false
	}
	if c.now().After(e.expiresAt) {
		delete(c.entries, key)
		return "", false
	}
	return e.value, true
}

func (c *Cache) Set(key, value string, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = entry{value: value, expiresAt: c.now().Add(ttl)}
}

// Evict removes every entry whose key has the given prefix. Used for
// `timeshare lock`, which evicts all entries for one project.
func (c *Cache) Evict(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key := range c.entries {
		if strings.HasPrefix(key, prefix) {
			delete(c.entries, key)
		}
	}
}

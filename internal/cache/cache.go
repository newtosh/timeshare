package cache

import (
	"sort"
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

// Meta is a cache entry's identity and expiry. It deliberately has no
// value: status listings must not copy secret material out of the cache.
type Meta struct {
	Key       string
	ExpiresAt time.Time
}

// List returns unexpired entries whose key has prefix, in key order.
// Expired entries are dropped as they are seen.
func (c *Cache) List(prefix string) []Meta {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	out := make([]Meta, 0)
	for key, e := range c.entries {
		if now.After(e.expiresAt) {
			delete(c.entries, key)
			continue
		}
		if strings.HasPrefix(key, prefix) {
			out = append(out, Meta{Key: key, ExpiresAt: e.expiresAt})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
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

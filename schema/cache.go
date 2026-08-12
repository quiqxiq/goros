package schema

import (
	"sync"
	"time"
)

// DefaultSchemaTTL is how long a discovered CommandSchema stays fresh before
// the next Discover re-probes the device. Kept short so newly added fields
// are noticed without manual invalidation.
const DefaultSchemaTTL = 30 * time.Second

// SchemaKey builds the cache key for a command: path + NUL + verb (NUL
// cannot appear in a RouterOS path/verb, so the pair is unambiguous).
func SchemaKey(path, verb string) string {
	return path + "\x00" + verb
}

type cacheEntry struct {
	schema  *CommandSchema
	expires time.Time
}

// Cache is a TTL cache of CommandSchema keyed by (path, verb). Values are
// immutable once stored — callers must not mutate them.
type Cache struct {
	mu      sync.Mutex
	ttl     time.Duration
	entries map[string]cacheEntry
}

// NewCache returns a cache with the DefaultSchemaTTL.
func NewCache() *Cache { return NewCacheWithTTL(DefaultSchemaTTL) }

// NewCacheWithTTL returns a cache with a custom TTL; a zero TTL disables
// expiry (entries live until invalidated).
func NewCacheWithTTL(ttl time.Duration) *Cache {
	return &Cache{ttl: ttl, entries: make(map[string]cacheEntry)}
}

// Get returns the cached schema for key, or nil when absent or expired.
func (c *Cache) Get(key string) *CommandSchema {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return nil
	}
	if c.ttl > 0 && time.Now().After(e.expires) {
		delete(c.entries, key)
		return nil
	}
	return e.schema
}

// Put stores schema under key.
func (c *Cache) Put(key string, s *CommandSchema) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = cacheEntry{schema: s, expires: time.Now().Add(c.ttl)}
}

// Delete removes one (path, verb) entry; Clear removes all entries.
func (c *Cache) Delete(path, verb string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, SchemaKey(path, verb))
}

// Clear removes all cached schemas.
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]cacheEntry)
}

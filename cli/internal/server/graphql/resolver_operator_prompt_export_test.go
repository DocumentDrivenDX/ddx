package graphql

import "time"

// SetMemoryIdempotencyCacheClockForTest replaces the cache's clock with a
// caller-supplied function so tests can advance time deterministically.
func SetMemoryIdempotencyCacheClockForTest(c *MemoryIdempotencyCache, now func() time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = now
}

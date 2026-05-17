package cache

import (
	"sync"
	"time"

	"github.com/sanketn26/gossipcache/internal/storage/memory"
)

// fakeClock is a shared clock for cache and storage tests. It implements both
// cache.Clock and memory.Clock so a single instance can advance time for both
// the manager (which calls Now() during SetMulti) and the storage engine
// (which calls Now() during Set/Get/expiry sweeps).
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(start time.Time) *fakeClock {
	return &fakeClock{now: start}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

var _ Clock = (*fakeClock)(nil)
var _ memory.Clock = (*fakeClock)(nil)

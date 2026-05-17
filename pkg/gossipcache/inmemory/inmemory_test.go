package inmemory

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/sanketn26/gossipcache/pkg/gossipcache"
)

func TestNewReturnsWorkingCache(t *testing.T) {
	c, err := New(Options{MaxSize: 1 << 20, DefaultTTL: time.Minute})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	ctx := context.Background()
	if err := c.Set(ctx, "k", []byte("v"), 0); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := c.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !bytes.Equal(got, []byte("v")) {
		t.Fatalf("Get = %q, want %q", got, "v")
	}
}

func TestNewSurfacesUnsupportedEvictionPolicy(t *testing.T) {
	_, err := New(Options{MaxSize: 1 << 20, EvictionPolicy: "fifo"})
	if err == nil {
		t.Fatal("New returned nil error, want unsupported eviction policy")
	}
}

func TestNewReturnsPublicSentinels(t *testing.T) {
	c, err := New(Options{MaxSize: 1 << 20, MaxKeySize: 4, MaxValueSize: 4})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	ctx := context.Background()
	if _, err := c.Get(ctx, "missing"); !errors.Is(err, gossipcache.ErrKeyNotFound) {
		t.Fatalf("Get missing error = %v, want gossipcache.ErrKeyNotFound", err)
	}
	if err := c.Set(ctx, "too-long-key", []byte("v"), time.Minute); !errors.Is(err, gossipcache.ErrKeyTooLarge) {
		t.Fatalf("Set oversized key error = %v, want gossipcache.ErrKeyTooLarge", err)
	}
	if err := c.Set(ctx, "k", []byte("too long"), time.Minute); !errors.Is(err, gossipcache.ErrValueTooLarge) {
		t.Fatalf("Set oversized value error = %v, want gossipcache.ErrValueTooLarge", err)
	}
}

type sharedFakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *sharedFakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *sharedFakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func TestNewHonorsInjectedClock(t *testing.T) {
	clock := &sharedFakeClock{now: time.Unix(1_700_000_000, 0)}
	c, err := New(Options{MaxSize: 1 << 20, DefaultTTL: time.Minute, Clock: clock})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	ctx := context.Background()
	if err := c.Set(ctx, "k", []byte("v"), 20*time.Millisecond); err != nil {
		t.Fatalf("Set: %v", err)
	}
	clock.Advance(time.Second)
	if _, err := c.Get(ctx, "k"); !errors.Is(err, gossipcache.ErrKeyNotFound) {
		t.Fatalf("Get expired error = %v, want gossipcache.ErrKeyNotFound", err)
	}
}

type fakeRecorder struct {
	mu  sync.Mutex
	ops []string
}

func (r *fakeRecorder) RecordCacheOperation(op string, _ error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ops = append(r.ops, op)
}

func (r *fakeRecorder) SetCacheStats(_, _ int64) {}

func TestNewHonorsInjectedMetrics(t *testing.T) {
	recorder := &fakeRecorder{}
	c, err := New(Options{MaxSize: 1 << 20, DefaultTTL: time.Minute, Metrics: recorder})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	ctx := context.Background()
	if err := c.Set(ctx, "k", []byte("v"), 0); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if _, err := c.Get(ctx, "k"); err != nil {
		t.Fatalf("Get: %v", err)
	}

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.ops) != 2 || recorder.ops[0] != "set" || recorder.ops[1] != "get" {
		t.Fatalf("ops = %v, want [set get]", recorder.ops)
	}
}

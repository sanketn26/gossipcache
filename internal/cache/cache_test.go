package cache

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/sanketn26/gossipcache/internal/storage/memory"
	"github.com/sanketn26/gossipcache/pkg/gossipcache"
)

func TestManagerOperations(t *testing.T) {
	store, err := memory.New(memory.Options{MaxSize: 1 << 20, EvictionPolicy: "lru"})
	if err != nil {
		t.Fatalf("memory.New: %v", err)
	}

	manager := NewManager(store, &CacheConfig{DefaultTTL: time.Minute})
	ctx := context.Background()

	if err := manager.Set(ctx, "a", []byte("one"), 0); err != nil {
		t.Fatalf("Set: %v", err)
	}

	value, err := manager.Get(ctx, "a")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !bytes.Equal(value, []byte("one")) {
		t.Fatalf("Get value = %q, want one", value)
	}

	if err := manager.SetMulti(ctx, map[string][]byte{
		"b": []byte("two"),
		"c": []byte("three"),
	}, 0); err != nil {
		t.Fatalf("SetMulti: %v", err)
	}

	values, err := manager.GetMulti(ctx, []string{"a", "b", "missing", "c"})
	if err != nil {
		t.Fatalf("GetMulti: %v", err)
	}
	if len(values) != 3 {
		t.Fatalf("GetMulti returned %d values, want 3", len(values))
	}
	if string(values["b"]) != "two" || string(values["c"]) != "three" {
		t.Fatalf("GetMulti values = %#v", values)
	}

	stats, err := manager.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.Keys != 3 {
		t.Fatalf("Stats.Keys = %d, want 3", stats.Keys)
	}

	if err := manager.Delete(ctx, "a"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := manager.Get(ctx, "a"); !errors.Is(err, gossipcache.ErrKeyNotFound) {
		t.Fatalf("Get deleted error = %v, want gossipcache.ErrKeyNotFound", err)
	}

	if err := manager.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	stats, err = manager.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats after Flush: %v", err)
	}
	if stats.Keys != 0 {
		t.Fatalf("Stats.Keys after Flush = %d, want 0", stats.Keys)
	}

	if err := manager.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestManagerSetUsesExplicitTTL(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))
	store, err := memory.New(memory.Options{MaxSize: 1 << 20, EvictionPolicy: "lru", Clock: clock})
	if err != nil {
		t.Fatalf("memory.New: %v", err)
	}
	defer store.Close()

	manager := NewManager(store, &CacheConfig{DefaultTTL: time.Hour, Clock: clock})
	ctx := context.Background()

	if err := manager.Set(ctx, "short", []byte("value"), 20*time.Millisecond); err != nil {
		t.Fatalf("Set: %v", err)
	}
	clock.Advance(30 * time.Millisecond)

	if _, err := manager.Get(ctx, "short"); !errors.Is(err, gossipcache.ErrKeyNotFound) {
		t.Fatalf("Get expired error = %v, want gossipcache.ErrKeyNotFound", err)
	}
}

type recordedOp struct {
	op  string
	err error
}

type fakeRecorder struct {
	mu    sync.Mutex
	ops   []recordedOp
	stats []struct {
		size int64
		keys int64
	}
}

func (r *fakeRecorder) RecordCacheOperation(op string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ops = append(r.ops, recordedOp{op: op, err: err})
}

func (r *fakeRecorder) SetCacheStats(sizeBytes, keys int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stats = append(r.stats, struct {
		size int64
		keys int64
	}{size: sizeBytes, keys: keys})
}

func (r *fakeRecorder) snapshot() []recordedOp {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]recordedOp, len(r.ops))
	copy(out, r.ops)
	return out
}

func TestManagerRecordsMetrics(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))
	store, err := memory.New(memory.Options{MaxSize: 1 << 20, EvictionPolicy: "lru", Clock: clock})
	if err != nil {
		t.Fatalf("memory.New: %v", err)
	}
	defer store.Close()

	recorder := &fakeRecorder{}
	manager := NewManager(store, &CacheConfig{DefaultTTL: time.Minute, Clock: clock, Metrics: recorder})
	ctx := context.Background()

	if err := manager.Set(ctx, "k", []byte("v"), 0); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if _, err := manager.Get(ctx, "k"); err != nil {
		t.Fatalf("Get hit: %v", err)
	}
	if _, err := manager.Get(ctx, "missing"); !errors.Is(err, gossipcache.ErrKeyNotFound) {
		t.Fatalf("Get miss error = %v, want gossipcache.ErrKeyNotFound", err)
	}
	if err := manager.Delete(ctx, "k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := manager.Stats(ctx); err != nil {
		t.Fatalf("Stats: %v", err)
	}

	ops := recorder.snapshot()
	if len(ops) != 5 {
		t.Fatalf("recorded %d ops, want 5: %#v", len(ops), ops)
	}
	want := []recordedOp{
		{op: "set", err: nil},
		{op: "get", err: nil},
		{op: "get", err: gossipcache.ErrKeyNotFound},
		{op: "delete", err: nil},
		{op: "stats", err: nil},
	}
	for i, w := range want {
		if ops[i].op != w.op {
			t.Fatalf("ops[%d].op = %q, want %q", i, ops[i].op, w.op)
		}
		if w.err == nil {
			if ops[i].err != nil {
				t.Fatalf("ops[%d].err = %v, want nil", i, ops[i].err)
			}
		} else if !errors.Is(ops[i].err, w.err) {
			t.Fatalf("ops[%d].err = %v, want errors.Is(%v)", i, ops[i].err, w.err)
		}
	}

	recorder.mu.Lock()
	statsCalls := len(recorder.stats)
	recorder.mu.Unlock()
	if statsCalls != 1 {
		t.Fatalf("SetCacheStats called %d times, want 1", statsCalls)
	}
}

func TestManagerSetMultiUsesInjectedClock(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))
	store, err := memory.New(memory.Options{MaxSize: 1 << 20, EvictionPolicy: "lru", Clock: clock})
	if err != nil {
		t.Fatalf("memory.New: %v", err)
	}
	defer store.Close()

	manager := NewManager(store, &CacheConfig{DefaultTTL: time.Minute, Clock: clock})
	ctx := context.Background()

	if err := manager.SetMulti(ctx, map[string][]byte{"a": []byte("1"), "b": []byte("2")}, 30*time.Millisecond); err != nil {
		t.Fatalf("SetMulti: %v", err)
	}

	if _, err := manager.Get(ctx, "a"); err != nil {
		t.Fatalf("Get a before expiry: %v", err)
	}

	clock.Advance(time.Second)
	if _, err := manager.Get(ctx, "a"); !errors.Is(err, gossipcache.ErrKeyNotFound) {
		t.Fatalf("Get a after expiry error = %v, want gossipcache.ErrKeyNotFound", err)
	}
	if _, err := manager.Get(ctx, "b"); !errors.Is(err, gossipcache.ErrKeyNotFound) {
		t.Fatalf("Get b after expiry error = %v, want gossipcache.ErrKeyNotFound", err)
	}
}

func TestManagerTranslatesClosedToPublicSentinel(t *testing.T) {
	store, err := memory.New(memory.Options{MaxSize: 1 << 20, EvictionPolicy: "lru"})
	if err != nil {
		t.Fatalf("memory.New: %v", err)
	}
	manager := NewManager(store, &CacheConfig{DefaultTTL: time.Minute})
	ctx := context.Background()

	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := manager.Get(ctx, "k"); !errors.Is(err, gossipcache.ErrClosed) {
		t.Fatalf("Get after close error = %v, want gossipcache.ErrClosed", err)
	}
	if err := manager.Set(ctx, "k", []byte("v"), 0); !errors.Is(err, gossipcache.ErrClosed) {
		t.Fatalf("Set after close error = %v, want gossipcache.ErrClosed", err)
	}
	if err := manager.Delete(ctx, "k"); !errors.Is(err, gossipcache.ErrClosed) {
		t.Fatalf("Delete after close error = %v, want gossipcache.ErrClosed", err)
	}
}

func TestManagerTranslatesKeyAndValueTooLarge(t *testing.T) {
	store, err := memory.New(memory.Options{
		MaxSize:        1 << 20,
		EvictionPolicy: "lru",
		MaxKeySize:     4,
		MaxValueSize:   4,
	})
	if err != nil {
		t.Fatalf("memory.New: %v", err)
	}
	defer store.Close()

	manager := NewManager(store, &CacheConfig{DefaultTTL: time.Minute})
	ctx := context.Background()

	err = manager.Set(ctx, "too-long", []byte("ok"), 0)
	if !errors.Is(err, gossipcache.ErrKeyTooLarge) {
		t.Fatalf("Set oversized key error = %v, want gossipcache.ErrKeyTooLarge", err)
	}

	err = manager.Set(ctx, "k", []byte("too long"), 0)
	if !errors.Is(err, gossipcache.ErrValueTooLarge) {
		t.Fatalf("Set oversized value error = %v, want gossipcache.ErrValueTooLarge", err)
	}
}

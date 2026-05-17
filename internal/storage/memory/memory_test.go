package memory

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sanketn26/gossipcache/internal/storage"
)

func newTestStorage(t *testing.T, opts Options) *MemoryStorage {
	t.Helper()
	if opts.MaxSize == 0 {
		opts.MaxSize = 1 << 20
	}
	if opts.EvictionPolicy == "" {
		opts.EvictionPolicy = "lru"
	}
	store, err := New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}

func TestMemoryStorageSetGetReturnsCopy(t *testing.T) {
	store := newTestStorage(t, Options{})

	ctx := context.Background()
	original := []byte("value1")
	if err := store.Set(ctx, "key1", original, time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}

	original[0] = 'V'

	entry, err := store.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if entry.Key != "key1" {
		t.Fatalf("Key = %q, want key1", entry.Key)
	}
	if !bytes.Equal(entry.Value, []byte("value1")) {
		t.Fatalf("Value = %q, want value1", entry.Value)
	}

	entry.Value[0] = 'X'
	entry, err = store.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Get after mutation: %v", err)
	}
	if !bytes.Equal(entry.Value, []byte("value1")) {
		t.Fatalf("stored value was mutated through returned entry: %q", entry.Value)
	}
}

func TestMemoryStorageMissingExpiredDeleteAndStats(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))
	store := newTestStorage(t, Options{Clock: clock})

	ctx := context.Background()
	if _, err := store.Get(ctx, "missing"); !errors.Is(err, storage.ErrKeyNotFound) {
		t.Fatalf("Get missing error = %v, want ErrKeyNotFound", err)
	}

	if err := store.Set(ctx, "short", []byte("value"), 20*time.Millisecond); err != nil {
		t.Fatalf("Set short: %v", err)
	}
	clock.Advance(30 * time.Millisecond)

	if _, err := store.Get(ctx, "short"); !errors.Is(err, storage.ErrKeyNotFound) {
		t.Fatalf("Get expired error = %v, want ErrKeyNotFound", err)
	}

	if err := store.Set(ctx, "delete-me", []byte("value"), time.Minute); err != nil {
		t.Fatalf("Set delete-me: %v", err)
	}
	if err := store.Delete(ctx, "delete-me"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get(ctx, "delete-me"); !errors.Is(err, storage.ErrKeyNotFound) {
		t.Fatalf("Get deleted error = %v, want ErrKeyNotFound", err)
	}

	stats, err := store.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.Hits != 0 {
		t.Fatalf("Hits = %d, want 0", stats.Hits)
	}
	if stats.Misses != 3 {
		t.Fatalf("Misses = %d, want 3", stats.Misses)
	}
}

func TestMemoryStorageGetMultiSetMultiKeysAndClose(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))
	store, err := New(Options{MaxSize: 1 << 20, EvictionPolicy: "lru", Clock: clock})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	entries := map[string]*storage.Entry{
		"a": {Key: "a", Value: []byte("one"), ExpiresAt: clock.Now().Add(time.Minute)},
		"b": {Key: "b", Value: []byte("two"), ExpiresAt: clock.Now().Add(time.Minute)},
	}
	if err := store.SetMulti(ctx, entries); err != nil {
		t.Fatalf("SetMulti: %v", err)
	}

	got, err := store.GetMulti(ctx, []string{"a", "missing", "b"})
	if err != nil {
		t.Fatalf("GetMulti: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("GetMulti returned %d entries, want 2", len(got))
	}
	if string(got["a"].Value) != "one" || string(got["b"].Value) != "two" {
		t.Fatalf("GetMulti values = %#v", got)
	}

	keys, err := store.Keys(ctx)
	if err != nil {
		t.Fatalf("Keys: %v", err)
	}
	if !sameStringSet(keys, []string{"a", "b"}) {
		t.Fatalf("Keys = %v, want [a b]", keys)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if err := store.Set(ctx, "closed", []byte("value"), 0); !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("Set after Close error = %v, want ErrClosed", err)
	}
}

func TestMemoryStorageLRUEviction(t *testing.T) {
	store := newTestStorage(t, Options{MaxSize: 40})

	ctx := context.Background()
	if err := store.Set(ctx, "a", []byte(strings.Repeat("a", 15)), time.Minute); err != nil {
		t.Fatalf("Set a: %v", err)
	}
	if err := store.Set(ctx, "b", []byte(strings.Repeat("b", 15)), time.Minute); err != nil {
		t.Fatalf("Set b: %v", err)
	}
	if _, err := store.Get(ctx, "a"); err != nil {
		t.Fatalf("Get a: %v", err)
	}
	if err := store.Set(ctx, "c", []byte(strings.Repeat("c", 15)), time.Minute); err != nil {
		t.Fatalf("Set c: %v", err)
	}

	if _, err := store.Get(ctx, "b"); !errors.Is(err, storage.ErrKeyNotFound) {
		t.Fatalf("Get b error = %v, want ErrKeyNotFound after LRU eviction", err)
	}
	if _, err := store.Get(ctx, "a"); err != nil {
		t.Fatalf("Get a after eviction: %v", err)
	}
	if _, err := store.Get(ctx, "c"); err != nil {
		t.Fatalf("Get c after eviction: %v", err)
	}

	stats, err := store.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.Evictions == 0 {
		t.Fatal("Evictions = 0, want at least one eviction")
	}
	if stats.Size > 40 {
		t.Fatalf("Size = %d, want <= 40", stats.Size)
	}
}

func TestMemoryStorageRejectsUnsupportedEvictionPolicy(t *testing.T) {
	_, err := New(Options{MaxSize: 1 << 20, EvictionPolicy: "fifo"})
	if err == nil {
		t.Fatal("New returned nil error, want unsupported eviction policy")
	}
	if !strings.Contains(err.Error(), "unsupported eviction policy") {
		t.Fatalf("error = %q, want unsupported eviction policy", err)
	}
}

func TestMemoryStorageConcurrentAccess(t *testing.T) {
	store := newTestStorage(t, Options{})

	ctx := context.Background()
	var wg sync.WaitGroup
	for worker := 0; worker < 16; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				key := fmt.Sprintf("worker-%d-key-%d", worker, i)
				if err := store.Set(ctx, key, []byte("value"), time.Minute); err != nil {
					t.Errorf("Set %q: %v", key, err)
					return
				}
				if _, err := store.Get(ctx, key); err != nil {
					t.Errorf("Get %q: %v", key, err)
					return
				}
			}
		}(worker)
	}
	wg.Wait()
}

func TestMemoryStorageRejectsOversizedKey(t *testing.T) {
	store := newTestStorage(t, Options{MaxKeySize: 4})

	ctx := context.Background()
	err := store.Set(ctx, "too-long-key", []byte("x"), time.Minute)
	if !errors.Is(err, storage.ErrKeyTooLarge) {
		t.Fatalf("Set oversized key error = %v, want ErrKeyTooLarge", err)
	}

	entries := map[string]*storage.Entry{
		"too-long-key": {Key: "too-long-key", Value: []byte("x"), ExpiresAt: time.Now().Add(time.Minute)},
	}
	if err := store.SetMulti(ctx, entries); !errors.Is(err, storage.ErrKeyTooLarge) {
		t.Fatalf("SetMulti oversized key error = %v, want ErrKeyTooLarge", err)
	}
}

func TestMemoryStorageRejectsOversizedValue(t *testing.T) {
	store := newTestStorage(t, Options{MaxValueSize: 4})

	ctx := context.Background()
	err := store.Set(ctx, "k", []byte("too long"), time.Minute)
	if !errors.Is(err, storage.ErrValueTooLarge) {
		t.Fatalf("Set oversized value error = %v, want ErrValueTooLarge", err)
	}
}

func TestMemoryStorageZeroLimitsDisableEnforcement(t *testing.T) {
	store := newTestStorage(t, Options{MaxKeySize: 0, MaxValueSize: 0})

	ctx := context.Background()
	longKey := strings.Repeat("k", 1024)
	longValue := bytes.Repeat([]byte("v"), 1<<16)
	if err := store.Set(ctx, longKey, longValue, time.Minute); err != nil {
		t.Fatalf("Set with zero limits returned error: %v", err)
	}
}

func TestMemoryStorageStatsSizeMatchesSetDeleteOverwriteEvict(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))
	store := newTestStorage(t, Options{MaxSize: 100, Clock: clock})

	ctx := context.Background()
	if err := store.Set(ctx, "k1", []byte("v1"), time.Minute); err != nil {
		t.Fatalf("Set k1: %v", err)
	}
	want := int64(len("k1") + len("v1"))
	if got := mustStats(t, store).Size; got != want {
		t.Fatalf("after Set, Size = %d, want %d", got, want)
	}

	if err := store.Set(ctx, "k1", []byte("longer"), time.Minute); err != nil {
		t.Fatalf("overwrite k1: %v", err)
	}
	want = int64(len("k1") + len("longer"))
	if got := mustStats(t, store).Size; got != want {
		t.Fatalf("after overwrite, Size = %d, want %d", got, want)
	}

	if err := store.Delete(ctx, "k1"); err != nil {
		t.Fatalf("Delete k1: %v", err)
	}
	if got := mustStats(t, store).Size; got != 0 {
		t.Fatalf("after Delete, Size = %d, want 0", got)
	}

	if err := store.Delete(ctx, "absent"); err != nil {
		t.Fatalf("Delete absent: %v", err)
	}
	if got := mustStats(t, store).Size; got != 0 {
		t.Fatalf("after Delete absent, Size = %d, want 0", got)
	}

	if err := store.Set(ctx, "expire", []byte("vv"), 10*time.Millisecond); err != nil {
		t.Fatalf("Set expire: %v", err)
	}
	clock.Advance(time.Second)
	if _, err := store.Get(ctx, "expire"); !errors.Is(err, storage.ErrKeyNotFound) {
		t.Fatalf("Get expired error = %v, want ErrKeyNotFound", err)
	}
	if got := mustStats(t, store).Size; got != 0 {
		t.Fatalf("after expired Get, Size = %d, want 0", got)
	}

	// Force LRU eviction: MaxSize=100, set entries totalling >100.
	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("ek%d", i)
		value := bytes.Repeat([]byte("x"), 40)
		if err := store.Set(ctx, key, value, time.Minute); err != nil {
			t.Fatalf("Set %q: %v", key, err)
		}
	}
	stats := mustStats(t, store)
	if stats.Evictions == 0 {
		t.Fatal("expected at least one eviction")
	}
	if stats.Size > 100 {
		t.Fatalf("Size = %d, want <= 100", stats.Size)
	}
}

func mustStats(t *testing.T, store *MemoryStorage) *storage.Stats {
	t.Helper()
	stats, err := store.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	return stats
}

func sameStringSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := make(map[string]bool, len(got))
	for _, value := range got {
		seen[value] = true
	}
	for _, value := range want {
		if !seen[value] {
			return false
		}
	}
	return true
}

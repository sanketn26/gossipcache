package cache

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sanketn26/gossipcache/internal/storage"
	"github.com/sanketn26/gossipcache/internal/storage/memory"
)

func TestManagerOperations(t *testing.T) {
	store, err := memory.New(1<<20, "lru")
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
	if _, err := manager.Get(ctx, "a"); !errors.Is(err, storage.ErrKeyNotFound) {
		t.Fatalf("Get deleted error = %v, want ErrKeyNotFound", err)
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
	store, err := memory.New(1<<20, "lru")
	if err != nil {
		t.Fatalf("memory.New: %v", err)
	}
	defer store.Close()

	manager := NewManager(store, &CacheConfig{DefaultTTL: time.Hour})
	ctx := context.Background()

	if err := manager.Set(ctx, "short", []byte("value"), 20*time.Millisecond); err != nil {
		t.Fatalf("Set: %v", err)
	}
	time.Sleep(30 * time.Millisecond)

	if _, err := manager.Get(ctx, "short"); !errors.Is(err, storage.ErrKeyNotFound) {
		t.Fatalf("Get expired error = %v, want ErrKeyNotFound", err)
	}
}

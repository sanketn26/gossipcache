package benchmark

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/sanketn26/gossipcache/internal/cache"
	"github.com/sanketn26/gossipcache/internal/storage/memory"
)

func BenchmarkCacheGet(b *testing.B) {
	manager := newBenchmarkCache(b)
	ctx := context.Background()
	if err := manager.Set(ctx, "key1", []byte("value1"), time.Minute); err != nil {
		b.Fatalf("Set: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := manager.Get(ctx, "key1"); err != nil {
			b.Fatalf("Get: %v", err)
		}
	}
}

func BenchmarkCacheSet(b *testing.B) {
	manager := newBenchmarkCache(b)
	ctx := context.Background()
	value := []byte("value1")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := manager.Set(ctx, fmt.Sprintf("key-%d", i), value, time.Minute); err != nil {
			b.Fatalf("Set: %v", err)
		}
	}
}

func BenchmarkCacheParallelGetSet(b *testing.B) {
	manager := newBenchmarkCache(b)
	ctx := context.Background()
	value := []byte("value1")

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key-%d", i%1024)
			if err := manager.Set(ctx, key, value, time.Minute); err != nil {
				b.Fatalf("Set: %v", err)
			}
			if _, err := manager.Get(ctx, key); err != nil {
				b.Fatalf("Get: %v", err)
			}
			i++
		}
	})
}

func newBenchmarkCache(b *testing.B) *cache.Manager {
	b.Helper()

	store, err := memory.New(1<<30, "lru")
	if err != nil {
		b.Fatalf("memory.New: %v", err)
	}
	b.Cleanup(func() {
		if err := store.Close(); err != nil {
			b.Fatalf("Close: %v", err)
		}
	})

	return cache.NewManager(store, &cache.CacheConfig{DefaultTTL: time.Minute})
}

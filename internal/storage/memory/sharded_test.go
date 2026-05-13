package memory

import (
	"fmt"
	"sync"
	"testing"

	"github.com/sanketn26/gossipcache/internal/storage"
)

func TestNewShardedMapDefaultsInvalidShardCount(t *testing.T) {
	sm := newShardedMap(0)
	if sm.count != defaultShards {
		t.Fatalf("count = %d, want %d", sm.count, defaultShards)
	}
}

func TestShardedMapConcurrentAccess(t *testing.T) {
	sm := newShardedMap(32)

	var wg sync.WaitGroup
	for worker := 0; worker < 16; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				key := fmt.Sprintf("%d-%d", worker, i)
				sm.set(key, &storage.Entry{Key: key, Value: []byte("value")})
				if _, ok := sm.get(key); !ok {
					t.Errorf("get(%q) returned !ok", key)
					return
				}
				if i%2 == 0 {
					sm.delete(key)
				}
			}
		}(worker)
	}
	wg.Wait()

	if sm.len() == 0 {
		t.Fatal("len = 0, want some remaining odd keys")
	}
}

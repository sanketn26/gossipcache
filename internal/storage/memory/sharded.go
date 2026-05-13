package memory

import (
	"hash/fnv"
	"sync"

	"github.com/sanketn26/gossipcache/internal/storage"
)

const defaultShards = 256

// shardedMap provides concurrent access to cache entries
// DRY: Encapsulates sharding logic
type shardedMap struct {
	shards []*shard
	count  int
}

type shard struct {
	mu      sync.RWMutex
	entries map[string]*storage.Entry
}

func newShardedMap(numShards int) *shardedMap {
	if numShards <= 0 {
		numShards = defaultShards
	}

	sm := &shardedMap{
		shards: make([]*shard, numShards),
		count:  numShards,
	}

	for i := 0; i < numShards; i++ {
		sm.shards[i] = &shard{
			entries: make(map[string]*storage.Entry),
		}
	}

	return sm
}

func (sm *shardedMap) getShard(key string) *shard {
	h := fnv.New32a()
	h.Write([]byte(key))
	return sm.shards[h.Sum32()%uint32(sm.count)]
}

func (sm *shardedMap) get(key string) (*storage.Entry, bool) {
	s := sm.getShard(key)
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.entries[key]
	return entry, ok
}

func (sm *shardedMap) set(key string, entry *storage.Entry) {
	s := sm.getShard(key)
	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries[key] = entry
}

func (sm *shardedMap) delete(key string) {
	s := sm.getShard(key)
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.entries, key)
}

func (sm *shardedMap) keys() []string {
	keys := make([]string, 0)

	for _, s := range sm.shards {
		s.mu.RLock()
		for k := range s.entries {
			keys = append(keys, k)
		}
		s.mu.RUnlock()
	}

	return keys
}

func (sm *shardedMap) len() int {
	total := 0
	for _, s := range sm.shards {
		s.mu.RLock()
		total += len(s.entries)
		s.mu.RUnlock()
	}
	return total
}

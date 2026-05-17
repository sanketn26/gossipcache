package memory

import (
	"hash/fnv"
	"sync"

	"github.com/sanketn26/gossipcache/internal/storage"
)

const defaultShards = 256

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

// set stores entry under key and returns the byte size of any prior entry that
// it replaced (zero if the key was new). Callers use the delta to maintain an
// O(1) running size counter without re-walking shards.
func (sm *shardedMap) set(key string, entry *storage.Entry) int64 {
	s := sm.getShard(key)
	s.mu.Lock()
	defer s.mu.Unlock()

	var prevSize int64
	if prev, ok := s.entries[key]; ok {
		prevSize = entrySize(prev)
	}
	s.entries[key] = entry
	return prevSize
}

// delete removes key and returns the byte size of the removed entry, or zero
// if the key was absent.
func (sm *shardedMap) delete(key string) int64 {
	s := sm.getShard(key)
	s.mu.Lock()
	defer s.mu.Unlock()

	prev, ok := s.entries[key]
	if !ok {
		return 0
	}
	delete(s.entries, key)
	return entrySize(prev)
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

func entrySize(e *storage.Entry) int64 {
	if e == nil {
		return 0
	}
	return int64(len(e.Key) + len(e.Value))
}

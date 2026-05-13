// internal/storage/memory/eviction.go
package memory

import (
	"container/list"
	"errors"
	"sync"
)

// EvictionPolicy defines the interface for eviction policies
// OCP: Open for extension (add new policies), closed for modification
type EvictionPolicy interface {
	OnAdd(key string)
	OnAccess(key string)
	OnRemove(key string)
	SelectVictim() string
}

func newEvictionPolicy(policy string) (EvictionPolicy, error) {
	switch policy {
	case "lru":
		return newLRUPolicy(), nil
	default:
		return nil, errors.New("unsupported eviction policy")
	}
}

// LRU eviction policy
type lruPolicy struct {
	mu       sync.Mutex
	list     *list.List
	elements map[string]*list.Element
}

func newLRUPolicy() *lruPolicy {
	return &lruPolicy{
		list:     list.New(),
		elements: make(map[string]*list.Element),
	}
}

func (lru *lruPolicy) OnAdd(key string) {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	if elem, ok := lru.elements[key]; ok {
		lru.list.MoveToFront(elem)
		return
	}

	elem := lru.list.PushFront(key)
	lru.elements[key] = elem
}

func (lru *lruPolicy) OnAccess(key string) {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	if elem, ok := lru.elements[key]; ok {
		lru.list.MoveToFront(elem)
	}
}

func (lru *lruPolicy) OnRemove(key string) {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	if elem, ok := lru.elements[key]; ok {
		lru.list.Remove(elem)
		delete(lru.elements, key)
	}
}

func (lru *lruPolicy) SelectVictim() string {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	elem := lru.list.Back()
	if elem == nil {
		return ""
	}

	return elem.Value.(string)
}

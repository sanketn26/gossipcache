package l1

import (
	"time"

	"github.com/sanketn26/gossipcache/internal/wire"
)

// Op identifies an operation performed by a deterministic fake Hub.
type Op uint8

const (
	OpGet Op = iota
	OpSet
	OpDelete
)

// Mutation is the immutable write snapshot observed by a fake's commit
// barrier. Version is intentionally absent because the barrier runs before the
// authoritative commit assigns one.
type Mutation struct {
	Op      Op
	Key     []byte
	Value   []byte
	TTL     time.Duration
	Options WriteOptions
}

// Clone returns an ownership-independent mutation.
func (m Mutation) Clone() Mutation {
	m.Key = wire.CopyBytes(m.Key)
	m.Value = wire.CopyBytes(m.Value)
	return m
}

// FakeHooks defines deterministic barriers and one-shot failure injection for
// HubClient contract tests. The fake invokes a returned release function inside
// the operation; tests can make that function wait on a channel to control the
// exact interleaving without sleeps.
type FakeHooks struct {
	BeforeGetReturn func(key []byte) (release func())
	BeforeCommit    func(m Mutation) (release func())
	FailNext        func(op Op) error
}

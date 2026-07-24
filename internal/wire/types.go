// Package wire defines contracts shared by GossipCache nodes and hubs.
package wire

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	// MaxKeyLen is the largest key accepted by the data plane.
	MaxKeyLen = 4 * 1024
	// MaxValueLen is the largest value accepted by the data plane.
	MaxValueLen = 1 << 20
	// TTLNone denotes a value with no expiry. Non-zero TTL values are whole
	// milliseconds.
	TTLNone uint64 = 0
	// MaxWriteTimeoutMillis is the largest write timeout the wire protocol can
	// carry: the RPC field is uint32 milliseconds.
	MaxWriteTimeoutMillis = math.MaxUint32
)

var (
	ErrKeyEmpty                   = errors.New("key must not be empty")
	ErrKeyTooLarge                = errors.New("key exceeds maximum length")
	ErrValueTooLarge              = errors.New("value exceeds maximum length")
	ErrMutationIDRequired         = errors.New("mutation ID must not be zero")
	ErrInvalidWriteMode           = errors.New("invalid write mode")
	ErrInvalidConfirmLevel        = errors.New("invalid confirmation level")
	ErrInvalidStorageProfile      = errors.New("invalid storage profile")
	ErrPersistenceConfigRequired  = errors.New("durable storage requires persistence configuration")
	ErrInvalidWriteTimeout        = errors.New("write timeout must not be negative")
	ErrTimeoutNotWholeMillis      = errors.New("write timeout must be a whole number of milliseconds")
	ErrWriteTimeoutTooLarge       = errors.New("write timeout exceeds maximum representable milliseconds")
	ErrTTLNotWholeMilliseconds    = errors.New("ttl must be a whole number of milliseconds")
	ErrInvalidProtocolRange       = errors.New("invalid protocol version range")
	ErrIncompatibleProtocolRanges = errors.New("protocol version ranges do not overlap")
)

// VersionTag identifies a mutation in a hub partition. Tags are ordered only
// within the same partition and hub generation.
type VersionTag struct {
	PartitionID uint32
	Sequence    uint64
}

// IsNewer reports whether v supersedes other. The caller is responsible for
// checking that both tags belong to the same hub generation.
func (v VersionTag) IsNewer(other VersionTag) bool {
	return v.PartitionID == other.PartitionID && v.Sequence > other.Sequence
}

// MutationID is a node-minted mutation identity. Its first eight bytes encode
// the node epoch and its last eight bytes encode a monotonic counter, both in
// network byte order. A node must mint it once and retain it across retries.
type MutationID [16]byte

// NewMutationID constructs a stable mutation identity.
func NewMutationID(nodeEpoch, counter uint64) MutationID {
	var id MutationID
	binary.BigEndian.PutUint64(id[:8], nodeEpoch)
	binary.BigEndian.PutUint64(id[8:], counter)
	return id
}

// NodeEpoch returns the node-epoch component of id.
func (id MutationID) NodeEpoch() uint64 {
	return binary.BigEndian.Uint64(id[:8])
}

// Counter returns the monotonic-counter component of id.
func (id MutationID) Counter() uint64 {
	return binary.BigEndian.Uint64(id[8:])
}

// IsZero reports whether id is the zero mutation identity.
func (id MutationID) IsZero() bool {
	return id == MutationID{}
}

// WriteMode controls when the hub acknowledges the durability portion of a
// write. It is independent of peer confirmation W.
type WriteMode uint8

const (
	// WriteFast acknowledges an atomic hub memory commit. It is the default.
	WriteFast WriteMode = iota
	// WriteSync acknowledges only after the durable synchronization fence.
	WriteSync
)

// Valid reports whether m is a supported write mode.
func (m WriteMode) Valid() bool {
	return m == WriteFast || m == WriteSync
}

// StorageProfile describes the hub's authority storage posture.
type StorageProfile uint8

const (
	// StorageMemory is ephemeral and is the default profile.
	StorageMemory StorageProfile = iota
	// StorageDurable enables opt-in persistence.
	StorageDurable
)

// Valid reports whether p is a supported storage profile.
func (p StorageProfile) Valid() bool {
	return p == StorageMemory || p == StorageDurable
}

// Validate checks the shared storage-profile contract. The hub remains
// responsible for checking that its configured persistence path is usable.
func (p StorageProfile) Validate(persistenceConfigured bool) error {
	if !p.Valid() {
		return fmt.Errorf("%w: %d", ErrInvalidStorageProfile, p)
	}
	if p == StorageDurable && !persistenceConfigured {
		return ErrPersistenceConfigRequired
	}
	return nil
}

// ConfirmLevel specifies what a peer confirmation means.
type ConfirmLevel uint8

const (
	// ConfirmInvalidateApplied is the only v1 confirmation level.
	ConfirmInvalidateApplied ConfirmLevel = iota
)

// Valid reports whether c is a supported confirmation level.
func (c ConfirmLevel) Valid() bool {
	return c == ConfirmInvalidateApplied
}

// WriteOptions is the shared write-policy model used by node options, hub
// seams, and RPC mapping.
type WriteOptions struct {
	W            uint16
	Mode         WriteMode
	ConfirmLevel ConfirmLevel
	Timeout      time.Duration
}

// Validate checks that the options use values supported by this protocol
// version. A zero timeout is allowed so a caller can apply its configured
// default.
func (o WriteOptions) Validate() error {
	if !o.Mode.Valid() {
		return fmt.Errorf("%w: %d", ErrInvalidWriteMode, o.Mode)
	}
	if !o.ConfirmLevel.Valid() {
		return fmt.Errorf("%w: %d", ErrInvalidConfirmLevel, o.ConfirmLevel)
	}
	if o.Timeout < 0 {
		return ErrInvalidWriteTimeout
	}
	if o.Timeout%time.Millisecond != 0 {
		return ErrTimeoutNotWholeMillis
	}
	if o.Timeout/time.Millisecond > MaxWriteTimeoutMillis {
		return ErrWriteTimeoutTooLarge
	}
	return nil
}

// GetRequest is the shared bounded model for an authoritative read.
type GetRequest struct {
	Key        []byte
	MinVersion *VersionTag
}

// Validate checks request bounds and partition consistency.
func (r GetRequest) Validate(partitionCount uint32) error {
	if err := validateKey(r.Key); err != nil {
		return err
	}
	if err := validatePartitionCount(partitionCount); err != nil {
		return err
	}
	if r.MinVersion != nil && r.MinVersion.PartitionID != PartitionOf(r.Key, partitionCount) {
		return fmt.Errorf("minimum version partition %d does not match routed partition", r.MinVersion.PartitionID)
	}
	return nil
}

// Clone returns an ownership-independent copy of r.
func (r GetRequest) Clone() GetRequest {
	cloned := GetRequest{Key: CopyBytes(r.Key)}
	if r.MinVersion != nil {
		version := *r.MinVersion
		cloned.MinVersion = &version
	}
	return cloned
}

// SetRequest is the shared bounded model for an authoritative set.
type SetRequest struct {
	Key        []byte
	Value      []byte
	TTLMillis  uint64
	MutationID MutationID
	Options    WriteOptions
}

// Validate checks request bounds and write options.
func (r SetRequest) Validate() error {
	if err := validateKey(r.Key); err != nil {
		return err
	}
	if len(r.Value) > MaxValueLen {
		return fmt.Errorf("%w: got %d, max %d", ErrValueTooLarge, len(r.Value), MaxValueLen)
	}
	if r.MutationID.IsZero() {
		return ErrMutationIDRequired
	}
	return r.Options.Validate()
}

// Clone returns an ownership-independent copy of r.
func (r SetRequest) Clone() SetRequest {
	r.Key = CopyBytes(r.Key)
	r.Value = CopyBytes(r.Value)
	return r
}

// DeleteRequest is the shared bounded model for an authoritative delete.
type DeleteRequest struct {
	Key        []byte
	MutationID MutationID
	Options    WriteOptions
}

// Validate checks request bounds and write options.
func (r DeleteRequest) Validate() error {
	if err := validateKey(r.Key); err != nil {
		return err
	}
	if r.MutationID.IsZero() {
		return ErrMutationIDRequired
	}
	return r.Options.Validate()
}

// Clone returns an ownership-independent copy of r.
func (r DeleteRequest) Clone() DeleteRequest {
	r.Key = CopyBytes(r.Key)
	return r
}

// CopyBytes copies mutable bytes at an ownership boundary. It preserves nil.
func CopyBytes(b []byte) []byte {
	return append([]byte(nil), b...)
}

// TTLMillis converts a public API duration to the wire representation.
func TTLMillis(ttl time.Duration) (uint64, error) {
	if ttl < 0 {
		return 0, fmt.Errorf("ttl must not be negative")
	}
	if ttl%time.Millisecond != 0 {
		return 0, ErrTTLNotWholeMilliseconds
	}
	return uint64(ttl / time.Millisecond), nil
}

func validateKey(key []byte) error {
	switch {
	case len(key) == 0:
		return ErrKeyEmpty
	case len(key) > MaxKeyLen:
		return fmt.Errorf("%w: got %d, max %d", ErrKeyTooLarge, len(key), MaxKeyLen)
	default:
		return nil
	}
}

// PersistenceConfigured reports whether a data-directory setting is
// non-empty. Runtime config must additionally verify that the path is usable.
func PersistenceConfigured(dataDir string) bool {
	return strings.TrimSpace(dataDir) != ""
}

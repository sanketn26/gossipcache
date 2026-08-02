package rpc

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sanketn26/gossipcache/internal/wire"
)

var (
	ErrInvalidHubGeneration        = errors.New("hub generation must not be zero")
	ErrInvalidDurableHealth        = errors.New("memory profile cannot advertise durable health")
	ErrInvalidOp                   = errors.New("invalid mutation operation")
	ErrUnexpectedTTL               = errors.New("message must not carry a ttl")
	ErrInvalidMutationStatus       = errors.New("invalid status for mutation response")
	ErrInvalidNodeID               = errors.New("node ID must not be zero")
	ErrMutationFingerprintMismatch = errors.New("mutation fingerprint does not match retained request")
	ErrInvalidStatus               = errors.New("invalid status")
	ErrInvalidEnum                 = errors.New("invalid protobuf enum value")
	ErrInvalidClusterID            = errors.New("cluster id must not be empty")

	// Shared get/record errors — aliases of wire so callers can use either package.
	ErrUnexpectedValue   = wire.ErrUnexpectedValue
	ErrUnexpectedVersion = wire.ErrUnexpectedVersion
	ErrMissingVersion    = wire.ErrMissingVersion
	ErrInvalidGetStatus  = wire.ErrInvalidGetStatus
	ErrInvalidRecordKind = wire.ErrInvalidRecordKind
	ErrInvalidResultTTL  = wire.ErrInvalidResultTTL
)

// Op is a mutation operation.
type Op uint8

const (
	// OpSet installs or replaces a live value.
	OpSet Op = 1
	// OpDelete installs a versioned tombstone.
	OpDelete Op = 2
)

// Valid reports whether o is a supported mutation operation.
func (o Op) Valid() bool {
	return o == OpSet || o == OpDelete
}

// HandshakeRequest is sent by a node to begin the data-plane session.
// ExpectedHubGeneration zero means bootstrap (no trusted generation yet).
type HandshakeRequest struct {
	Protocol              wire.ProtocolRange
	ClusterID             string
	ExpectedHubGeneration uint64
}

// Validate checks the handshake request payload.
func (m HandshakeRequest) Validate() error {
	if err := m.Protocol.Validate(); err != nil {
		return err
	}
	return validateClusterID(m.ClusterID)
}

// Handshake is the hub's session advertisement. StorageProfile is fixed for
// the hub lifetime; DurableHealthy gates WriteSync on a durable-profile hub.
type Handshake struct {
	ProtocolVersion wire.ProtocolVersion
	HubGeneration   uint64
	PartitionCount  uint32
	StorageProfile  wire.StorageProfile
	DurableHealthy  bool
	ClusterID       string
}

// Validate checks the hub handshake payload.
func (m Handshake) Validate() error {
	if m.ProtocolVersion < wire.MinSupportedProtocolVersion || m.ProtocolVersion > wire.CurrentProtocolVersion {
		return fmt.Errorf("%w: unsupported protocol version %d", wire.ErrIncompatibleProtocolRanges, m.ProtocolVersion)
	}
	if m.HubGeneration == 0 {
		return ErrInvalidHubGeneration
	}
	if m.PartitionCount == 0 {
		return wire.ErrInvalidPartitionCount
	}
	if !m.StorageProfile.Valid() {
		return fmt.Errorf("%w: %d", wire.ErrInvalidStorageProfile, m.StorageProfile)
	}
	if m.StorageProfile == wire.StorageMemory && m.DurableHealthy {
		return ErrInvalidDurableHealth
	}
	return validateClusterID(m.ClusterID)
}

// HubStatusRequest is a generation-scoped status probe.
type HubStatusRequest struct {
	HubGeneration uint64
}

// Validate checks the hub status request.
func (m HubStatusRequest) Validate() error {
	if m.HubGeneration == 0 {
		return ErrInvalidHubGeneration
	}
	return nil
}

// HubStatus carries the active storage profile on the management path.
type HubStatus struct {
	HubGeneration  uint64
	StorageProfile wire.StorageProfile
	DurableHealthy bool
	Status         wire.Status
}

// Validate checks the hub status payload.
func (m HubStatus) Validate() error {
	if m.HubGeneration == 0 {
		return ErrInvalidHubGeneration
	}
	if !m.Status.Valid() {
		return fmt.Errorf("%w: %s", ErrInvalidStatus, m.Status)
	}
	switch m.Status {
	case wire.StatusOK:
		if !m.StorageProfile.Valid() {
			return fmt.Errorf("%w: %d", wire.ErrInvalidStorageProfile, m.StorageProfile)
		}
		if m.StorageProfile == wire.StorageMemory && m.DurableHealthy {
			return ErrInvalidDurableHealth
		}
		return nil
	case wire.StatusErrBadGeneration, wire.StatusErrInvalidArgument:
		return nil
	default:
		return fmt.Errorf("%w: unexpected hub status %s", ErrInvalidStatus, m.Status)
	}
}

// GetRequest is an authoritative read. HubGeneration is the node's adopted
// generation and must be checked by the hub before lookup.
type GetRequest struct {
	Key           []byte
	MinVersion    *wire.VersionTag
	HubGeneration uint64
}

// Validate checks request bounds. Partition consistency for MinVersion is
// checked by ValidateForPartitions once the hub partition count is known.
func (m GetRequest) Validate() error {
	if m.HubGeneration == 0 {
		return ErrInvalidHubGeneration
	}
	return validateKey(m.Key)
}

// ValidateForPartitions checks key bounds and that MinVersion, when present,
// routes to the same partition as Key.
func (m GetRequest) ValidateForPartitions(partitionCount uint32) error {
	if err := m.Validate(); err != nil {
		return err
	}
	return wire.GetRequest{Key: m.Key, MinVersion: m.MinVersion}.Validate(partitionCount)
}

// Clone returns an ownership-independent copy of m.
func (m GetRequest) Clone() GetRequest {
	cloned := GetRequest{
		Key:           wire.CopyBytes(m.Key),
		HubGeneration: m.HubGeneration,
	}
	if m.MinVersion != nil {
		version := *m.MinVersion
		cloned.MinVersion = &version
	}
	return cloned
}

// GetResponse is an authoritative read result. StatusNotFound carries no
// version; a deleted key is StatusOK with RecordTombstone and a real version.
// StatusNotCaughtUp means MinVersion is above the committed head and is
// retryable with the same request.
type GetResponse struct {
	Status        wire.Status
	HubGeneration uint64
	Version       wire.VersionTag
	Value         []byte
	TTLMillis     uint64
	Kind          wire.RecordKind
}

// Validate checks the frozen authoritative-read response semantics.
func (m GetResponse) Validate() error {
	if m.HubGeneration == 0 {
		return ErrInvalidHubGeneration
	}
	if err := (wire.GetRecordFields{
		Status:  m.Status,
		Version: m.Version,
		Kind:    m.Kind,
		Value:   m.Value,
	}).Validate(); err != nil {
		return err
	}
	if !StatusCarriesGetRecord(m.Status) && m.TTLMillis != 0 {
		return ErrInvalidResultTTL
	}
	return nil
}

// Clone returns an ownership-independent copy of m.
func (m GetResponse) Clone() GetResponse {
	m.Value = wire.CopyBytes(m.Value)
	return m
}

// MutationRequest is an authoritative set or delete. Mode selects WriteFast or
// WriteSync; W is peer confirmation and is independent of Mode.
// HubGeneration is checked by the hub before any commit or sequence assignment.
type MutationRequest struct {
	Op            Op
	Key           []byte
	Value         []byte
	TTLMillis     uint64
	MutationID    wire.MutationID
	Mode          wire.WriteMode
	W             uint16
	Confirm       wire.ConfirmLevel
	Timeout       uint32 // milliseconds; meaningful when W > 0
	HubGeneration uint64
}

// Validate checks mutation bounds and write policy via the shared wire models.
func (m MutationRequest) Validate() error {
	if m.HubGeneration == 0 {
		return ErrInvalidHubGeneration
	}
	if !m.Op.Valid() {
		return fmt.Errorf("%w: %d", ErrInvalidOp, m.Op)
	}
	opts := m.WriteOptions()
	switch m.Op {
	case OpSet:
		return (wire.SetRequest{
			Key:        m.Key,
			Value:      m.Value,
			TTLMillis:  m.TTLMillis,
			MutationID: m.MutationID,
			Options:    opts,
		}).Validate()
	case OpDelete:
		if len(m.Value) != 0 {
			return ErrUnexpectedValue
		}
		if m.TTLMillis != 0 {
			return ErrUnexpectedTTL
		}
		return (wire.DeleteRequest{
			Key:        m.Key,
			MutationID: m.MutationID,
			Options:    opts,
		}).Validate()
	default:
		return fmt.Errorf("%w: %d", ErrInvalidOp, m.Op)
	}
}

// Clone returns an ownership-independent copy of m.
func (m MutationRequest) Clone() MutationRequest {
	m.Key = wire.CopyBytes(m.Key)
	m.Value = wire.CopyBytes(m.Value)
	return m
}

// WriteOptions projects the write-policy fields into the shared wire model.
func (m MutationRequest) WriteOptions() wire.WriteOptions {
	return wire.WriteOptions{
		W:            m.W,
		Mode:         m.Mode,
		ConfirmLevel: m.Confirm,
		Timeout:      time.Duration(m.Timeout) * time.Millisecond,
	}
}

// MutationResponse is the durability and optional W-confirmation outcome of a
// mutation. StatusErrWriteConfirmTimeout still carries the committed Version
// and is never treated as a retryable commit failure.
type MutationResponse struct {
	Status        wire.Status
	HubGeneration uint64
	Version       wire.VersionTag
}

// Validate checks mutation response semantics, including the committed-version
// requirement for success and write-confirm timeout.
func (m MutationResponse) Validate() error {
	if !m.Status.Valid() {
		return fmt.Errorf("%w: %s", ErrInvalidMutationStatus, m.Status)
	}
	switch m.Status {
	case wire.StatusNotFound, wire.StatusNotCaughtUp:
		return fmt.Errorf("%w: %s", ErrInvalidMutationStatus, m.Status)
	}
	if m.HubGeneration == 0 {
		return ErrInvalidHubGeneration
	}
	if StatusCarriesCommittedVersion(m.Status) {
		return m.Version.ValidateCommitted()
	}
	if !m.Version.IsZero() {
		return ErrUnexpectedVersion
	}
	return nil
}

func validateKey(key []byte) error {
	switch {
	case len(key) == 0:
		return wire.ErrKeyEmpty
	case len(key) > wire.MaxKeyLen:
		return fmt.Errorf("%w: got %d, max %d", wire.ErrKeyTooLarge, len(key), wire.MaxKeyLen)
	default:
		return nil
	}
}

func validateClusterID(id string) error {
	if strings.TrimSpace(id) == "" {
		return ErrInvalidClusterID
	}
	return nil
}

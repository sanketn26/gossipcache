// Package rpc defines the versioned data-plane RPC protocol shared by
// GossipCache hubs and nodes.
//
// Transport is length-prefixed request/response frames over mTLS TCP (default
// port DefaultRPCPort). Frames are multiplexed with a 4-byte correlation ID.
// gRPC is intentionally not used in v1 so the wire stays frozen and
// dependency-free.
package rpc

import (
	"errors"
	"fmt"
	"time"

	"github.com/sanketn26/gossipcache/internal/wire"
)

const (
	// HeaderSize is the fixed encoded frame-header size, including the
	// correlation ID and header CRC32C.
	HeaderSize = 20
	// mutationPayloadOverhead is the fixed encoding size of a MutationRequest
	// excluding the key and value bodies:
	// op(1) + keyLen(4) + valueLen(4) + ttl(8) + mutationID(16) + mode(1) +
	// w(2) + confirm(1) + timeout(4).
	mutationPayloadOverhead = 41
	// MaxRPCPayload is the v1 schema-derived payload ceiling: large enough for
	// a maximum-sized MutationRequest (max key + max value + fixed fields).
	// MaxValueLen alone is not enough because payloads also carry key,
	// version/policy metadata, and length prefixes. Recompute this constant
	// when adding or resizing RPC message fields so max legal values remain
	// encodable.
	MaxRPCPayload = wire.MaxValueLen + wire.MaxKeyLen + mutationPayloadOverhead
	// DefaultRPCPort is the conventional hub data-plane listen port.
	DefaultRPCPort = 7400
	// DefaultDedupWindow is how long a hub retains a per-node MutationID
	// outcome so retries join the original waiter or replay its result.
	DefaultDedupWindow = 5 * time.Minute
)

var (
	ErrBadMagic                    = errors.New("invalid rpc frame magic")
	ErrBadHeaderCRC                = errors.New("invalid rpc frame header checksum")
	ErrUnsupportedVersion          = errors.New("unsupported rpc protocol version")
	ErrUnknownMessageType          = errors.New("unknown rpc message type")
	ErrPayloadTooLarge             = errors.New("rpc payload exceeds maximum length")
	ErrTruncatedFrame              = errors.New("truncated rpc frame")
	ErrTrailingPayload             = errors.New("rpc payload has trailing bytes")
	ErrInvalidMessage              = errors.New("invalid rpc message")
	ErrInvalidHubGeneration        = errors.New("hub generation must not be zero")
	ErrInvalidDurableHealth        = errors.New("memory profile cannot advertise durable health")
	ErrInvalidOp                   = errors.New("invalid mutation operation")
	ErrUnexpectedTTL               = errors.New("message must not carry a ttl")
	ErrInvalidMutationStatus       = errors.New("invalid status for mutation response")
	ErrInvalidNodeID               = errors.New("node ID must not be zero")
	ErrMutationFingerprintMismatch = errors.New("mutation fingerprint does not match retained request")

	// Shared get/record errors — aliases of wire so callers can use either package.
	ErrUnexpectedValue   = wire.ErrUnexpectedValue
	ErrUnexpectedVersion = wire.ErrUnexpectedVersion
	ErrMissingVersion    = wire.ErrMissingVersion
	ErrInvalidGetStatus  = wire.ErrInvalidGetStatus
	ErrInvalidRecordKind = wire.ErrInvalidRecordKind
	ErrInvalidResultTTL  = wire.ErrInvalidResultTTL
)

// MessageType is the closed set of data-plane payload schemas.
type MessageType uint16

const (
	MessageHandshakeRequest MessageType = 1
	MessageHandshake        MessageType = 2
	MessageHubStatus        MessageType = 3
	MessageGetRequest       MessageType = 4
	MessageGetResponse      MessageType = 5
	MessageMutationRequest  MessageType = 6
	MessageMutationResponse MessageType = 7
)

// Valid reports whether t has a schema in this protocol version.
func (t MessageType) Valid() bool {
	switch t {
	case MessageHandshakeRequest,
		MessageHandshake,
		MessageHubStatus,
		MessageGetRequest,
		MessageGetResponse,
		MessageMutationRequest,
		MessageMutationResponse:
		return true
	default:
		return false
	}
}

// Message is implemented by every RPC payload.
type Message interface {
	Type() MessageType
	Validate() error
}

// Op is a mutation operation carried on MutationRequest.
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
type HandshakeRequest struct {
	Protocol wire.ProtocolRange
}

func (HandshakeRequest) Type() MessageType { return MessageHandshakeRequest }

// Validate checks the handshake request payload.
func (m HandshakeRequest) Validate() error {
	return m.Protocol.Validate()
}

// Handshake is the hub's session advertisement. StorageProfile is fixed for
// the hub lifetime; DurableHealthy gates WriteSync on a durable-profile hub.
type Handshake struct {
	ProtocolVersion wire.ProtocolVersion
	HubGeneration   uint64
	PartitionCount  uint32
	StorageProfile  wire.StorageProfile
	DurableHealthy  bool
}

func (Handshake) Type() MessageType { return MessageHandshake }

// Validate checks the hub handshake payload.
func (m Handshake) Validate() error {
	if !supportedVersion(m.ProtocolVersion) {
		return fmt.Errorf("%w: %d", ErrUnsupportedVersion, m.ProtocolVersion)
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
	return nil
}

// HubStatus carries the active storage profile on the management path.
type HubStatus struct {
	HubGeneration  uint64
	StorageProfile wire.StorageProfile
	DurableHealthy bool
}

func (HubStatus) Type() MessageType { return MessageHubStatus }

// Validate checks the hub status payload.
func (m HubStatus) Validate() error {
	if m.HubGeneration == 0 {
		return ErrInvalidHubGeneration
	}
	if !m.StorageProfile.Valid() {
		return fmt.Errorf("%w: %d", wire.ErrInvalidStorageProfile, m.StorageProfile)
	}
	if m.StorageProfile == wire.StorageMemory && m.DurableHealthy {
		return ErrInvalidDurableHealth
	}
	return nil
}

// GetRequest is an authoritative read.
type GetRequest struct {
	Key        []byte
	MinVersion *wire.VersionTag
}

func (GetRequest) Type() MessageType { return MessageGetRequest }

// Validate checks request bounds. Partition consistency for MinVersion is
// checked by ValidateForPartitions once the hub partition count is known.
func (m GetRequest) Validate() error {
	return validateKey(m.Key)
}

// ValidateForPartitions checks key bounds and that MinVersion, when present,
// routes to the same partition as Key.
func (m GetRequest) ValidateForPartitions(partitionCount uint32) error {
	return wire.GetRequest{Key: m.Key, MinVersion: m.MinVersion}.Validate(partitionCount)
}

// Clone returns an ownership-independent copy of m.
func (m GetRequest) Clone() GetRequest {
	cloned := GetRequest{Key: wire.CopyBytes(m.Key)}
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

func (GetResponse) Type() MessageType { return MessageGetResponse }

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
type MutationRequest struct {
	Op         Op
	Key        []byte
	Value      []byte
	TTLMillis  uint64
	MutationID wire.MutationID
	Mode       wire.WriteMode
	W          uint16
	Confirm    wire.ConfirmLevel
	Timeout    uint32 // milliseconds; meaningful when W > 0
}

func (MutationRequest) Type() MessageType { return MessageMutationRequest }

// Validate checks mutation bounds and write policy via the shared wire models.
func (m MutationRequest) Validate() error {
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

func (MutationResponse) Type() MessageType { return MessageMutationResponse }

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

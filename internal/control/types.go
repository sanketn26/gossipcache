// Package control defines the versioned control-stream protocol shared by
// GossipCache hubs and nodes.
package control

import (
	"errors"
	"fmt"
	"time"

	"github.com/sanketn26/gossipcache/internal/wire"
)

const (
	// HeaderSize is the fixed encoded frame-header size.
	HeaderSize = 16
	// MaxControlPayload bounds one control frame payload.
	MaxControlPayload = 1 << 20
	// MaxBatchEvents prevents tiny event encodings from creating unbounded
	// allocation pressure during decode.
	MaxBatchEvents = 4096
	// MaxSubscriptions bounds the partitions advertised by one connection.
	MaxSubscriptions = 4096
	// DefaultSubscriberQueue is the default per-subscriber event capacity.
	DefaultSubscriberQueue = 4096
)

const (
	// DefaultCheckpointInterval is the maximum default idle period between
	// authoritative stream checkpoints.
	DefaultCheckpointInterval = time.Second
	// DefaultStreamFreshnessTimeout gates readiness after three missed default
	// checkpoint intervals.
	DefaultStreamFreshnessTimeout = 3 * time.Second
)

var (
	ErrBadMagic               = errors.New("invalid control frame magic")
	ErrBadHeaderCRC           = errors.New("invalid control frame header checksum")
	ErrUnsupportedVersion     = errors.New("unsupported control protocol version")
	ErrUnknownMessageType     = errors.New("unknown control message type")
	ErrPayloadTooLarge        = errors.New("control payload exceeds maximum length")
	ErrTruncatedFrame         = errors.New("truncated control frame")
	ErrTrailingPayload        = errors.New("control payload has trailing bytes")
	ErrInvalidMessage         = errors.New("invalid control message")
	ErrInvalidHubGeneration   = errors.New("hub generation must not be zero")
	ErrInvalidNodeID          = errors.New("node ID must not be zero")
	ErrInvalidSequence        = errors.New("stream sequence must not be zero")
	ErrInvalidSequenceRange   = errors.New("invalid stream sequence range")
	ErrNonContiguousBatch     = errors.New("invalidation batch is not contiguous")
	ErrPartitionMismatch      = errors.New("event version partition does not match stream")
	ErrInvalidRecordKind      = errors.New("invalid invalidation record kind")
	ErrTooManyEvents          = errors.New("invalidation batch exceeds event limit")
	ErrTooManySubscriptions   = errors.New("subscription count exceeds limit")
	ErrDuplicateSubscription  = errors.New("duplicate stream subscription")
	ErrEmptySubscriptions     = errors.New("subscription list must not be empty")
	ErrEmptyInvalidationBatch = errors.New("invalidation batch must not be empty")
)

// MessageType is the closed set of control-frame payload schemas.
type MessageType uint16

const (
	MessageHello                 MessageType = 1
	MessageSubscribe             MessageType = 2
	MessageInvalidationBatch     MessageType = 3
	MessageHopFrameAck           MessageType = 4
	MessageStreamAcknowledgement MessageType = 5
	MessageStreamCheckpoint      MessageType = 6
	MessageReplayRequest         MessageType = 7
	MessageReplayUnavailable     MessageType = 8
	MessageInvalidateConfirm     MessageType = 9
)

// Valid reports whether t has a schema in this protocol version.
func (t MessageType) Valid() bool {
	switch t {
	case MessageHello,
		MessageSubscribe,
		MessageInvalidationBatch,
		MessageHopFrameAck,
		MessageStreamAcknowledgement,
		MessageStreamCheckpoint,
		MessageReplayRequest,
		MessageReplayUnavailable,
		MessageInvalidateConfirm:
		return true
	default:
		return false
	}
}

// Message is implemented by every control payload.
type Message interface {
	Type() MessageType
	Validate() error
}

// StreamWatermark describes a directly subscribed partition when reconnecting.
type StreamWatermark struct {
	StreamID       uint32
	AppliedThrough uint64
	HubGeneration  uint64
}

// Hello identifies a node, advertises its supported protocol range, and
// carries the application watermarks needed to resume direct subscriptions.
type Hello struct {
	NodeID        uint64
	Protocol      wire.ProtocolRange
	Subscriptions []StreamWatermark
}

func (Hello) Type() MessageType { return MessageHello }

// Validate checks the hello payload.
func (m Hello) Validate() error {
	if m.NodeID == 0 {
		return ErrInvalidNodeID
	}
	if err := m.Protocol.Validate(); err != nil {
		return err
	}
	if len(m.Subscriptions) > MaxSubscriptions {
		return ErrTooManySubscriptions
	}
	seen := make(map[uint32]struct{}, len(m.Subscriptions))
	for _, subscription := range m.Subscriptions {
		if subscription.HubGeneration == 0 {
			return ErrInvalidHubGeneration
		}
		if _, ok := seen[subscription.StreamID]; ok {
			return fmt.Errorf("%w: %d", ErrDuplicateSubscription, subscription.StreamID)
		}
		seen[subscription.StreamID] = struct{}{}
	}
	return nil
}

// Clone returns an ownership-independent hello.
func (m Hello) Clone() Hello {
	m.Subscriptions = append([]StreamWatermark(nil), m.Subscriptions...)
	return m
}

// Subscribe requests direct Hub delivery for the listed partition streams.
type Subscribe struct {
	HubGeneration uint64
	StreamIDs     []uint32
}

func (Subscribe) Type() MessageType { return MessageSubscribe }

// Validate checks the subscribe payload.
func (m Subscribe) Validate() error {
	if m.HubGeneration == 0 {
		return ErrInvalidHubGeneration
	}
	if len(m.StreamIDs) == 0 {
		return ErrEmptySubscriptions
	}
	if len(m.StreamIDs) > MaxSubscriptions {
		return ErrTooManySubscriptions
	}
	seen := make(map[uint32]struct{}, len(m.StreamIDs))
	for _, streamID := range m.StreamIDs {
		if _, ok := seen[streamID]; ok {
			return fmt.Errorf("%w: %d", ErrDuplicateSubscription, streamID)
		}
		seen[streamID] = struct{}{}
	}
	return nil
}

// Clone returns an ownership-independent subscription request.
func (m Subscribe) Clone() Subscribe {
	m.StreamIDs = append([]uint32(nil), m.StreamIDs...)
	return m
}

// InvalidationBatch carries a contiguous run of authoritative invalidations.
type InvalidationBatch struct {
	StreamID      uint32
	HubGeneration uint64
	Events        []InvalidationEvent
}

func (InvalidationBatch) Type() MessageType { return MessageInvalidationBatch }

// Validate checks batch bounds, ordering, and partition ownership.
func (m InvalidationBatch) Validate() error {
	if m.HubGeneration == 0 {
		return ErrInvalidHubGeneration
	}
	if len(m.Events) == 0 {
		return ErrEmptyInvalidationBatch
	}
	if len(m.Events) > MaxBatchEvents {
		return ErrTooManyEvents
	}
	var previous uint64
	encodedSize := 16 // stream ID, hub generation, and event count
	for i, event := range m.Events {
		if err := event.validate(m.StreamID); err != nil {
			return fmt.Errorf("event %d: %w", i, err)
		}
		encodedSize += 41 + len(event.Key)
		if encodedSize > MaxControlPayload {
			return ErrPayloadTooLarge
		}
		if i > 0 && event.StreamSequence != previous+1 {
			return fmt.Errorf("%w: previous=%d current=%d", ErrNonContiguousBatch, previous, event.StreamSequence)
		}
		previous = event.StreamSequence
	}
	return nil
}

// Clone returns an ownership-independent invalidation batch.
func (m InvalidationBatch) Clone() InvalidationBatch {
	m.Events = append([]InvalidationEvent(nil), m.Events...)
	for i := range m.Events {
		m.Events[i].Key = wire.CopyBytes(m.Events[i].Key)
	}
	return m
}

// InvalidationEvent is one hub-numbered stream delivery.
type InvalidationEvent struct {
	StreamSequence uint64
	Key            []byte
	Version        wire.VersionTag
	Kind           wire.RecordKind
	MutationID     wire.MutationID
}

func (e InvalidationEvent) validate(streamID uint32) error {
	if e.StreamSequence == 0 || e.Version.Sequence == 0 {
		return ErrInvalidSequence
	}
	if len(e.Key) == 0 {
		return wire.ErrKeyEmpty
	}
	if len(e.Key) > wire.MaxKeyLen {
		return wire.ErrKeyTooLarge
	}
	if e.Version.PartitionID != streamID {
		return ErrPartitionMismatch
	}
	if !e.Kind.Valid() {
		return ErrInvalidRecordKind
	}
	if e.MutationID.IsZero() {
		return wire.ErrMutationIDRequired
	}
	return nil
}

// HopFrameAck confirms decoded transport receipt through a stream sequence. It
// does not mean that the node state machine applied the invalidation.
type HopFrameAck struct {
	StreamID        uint32
	ReceivedThrough uint64
}

func (HopFrameAck) Type() MessageType { return MessageHopFrameAck }
func (m HopFrameAck) Validate() error {
	if m.ReceivedThrough == 0 {
		return ErrInvalidSequence
	}
	return nil
}

// StreamAcknowledgement confirms state-machine application through a sequence.
type StreamAcknowledgement struct {
	StreamID       uint32
	AppliedThrough uint64
}

func (StreamAcknowledgement) Type() MessageType { return MessageStreamAcknowledgement }
func (m StreamAcknowledgement) Validate() error {
	if m.AppliedThrough == 0 {
		return ErrInvalidSequence
	}
	return nil
}

// StreamCheckpoint communicates an idle stream's authoritative head.
type StreamCheckpoint struct {
	StreamID      uint32
	HubGeneration uint64
	StreamHead    uint64
}

func (StreamCheckpoint) Type() MessageType { return MessageStreamCheckpoint }
func (m StreamCheckpoint) Validate() error {
	if m.HubGeneration == 0 {
		return ErrInvalidHubGeneration
	}
	return nil
}

// ReplayRequest asks the hub for an inclusive sequence range.
type ReplayRequest struct {
	StreamID     uint32
	FromSequence uint64
	ToSequence   uint64
}

func (ReplayRequest) Type() MessageType { return MessageReplayRequest }
func (m ReplayRequest) Validate() error {
	if m.FromSequence == 0 || m.ToSequence < m.FromSequence {
		return ErrInvalidSequenceRange
	}
	return nil
}

// ReplayUnavailable reports the retained replay window after a failed request.
// OldestAvailable is StreamHead+1 when the retained window is empty.
type ReplayUnavailable struct {
	StreamID        uint32
	HubGeneration   uint64
	RequestedFrom   uint64
	OldestAvailable uint64
	StreamHead      uint64
}

func (ReplayUnavailable) Type() MessageType { return MessageReplayUnavailable }
func (m ReplayUnavailable) Validate() error {
	if m.HubGeneration == 0 {
		return ErrInvalidHubGeneration
	}
	if m.RequestedFrom == 0 || m.OldestAvailable == 0 {
		return ErrInvalidSequenceRange
	}
	if m.RequestedFrom >= m.OldestAvailable {
		return ErrInvalidSequenceRange
	}
	if m.OldestAvailable > m.StreamHead && m.OldestAvailable-m.StreamHead != 1 {
		return ErrInvalidSequenceRange
	}
	return nil
}

// InvalidateConfirm confirms application for W correlation.
type InvalidateConfirm struct {
	StreamID       uint32
	StreamSequence uint64
	NodeID         uint64
}

func (InvalidateConfirm) Type() MessageType { return MessageInvalidateConfirm }
func (m InvalidateConfirm) Validate() error {
	if m.StreamSequence == 0 {
		return ErrInvalidSequence
	}
	if m.NodeID == 0 {
		return ErrInvalidNodeID
	}
	return nil
}

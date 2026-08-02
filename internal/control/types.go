// Package control holds control-plane stream semantics above the gRPC Control
// service. Framing is protobuf (api/proto/gossipcache/v1); this package owns
// delivery-rule validation, defaults, and wire↔proto conversion.
package control

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sanketn26/gossipcache/internal/wire"
)

const (
	// MaxBatchEvents prevents tiny event encodings from creating unbounded
	// allocation pressure during decode/apply. Encoded size is the binding
	// limit; this is a secondary cap.
	MaxBatchEvents = 4096
	// MaxControlMessageBytes is the maximum encoded size of any single
	// ControlClientMessage or ControlServerMessage (including
	// InvalidationBatch). It is frozen at 3 MiB so messages fit inside the
	// default gRPC 4 MiB send/receive limit with headroom for framing.
	// Hub and Node gRPC servers/clients MUST set MaxSendMsgSize and
	// MaxRecvMsgSize to at least this value (defaults already are 4 MiB).
	MaxControlMessageBytes = 3 << 20
	// controlMessageOverhead is a conservative protobuf encoding budget for
	// envelope tags and batch fixed fields (not including per-event bodies).
	controlMessageOverhead = 128
	// eventEncodedOverhead is a conservative per-event protobuf budget excluding
	// the key bytes (stream_sequence, version, kind, mutation_id, field tags).
	eventEncodedOverhead = 64
	// MaxSubscriptions bounds the partitions advertised by one connection.
	MaxSubscriptions = 4096
	// DefaultSubscriberQueue is the default per-subscriber event capacity.
	DefaultSubscriberQueue = 4096
	// DefaultCheckpointInterval is the maximum default idle period between
	// authoritative stream checkpoints.
	DefaultCheckpointInterval = time.Second
	// DefaultStreamFreshnessTimeout gates readiness after three missed default
	// checkpoint intervals.
	DefaultStreamFreshnessTimeout = 3 * time.Second
	// MaxControlErrorDetail is the largest ControlError.detail string.
	MaxControlErrorDetail = 256
)

var (
	ErrInvalidHubGeneration   = errors.New("hub generation must not be zero")
	ErrInvalidNodeID          = errors.New("node ID must not be zero")
	ErrInvalidSequence        = errors.New("stream sequence must not be zero")
	ErrInvalidSequenceRange   = errors.New("invalid stream sequence range")
	ErrNonContiguousBatch     = errors.New("invalidation batch is not contiguous")
	ErrPartitionMismatch      = errors.New("event version partition does not match stream")
	ErrInvalidRecordKind      = errors.New("invalid invalidation record kind")
	ErrTooManyEvents          = errors.New("invalidation batch exceeds event limit")
	ErrBatchTooLarge          = errors.New("invalidation batch exceeds encoded size limit")
	ErrTooManySubscriptions   = errors.New("subscription count exceeds limit")
	ErrDuplicateSubscription  = errors.New("duplicate stream subscription")
	ErrEmptySubscriptions     = errors.New("subscription list must not be empty")
	ErrEmptyInvalidationBatch = errors.New("invalidation batch must not be empty")
	ErrInvalidMessage         = errors.New("invalid control message")
	ErrInvalidClusterID       = errors.New("cluster id must not be empty")
	ErrInvalidControlStatus   = errors.New("invalid control error status")
	ErrControlDetailTooLarge  = errors.New("control error detail exceeds limit")
)

// StreamWatermark describes a directly subscribed partition when reconnecting.
type StreamWatermark struct {
	StreamID       uint32
	AppliedThrough uint64
	HubGeneration  uint64
}

// Hello identifies a node, advertises its supported protocol range, and
// carries the application watermarks needed to resume direct subscriptions.
// ExpectedHubGeneration zero means bootstrap (adopt the hub generation).
type Hello struct {
	NodeID                uint64
	Protocol              wire.ProtocolRange
	Subscriptions         []StreamWatermark
	ClusterID             string
	ExpectedHubGeneration uint64
}

// Validate checks the hello payload.
func (m Hello) Validate() error {
	if m.NodeID == 0 {
		return ErrInvalidNodeID
	}
	if err := m.Protocol.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(m.ClusterID) == "" {
		return ErrInvalidClusterID
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

// Validate checks batch bounds, ordering, partition ownership, and encoded size.
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
	for i, event := range m.Events {
		if err := event.validate(m.StreamID); err != nil {
			return fmt.Errorf("event %d: %w", i, err)
		}
		if i > 0 && event.StreamSequence != previous+1 {
			return fmt.Errorf("%w: previous=%d current=%d", ErrNonContiguousBatch, previous, event.StreamSequence)
		}
		previous = event.StreamSequence
	}
	if m.EstimatedEncodedSize() > MaxControlMessageBytes {
		return fmt.Errorf("%w: estimated %d bytes, max %d", ErrBatchTooLarge, m.EstimatedEncodedSize(), MaxControlMessageBytes)
	}
	return nil
}

// EstimatedEncodedSize returns a conservative upper bound on the protobuf
// encoding size of this batch wrapped as a ControlServerMessage. Hubs must
// split publish batches so each message stays within MaxControlMessageBytes.
func (m InvalidationBatch) EstimatedEncodedSize() int {
	n := controlMessageOverhead
	for _, event := range m.Events {
		n += eventEncodedOverhead + len(event.Key)
	}
	return n
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

// StreamAcknowledgement confirms state-machine application through a sequence.
// Transport receipt is gRPC flow control; only application apply is ack'd here.
type StreamAcknowledgement struct {
	StreamID       uint32
	AppliedThrough uint64
}

// Validate checks the acknowledgement.
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

// Validate checks the checkpoint.
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

// Validate checks the replay range.
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

// Validate checks the unavailable report.
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

// Validate checks the confirm message.
func (m InvalidateConfirm) Validate() error {
	if m.StreamSequence == 0 {
		return ErrInvalidSequence
	}
	if m.NodeID == 0 {
		return ErrInvalidNodeID
	}
	return nil
}

// ControlError is an application-level error on the control stream.
// Allowed statuses in v1: ERR_RATE_LIMITED, ERR_BAD_GENERATION,
// ERR_INVALID_ARGUMENT, ERR_INTERNAL.
type ControlError struct {
	Status wire.Status
	Detail string
}

// Validate checks the control error payload.
func (m ControlError) Validate() error {
	switch m.Status {
	case wire.StatusErrRateLimited,
		wire.StatusErrBadGeneration,
		wire.StatusErrInvalidArgument,
		wire.StatusErrInternal:
		// ok
	default:
		return fmt.Errorf("%w: %s", ErrInvalidControlStatus, m.Status)
	}
	if len(m.Detail) > MaxControlErrorDetail {
		return ErrControlDetailTooLarge
	}
	return nil
}

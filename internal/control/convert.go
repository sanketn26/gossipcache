package control

import (
	"fmt"
	"math"

	pb "github.com/sanketn26/gossipcache/api/gen/gossipcache/v1"
	"github.com/sanketn26/gossipcache/internal/wire"
)

// ProtoHello converts a domain Hello to protobuf.
func ProtoHello(m Hello) *pb.Hello {
	out := &pb.Hello{
		NodeId:                m.NodeID,
		ProtocolVersion:       uint32(m.Protocol.Version),
		MinSupportedVersion:   uint32(m.Protocol.MinSupported),
		Subscriptions:         make([]*pb.StreamWatermark, 0, len(m.Subscriptions)),
		ClusterId:             m.ClusterID,
		ExpectedHubGeneration: m.ExpectedHubGeneration,
	}
	for _, s := range m.Subscriptions {
		out.Subscriptions = append(out.Subscriptions, &pb.StreamWatermark{
			StreamId:       s.StreamID,
			AppliedThrough: s.AppliedThrough,
			HubGeneration:  s.HubGeneration,
		})
	}
	return out
}

// ProtoControlError converts a domain control error to protobuf.
func ProtoControlError(m ControlError) *pb.ControlError {
	return &pb.ControlError{
		Status: pb.Status(m.Status),
		Detail: m.Detail,
	}
}

// ProtoSubscribe converts a domain Subscribe to protobuf.
func ProtoSubscribe(m Subscribe) *pb.Subscribe {
	return &pb.Subscribe{
		HubGeneration: m.HubGeneration,
		StreamIds:     append([]uint32(nil), m.StreamIDs...),
	}
}

// ProtoInvalidationBatch converts a domain batch to protobuf.
func ProtoInvalidationBatch(m InvalidationBatch) *pb.InvalidationBatch {
	out := &pb.InvalidationBatch{
		StreamId:      m.StreamID,
		HubGeneration: m.HubGeneration,
		Events:        make([]*pb.InvalidationEvent, 0, len(m.Events)),
	}
	for _, e := range m.Events {
		out.Events = append(out.Events, &pb.InvalidationEvent{
			StreamSequence: e.StreamSequence,
			Key:            wire.CopyBytes(e.Key),
			Version: &pb.VersionTag{
				PartitionId: e.Version.PartitionID,
				Sequence:    e.Version.Sequence,
			},
			Kind:       pb.RecordKind(e.Kind),
			MutationId: &pb.MutationID{Id: e.MutationID[:]},
		})
	}
	return out
}

// ProtoStreamAcknowledgement converts a domain ack to protobuf.
func ProtoStreamAcknowledgement(m StreamAcknowledgement) *pb.StreamAcknowledgement {
	return &pb.StreamAcknowledgement{
		StreamId:       m.StreamID,
		AppliedThrough: m.AppliedThrough,
	}
}

// ProtoStreamCheckpoint converts a domain checkpoint to protobuf.
func ProtoStreamCheckpoint(m StreamCheckpoint) *pb.StreamCheckpoint {
	return &pb.StreamCheckpoint{
		StreamId:      m.StreamID,
		HubGeneration: m.HubGeneration,
		StreamHead:    m.StreamHead,
	}
}

// ProtoReplayRequest converts a domain replay request to protobuf.
func ProtoReplayRequest(m ReplayRequest) *pb.ReplayRequest {
	return &pb.ReplayRequest{
		StreamId:     m.StreamID,
		FromSequence: m.FromSequence,
		ToSequence:   m.ToSequence,
	}
}

// ProtoReplayUnavailable converts a domain unavailable report to protobuf.
func ProtoReplayUnavailable(m ReplayUnavailable) *pb.ReplayUnavailable {
	return &pb.ReplayUnavailable{
		StreamId:        m.StreamID,
		HubGeneration:   m.HubGeneration,
		RequestedFrom:   m.RequestedFrom,
		OldestAvailable: m.OldestAvailable,
		StreamHead:      m.StreamHead,
	}
}

// ProtoInvalidateConfirm converts a domain confirm to protobuf.
func ProtoInvalidateConfirm(m InvalidateConfirm) *pb.InvalidateConfirm {
	return &pb.InvalidateConfirm{
		StreamId:       m.StreamID,
		StreamSequence: m.StreamSequence,
		NodeId:         m.NodeID,
	}
}

// ProtoClientMessage wraps a client payload in the stream envelope.
func ProtoClientMessage(body any) (*pb.ControlClientMessage, error) {
	msg := &pb.ControlClientMessage{}
	switch v := body.(type) {
	case Hello:
		msg.Body = &pb.ControlClientMessage_Hello{Hello: ProtoHello(v)}
	case *Hello:
		if v == nil {
			return nil, fmt.Errorf("%w: nil *Hello", ErrInvalidMessage)
		}
		msg.Body = &pb.ControlClientMessage_Hello{Hello: ProtoHello(*v)}
	case Subscribe:
		msg.Body = &pb.ControlClientMessage_Subscribe{Subscribe: ProtoSubscribe(v)}
	case *Subscribe:
		if v == nil {
			return nil, fmt.Errorf("%w: nil *Subscribe", ErrInvalidMessage)
		}
		msg.Body = &pb.ControlClientMessage_Subscribe{Subscribe: ProtoSubscribe(*v)}
	case StreamAcknowledgement:
		msg.Body = &pb.ControlClientMessage_StreamAcknowledgement{StreamAcknowledgement: ProtoStreamAcknowledgement(v)}
	case *StreamAcknowledgement:
		if v == nil {
			return nil, fmt.Errorf("%w: nil *StreamAcknowledgement", ErrInvalidMessage)
		}
		msg.Body = &pb.ControlClientMessage_StreamAcknowledgement{StreamAcknowledgement: ProtoStreamAcknowledgement(*v)}
	case ReplayRequest:
		msg.Body = &pb.ControlClientMessage_ReplayRequest{ReplayRequest: ProtoReplayRequest(v)}
	case *ReplayRequest:
		if v == nil {
			return nil, fmt.Errorf("%w: nil *ReplayRequest", ErrInvalidMessage)
		}
		msg.Body = &pb.ControlClientMessage_ReplayRequest{ReplayRequest: ProtoReplayRequest(*v)}
	case InvalidateConfirm:
		msg.Body = &pb.ControlClientMessage_InvalidateConfirm{InvalidateConfirm: ProtoInvalidateConfirm(v)}
	case *InvalidateConfirm:
		if v == nil {
			return nil, fmt.Errorf("%w: nil *InvalidateConfirm", ErrInvalidMessage)
		}
		msg.Body = &pb.ControlClientMessage_InvalidateConfirm{InvalidateConfirm: ProtoInvalidateConfirm(*v)}
	default:
		return nil, fmt.Errorf("%w: unsupported client body %T", ErrInvalidMessage, body)
	}
	return msg, nil
}

// ProtoServerMessage wraps a server payload in the stream envelope.
func ProtoServerMessage(body any) (*pb.ControlServerMessage, error) {
	msg := &pb.ControlServerMessage{}
	switch v := body.(type) {
	case InvalidationBatch:
		msg.Body = &pb.ControlServerMessage_InvalidationBatch{InvalidationBatch: ProtoInvalidationBatch(v)}
	case *InvalidationBatch:
		if v == nil {
			return nil, fmt.Errorf("%w: nil *InvalidationBatch", ErrInvalidMessage)
		}
		msg.Body = &pb.ControlServerMessage_InvalidationBatch{InvalidationBatch: ProtoInvalidationBatch(*v)}
	case StreamCheckpoint:
		msg.Body = &pb.ControlServerMessage_StreamCheckpoint{StreamCheckpoint: ProtoStreamCheckpoint(v)}
	case *StreamCheckpoint:
		if v == nil {
			return nil, fmt.Errorf("%w: nil *StreamCheckpoint", ErrInvalidMessage)
		}
		msg.Body = &pb.ControlServerMessage_StreamCheckpoint{StreamCheckpoint: ProtoStreamCheckpoint(*v)}
	case ReplayUnavailable:
		msg.Body = &pb.ControlServerMessage_ReplayUnavailable{ReplayUnavailable: ProtoReplayUnavailable(v)}
	case *ReplayUnavailable:
		if v == nil {
			return nil, fmt.Errorf("%w: nil *ReplayUnavailable", ErrInvalidMessage)
		}
		msg.Body = &pb.ControlServerMessage_ReplayUnavailable{ReplayUnavailable: ProtoReplayUnavailable(*v)}
	case ControlError:
		msg.Body = &pb.ControlServerMessage_ControlError{ControlError: ProtoControlError(v)}
	case *ControlError:
		if v == nil {
			return nil, fmt.Errorf("%w: nil *ControlError", ErrInvalidMessage)
		}
		msg.Body = &pb.ControlServerMessage_ControlError{ControlError: ProtoControlError(*v)}
	default:
		return nil, fmt.Errorf("%w: unsupported server body %T", ErrInvalidMessage, body)
	}
	return msg, nil
}

// FromProtoHello converts a protobuf Hello.
func FromProtoHello(m *pb.Hello) (Hello, error) {
	if m == nil {
		return Hello{}, fmt.Errorf("%w: nil hello", ErrInvalidMessage)
	}
	version, err := protocolVersionFromUint32(m.GetProtocolVersion())
	if err != nil {
		return Hello{}, err
	}
	minimum, err := protocolVersionFromUint32(m.GetMinSupportedVersion())
	if err != nil {
		return Hello{}, err
	}
	out := Hello{
		NodeID: m.GetNodeId(),
		Protocol: wire.ProtocolRange{
			Version:      version,
			MinSupported: minimum,
		},
		Subscriptions:         make([]StreamWatermark, 0, len(m.GetSubscriptions())),
		ClusterID:             m.GetClusterId(),
		ExpectedHubGeneration: m.GetExpectedHubGeneration(),
	}
	for _, s := range m.GetSubscriptions() {
		if s == nil {
			return Hello{}, fmt.Errorf("%w: nil subscription watermark", ErrInvalidMessage)
		}
		out.Subscriptions = append(out.Subscriptions, StreamWatermark{
			StreamID:       s.GetStreamId(),
			AppliedThrough: s.GetAppliedThrough(),
			HubGeneration:  s.GetHubGeneration(),
		})
	}
	return out, out.Validate()
}

// FromProtoSubscribe converts a protobuf Subscribe.
func FromProtoSubscribe(m *pb.Subscribe) (Subscribe, error) {
	if m == nil {
		return Subscribe{}, fmt.Errorf("%w: nil subscribe", ErrInvalidMessage)
	}
	out := Subscribe{
		HubGeneration: m.GetHubGeneration(),
		StreamIDs:     append([]uint32(nil), m.GetStreamIds()...),
	}
	return out, out.Validate()
}

// FromProtoInvalidationBatch converts a protobuf batch.
func FromProtoInvalidationBatch(m *pb.InvalidationBatch) (InvalidationBatch, error) {
	if m == nil {
		return InvalidationBatch{}, fmt.Errorf("%w: nil invalidation batch", ErrInvalidMessage)
	}
	out := InvalidationBatch{
		StreamID:      m.GetStreamId(),
		HubGeneration: m.GetHubGeneration(),
		Events:        make([]InvalidationEvent, 0, len(m.GetEvents())),
	}
	for i, e := range m.GetEvents() {
		if e == nil {
			return InvalidationBatch{}, fmt.Errorf("%w: nil event at index %d", ErrInvalidMessage, i)
		}
		id, err := mutationIDFromProto(e.GetMutationId())
		if err != nil {
			return InvalidationBatch{}, err
		}
		kind, err := recordKindFromProto(e.GetKind())
		if err != nil {
			return InvalidationBatch{}, err
		}
		var version wire.VersionTag
		if v := e.GetVersion(); v != nil {
			version = wire.VersionTag{PartitionID: v.GetPartitionId(), Sequence: v.GetSequence()}
		}
		out.Events = append(out.Events, InvalidationEvent{
			StreamSequence: e.GetStreamSequence(),
			Key:            wire.CopyBytes(e.GetKey()),
			Version:        version,
			Kind:           kind,
			MutationID:     id,
		})
	}
	return out, out.Validate()
}

// FromProtoStreamAcknowledgement converts a protobuf ack.
func FromProtoStreamAcknowledgement(m *pb.StreamAcknowledgement) (StreamAcknowledgement, error) {
	if m == nil {
		return StreamAcknowledgement{}, fmt.Errorf("%w: nil stream acknowledgement", ErrInvalidMessage)
	}
	out := StreamAcknowledgement{
		StreamID:       m.GetStreamId(),
		AppliedThrough: m.GetAppliedThrough(),
	}
	return out, out.Validate()
}

// FromProtoStreamCheckpoint converts a protobuf checkpoint.
func FromProtoStreamCheckpoint(m *pb.StreamCheckpoint) (StreamCheckpoint, error) {
	if m == nil {
		return StreamCheckpoint{}, fmt.Errorf("%w: nil stream checkpoint", ErrInvalidMessage)
	}
	out := StreamCheckpoint{
		StreamID:      m.GetStreamId(),
		HubGeneration: m.GetHubGeneration(),
		StreamHead:    m.GetStreamHead(),
	}
	return out, out.Validate()
}

// FromProtoReplayRequest converts a protobuf replay request.
func FromProtoReplayRequest(m *pb.ReplayRequest) (ReplayRequest, error) {
	if m == nil {
		return ReplayRequest{}, fmt.Errorf("%w: nil replay request", ErrInvalidMessage)
	}
	out := ReplayRequest{
		StreamID:     m.GetStreamId(),
		FromSequence: m.GetFromSequence(),
		ToSequence:   m.GetToSequence(),
	}
	return out, out.Validate()
}

// FromProtoReplayUnavailable converts a protobuf unavailable report.
func FromProtoReplayUnavailable(m *pb.ReplayUnavailable) (ReplayUnavailable, error) {
	if m == nil {
		return ReplayUnavailable{}, fmt.Errorf("%w: nil replay unavailable", ErrInvalidMessage)
	}
	out := ReplayUnavailable{
		StreamID:        m.GetStreamId(),
		HubGeneration:   m.GetHubGeneration(),
		RequestedFrom:   m.GetRequestedFrom(),
		OldestAvailable: m.GetOldestAvailable(),
		StreamHead:      m.GetStreamHead(),
	}
	return out, out.Validate()
}

// FromProtoInvalidateConfirm converts a protobuf confirm.
func FromProtoInvalidateConfirm(m *pb.InvalidateConfirm) (InvalidateConfirm, error) {
	if m == nil {
		return InvalidateConfirm{}, fmt.Errorf("%w: nil invalidate confirm", ErrInvalidMessage)
	}
	out := InvalidateConfirm{
		StreamID:       m.GetStreamId(),
		StreamSequence: m.GetStreamSequence(),
		NodeID:         m.GetNodeId(),
	}
	return out, out.Validate()
}

// FromProtoControlError converts a protobuf control error.
func FromProtoControlError(m *pb.ControlError) (ControlError, error) {
	if m == nil {
		return ControlError{}, fmt.Errorf("%w: nil control error", ErrInvalidMessage)
	}
	status, err := statusFromProto(m.GetStatus())
	if err != nil {
		return ControlError{}, err
	}
	out := ControlError{
		Status: status,
		Detail: m.GetDetail(),
	}
	return out, out.Validate()
}

// FromProtoClientMessage unpacks a client stream envelope into a domain value.
func FromProtoClientMessage(m *pb.ControlClientMessage) (any, error) {
	if m == nil {
		return nil, fmt.Errorf("%w: nil client message", ErrInvalidMessage)
	}
	switch body := m.GetBody().(type) {
	case *pb.ControlClientMessage_Hello:
		return FromProtoHello(body.Hello)
	case *pb.ControlClientMessage_Subscribe:
		return FromProtoSubscribe(body.Subscribe)
	case *pb.ControlClientMessage_StreamAcknowledgement:
		return FromProtoStreamAcknowledgement(body.StreamAcknowledgement)
	case *pb.ControlClientMessage_ReplayRequest:
		return FromProtoReplayRequest(body.ReplayRequest)
	case *pb.ControlClientMessage_InvalidateConfirm:
		return FromProtoInvalidateConfirm(body.InvalidateConfirm)
	default:
		return nil, fmt.Errorf("%w: empty client body", ErrInvalidMessage)
	}
}

// FromProtoServerMessage unpacks a server stream envelope into a domain value.
func FromProtoServerMessage(m *pb.ControlServerMessage) (any, error) {
	if m == nil {
		return nil, fmt.Errorf("%w: nil server message", ErrInvalidMessage)
	}
	switch body := m.GetBody().(type) {
	case *pb.ControlServerMessage_InvalidationBatch:
		return FromProtoInvalidationBatch(body.InvalidationBatch)
	case *pb.ControlServerMessage_StreamCheckpoint:
		return FromProtoStreamCheckpoint(body.StreamCheckpoint)
	case *pb.ControlServerMessage_ReplayUnavailable:
		return FromProtoReplayUnavailable(body.ReplayUnavailable)
	case *pb.ControlServerMessage_ControlError:
		return FromProtoControlError(body.ControlError)
	default:
		return nil, fmt.Errorf("%w: empty server body", ErrInvalidMessage)
	}
}

func mutationIDFromProto(m *pb.MutationID) (wire.MutationID, error) {
	var id wire.MutationID
	if m == nil {
		return id, wire.ErrMutationIDRequired
	}
	raw := m.GetId()
	if len(raw) == 0 {
		return id, wire.ErrMutationIDRequired
	}
	if len(raw) != len(id) {
		return id, fmt.Errorf("mutation id must be %d bytes, got %d", len(id), len(raw))
	}
	copy(id[:], raw)
	return id, nil
}

// recordKindFromProto maps a protobuf record kind without truncating unknown
// values into a valid domain zero value.
func recordKindFromProto(k pb.RecordKind) (wire.RecordKind, error) {
	switch k {
	case pb.RecordKind_RECORD_KIND_VALUE:
		return wire.RecordValue, nil
	case pb.RecordKind_RECORD_KIND_TOMBSTONE:
		return wire.RecordTombstone, nil
	default:
		return 0, fmt.Errorf("%w: record kind %v", ErrInvalidRecordKind, k)
	}
}

func statusFromProto(s pb.Status) (wire.Status, error) {
	switch s {
	case pb.Status_STATUS_OK:
		return wire.StatusOK, nil
	case pb.Status_STATUS_NOT_FOUND:
		return wire.StatusNotFound, nil
	case pb.Status_STATUS_NOT_CAUGHT_UP:
		return wire.StatusNotCaughtUp, nil
	case pb.Status_STATUS_ERR_DURABILITY_UNAVAILABLE:
		return wire.StatusErrDurabilityUnavailable, nil
	case pb.Status_STATUS_ERR_BAD_GENERATION:
		return wire.StatusErrBadGeneration, nil
	case pb.Status_STATUS_ERR_RATE_LIMITED:
		return wire.StatusErrRateLimited, nil
	case pb.Status_STATUS_ERR_INVALID_ARGUMENT:
		return wire.StatusErrInvalidArgument, nil
	case pb.Status_STATUS_ERR_WRITE_CONFIRM_TIMEOUT:
		return wire.StatusErrWriteConfirmTimeout, nil
	case pb.Status_STATUS_ERR_INTERNAL:
		return wire.StatusErrInternal, nil
	default:
		return 0, fmt.Errorf("%w: %v", ErrInvalidControlStatus, s)
	}
}

// protocolVersionFromUint32 rejects values that would wrap when narrowed to
// ProtocolVersion (uint16), so fail-closed negotiation cannot be bypassed.
func protocolVersionFromUint32(v uint32) (wire.ProtocolVersion, error) {
	if v > math.MaxUint16 {
		return 0, fmt.Errorf("%w: protocol version %d exceeds uint16", wire.ErrInvalidProtocolRange, v)
	}
	return wire.ProtocolVersion(v), nil
}

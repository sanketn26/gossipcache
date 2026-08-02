package rpc

import (
	"fmt"
	"math"

	pb "github.com/sanketn26/gossipcache/api/gen/gossipcache/v1"
	"github.com/sanketn26/gossipcache/internal/wire"
)

// --- wire domain → protobuf ---

// ProtoHandshakeRequest converts a domain handshake request to protobuf.
func ProtoHandshakeRequest(m HandshakeRequest) *pb.HandshakeRequest {
	return &pb.HandshakeRequest{
		ProtocolVersion:       uint32(m.Protocol.Version),
		MinSupportedVersion:   uint32(m.Protocol.MinSupported),
		ClusterId:             m.ClusterID,
		ExpectedHubGeneration: m.ExpectedHubGeneration,
	}
}

// ProtoHandshake converts a domain handshake response to protobuf.
func ProtoHandshake(m Handshake) *pb.HandshakeResponse {
	return &pb.HandshakeResponse{
		ProtocolVersion: uint32(m.ProtocolVersion),
		HubGeneration:   m.HubGeneration,
		PartitionCount:  m.PartitionCount,
		StorageProfile:  ProtoStorageProfile(m.StorageProfile),
		DurableHealthy:  m.DurableHealthy,
		ClusterId:       m.ClusterID,
	}
}

// ProtoHubStatusRequest converts a domain hub status request to protobuf.
func ProtoHubStatusRequest(m HubStatusRequest) *pb.HubStatusRequest {
	return &pb.HubStatusRequest{HubGeneration: m.HubGeneration}
}

// ProtoHubStatus converts a domain hub status to protobuf.
func ProtoHubStatus(m HubStatus) *pb.HubStatusResponse {
	return &pb.HubStatusResponse{
		HubGeneration:  m.HubGeneration,
		StorageProfile: ProtoStorageProfile(m.StorageProfile),
		DurableHealthy: m.DurableHealthy,
		Status:         ProtoStatus(m.Status),
	}
}

// ProtoGetRequest converts a domain get request to protobuf.
func ProtoGetRequest(m GetRequest) *pb.GetRequest {
	out := &pb.GetRequest{
		Key:           wire.CopyBytes(m.Key),
		HubGeneration: m.HubGeneration,
	}
	if m.MinVersion != nil {
		out.MinVersion = ProtoVersionTag(*m.MinVersion)
	}
	return out
}

// ProtoGetResponse converts a domain get response to protobuf.
func ProtoGetResponse(m GetResponse) *pb.GetResponse {
	return &pb.GetResponse{
		Status:        ProtoStatus(m.Status),
		HubGeneration: m.HubGeneration,
		Version:       ProtoVersionTag(m.Version),
		Value:         wire.CopyBytes(m.Value),
		TtlMillis:     m.TTLMillis,
		Kind:          ProtoRecordKind(m.Kind),
	}
}

// ProtoMutationRequest converts a domain mutation request to protobuf.
func ProtoMutationRequest(m MutationRequest) *pb.MutationRequest {
	return &pb.MutationRequest{
		Op:            ProtoMutationOp(m.Op),
		Key:           wire.CopyBytes(m.Key),
		Value:         wire.CopyBytes(m.Value),
		TtlMillis:     m.TTLMillis,
		MutationId:    ProtoMutationID(m.MutationID),
		Mode:          ProtoWriteMode(m.Mode),
		W:             uint32(m.W),
		Confirm:       ProtoConfirmLevel(m.Confirm),
		TimeoutMillis: m.Timeout,
		HubGeneration: m.HubGeneration,
	}
}

// ProtoMutationResponse converts a domain mutation response to protobuf.
func ProtoMutationResponse(m MutationResponse) *pb.MutationResponse {
	return &pb.MutationResponse{
		Status:        ProtoStatus(m.Status),
		HubGeneration: m.HubGeneration,
		Version:       ProtoVersionTag(m.Version),
	}
}

// ProtoVersionTag converts a domain version tag to protobuf.
func ProtoVersionTag(v wire.VersionTag) *pb.VersionTag {
	return &pb.VersionTag{
		PartitionId: v.PartitionID,
		Sequence:    v.Sequence,
	}
}

// ProtoMutationID converts a domain mutation id to protobuf.
func ProtoMutationID(id wire.MutationID) *pb.MutationID {
	return &pb.MutationID{Id: id[:]}
}

// ProtoStatus maps a domain status to protobuf.
func ProtoStatus(s wire.Status) pb.Status {
	return pb.Status(s)
}

// ProtoRecordKind maps a domain record kind to protobuf.
func ProtoRecordKind(k wire.RecordKind) pb.RecordKind {
	return pb.RecordKind(k)
}

// ProtoWriteMode maps a domain write mode to protobuf.
func ProtoWriteMode(m wire.WriteMode) pb.WriteMode {
	return pb.WriteMode(m)
}

// ProtoStorageProfile maps a domain storage profile to protobuf.
func ProtoStorageProfile(p wire.StorageProfile) pb.StorageProfile {
	return pb.StorageProfile(p)
}

// ProtoConfirmLevel maps a domain confirm level to protobuf.
func ProtoConfirmLevel(c wire.ConfirmLevel) pb.ConfirmLevel {
	return pb.ConfirmLevel(c)
}

// ProtoMutationOp maps a domain mutation op to protobuf.
func ProtoMutationOp(op Op) pb.MutationOp {
	switch op {
	case OpSet:
		return pb.MutationOp_MUTATION_OP_SET
	case OpDelete:
		return pb.MutationOp_MUTATION_OP_DELETE
	default:
		return pb.MutationOp_MUTATION_OP_UNSPECIFIED
	}
}

// --- protobuf → wire domain ---

// FromProtoHandshakeRequest converts a protobuf handshake request.
func FromProtoHandshakeRequest(m *pb.HandshakeRequest) (HandshakeRequest, error) {
	if m == nil {
		return HandshakeRequest{}, fmt.Errorf("nil handshake request")
	}
	version, err := protocolVersionFromUint32(m.GetProtocolVersion())
	if err != nil {
		return HandshakeRequest{}, err
	}
	minimum, err := protocolVersionFromUint32(m.GetMinSupportedVersion())
	if err != nil {
		return HandshakeRequest{}, err
	}
	out := HandshakeRequest{
		Protocol: wire.ProtocolRange{
			Version:      version,
			MinSupported: minimum,
		},
		ClusterID:             m.GetClusterId(),
		ExpectedHubGeneration: m.GetExpectedHubGeneration(),
	}
	return out, out.Validate()
}

// FromProtoHandshake converts a protobuf handshake response.
func FromProtoHandshake(m *pb.HandshakeResponse) (Handshake, error) {
	if m == nil {
		return Handshake{}, fmt.Errorf("nil handshake response")
	}
	version, err := protocolVersionFromUint32(m.GetProtocolVersion())
	if err != nil {
		return Handshake{}, err
	}
	profile, err := FromProtoStorageProfile(m.GetStorageProfile())
	if err != nil {
		return Handshake{}, err
	}
	out := Handshake{
		ProtocolVersion: version,
		HubGeneration:   m.GetHubGeneration(),
		PartitionCount:  m.GetPartitionCount(),
		StorageProfile:  profile,
		DurableHealthy:  m.GetDurableHealthy(),
		ClusterID:       m.GetClusterId(),
	}
	return out, out.Validate()
}

// FromProtoHubStatusRequest converts a protobuf hub status request.
func FromProtoHubStatusRequest(m *pb.HubStatusRequest) (HubStatusRequest, error) {
	if m == nil {
		return HubStatusRequest{}, fmt.Errorf("nil hub status request")
	}
	out := HubStatusRequest{HubGeneration: m.GetHubGeneration()}
	return out, out.Validate()
}

// FromProtoHubStatus converts a protobuf hub status response.
func FromProtoHubStatus(m *pb.HubStatusResponse) (HubStatus, error) {
	if m == nil {
		return HubStatus{}, fmt.Errorf("nil hub status")
	}
	status, err := FromProtoStatus(m.GetStatus())
	if err != nil {
		return HubStatus{}, err
	}
	profile, err := FromProtoStorageProfile(m.GetStorageProfile())
	if err != nil {
		return HubStatus{}, err
	}
	out := HubStatus{
		HubGeneration:  m.GetHubGeneration(),
		StorageProfile: profile,
		DurableHealthy: m.GetDurableHealthy(),
		Status:         status,
	}
	return out, out.Validate()
}

// FromProtoGetRequest converts a protobuf get request.
func FromProtoGetRequest(m *pb.GetRequest) (GetRequest, error) {
	if m == nil {
		return GetRequest{}, fmt.Errorf("nil get request")
	}
	out := GetRequest{
		Key:           wire.CopyBytes(m.GetKey()),
		HubGeneration: m.GetHubGeneration(),
	}
	if m.MinVersion != nil {
		v := FromProtoVersionTag(m.GetMinVersion())
		out.MinVersion = &v
	}
	return out, out.Validate()
}

// FromProtoGetResponse converts a protobuf get response.
func FromProtoGetResponse(m *pb.GetResponse) (GetResponse, error) {
	if m == nil {
		return GetResponse{}, fmt.Errorf("nil get response")
	}
	status, err := FromProtoStatus(m.GetStatus())
	if err != nil {
		return GetResponse{}, err
	}
	kind, err := FromProtoRecordKind(m.GetKind())
	if err != nil {
		return GetResponse{}, err
	}
	out := GetResponse{
		Status:        status,
		HubGeneration: m.GetHubGeneration(),
		Version:       FromProtoVersionTag(m.GetVersion()),
		Value:         wire.CopyBytes(m.GetValue()),
		TTLMillis:     m.GetTtlMillis(),
		Kind:          kind,
	}
	return out, out.Validate()
}

// FromProtoMutationRequest converts a protobuf mutation request.
func FromProtoMutationRequest(m *pb.MutationRequest) (MutationRequest, error) {
	if m == nil {
		return MutationRequest{}, fmt.Errorf("nil mutation request")
	}
	id, err := FromProtoMutationID(m.GetMutationId())
	if err != nil {
		return MutationRequest{}, err
	}
	if m.GetW() > math.MaxUint16 {
		return MutationRequest{}, fmt.Errorf("%w: w out of range", wire.ErrInvalidWriteTimeout)
	}
	op, err := FromProtoMutationOp(m.GetOp())
	if err != nil {
		return MutationRequest{}, err
	}
	mode, err := FromProtoWriteMode(m.GetMode())
	if err != nil {
		return MutationRequest{}, err
	}
	confirm, err := FromProtoConfirmLevel(m.GetConfirm())
	if err != nil {
		return MutationRequest{}, err
	}
	out := MutationRequest{
		Op:            op,
		Key:           wire.CopyBytes(m.GetKey()),
		Value:         wire.CopyBytes(m.GetValue()),
		TTLMillis:     m.GetTtlMillis(),
		MutationID:    id,
		Mode:          mode,
		W:             uint16(m.GetW()),
		Confirm:       confirm,
		Timeout:       m.GetTimeoutMillis(),
		HubGeneration: m.GetHubGeneration(),
	}
	return out, out.Validate()
}

// FromProtoMutationResponse converts a protobuf mutation response.
func FromProtoMutationResponse(m *pb.MutationResponse) (MutationResponse, error) {
	if m == nil {
		return MutationResponse{}, fmt.Errorf("nil mutation response")
	}
	status, err := FromProtoStatus(m.GetStatus())
	if err != nil {
		return MutationResponse{}, err
	}
	out := MutationResponse{
		Status:        status,
		HubGeneration: m.GetHubGeneration(),
		Version:       FromProtoVersionTag(m.GetVersion()),
	}
	return out, out.Validate()
}

// FromProtoVersionTag converts a protobuf version tag.
func FromProtoVersionTag(v *pb.VersionTag) wire.VersionTag {
	if v == nil {
		return wire.VersionTag{}
	}
	return wire.VersionTag{
		PartitionID: v.GetPartitionId(),
		Sequence:    v.GetSequence(),
	}
}

// FromProtoMutationID converts a protobuf mutation id.
func FromProtoMutationID(m *pb.MutationID) (wire.MutationID, error) {
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

// FromProtoStatus maps a protobuf status to domain. Unknown values fail closed
// without truncating into a valid Status.
func FromProtoStatus(s pb.Status) (wire.Status, error) {
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
		return 0, fmt.Errorf("%w: %v", ErrInvalidStatus, s)
	}
}

// FromProtoRecordKind maps a protobuf record kind to domain.
func FromProtoRecordKind(k pb.RecordKind) (wire.RecordKind, error) {
	switch k {
	case pb.RecordKind_RECORD_KIND_VALUE:
		return wire.RecordValue, nil
	case pb.RecordKind_RECORD_KIND_TOMBSTONE:
		return wire.RecordTombstone, nil
	default:
		return 0, fmt.Errorf("%w: record kind %v", ErrInvalidEnum, k)
	}
}

// FromProtoWriteMode maps a protobuf write mode to domain.
func FromProtoWriteMode(m pb.WriteMode) (wire.WriteMode, error) {
	switch m {
	case pb.WriteMode_WRITE_MODE_FAST:
		return wire.WriteFast, nil
	case pb.WriteMode_WRITE_MODE_SYNC:
		return wire.WriteSync, nil
	default:
		return 0, fmt.Errorf("%w: write mode %v", ErrInvalidEnum, m)
	}
}

// FromProtoStorageProfile maps a protobuf storage profile to domain.
func FromProtoStorageProfile(p pb.StorageProfile) (wire.StorageProfile, error) {
	switch p {
	case pb.StorageProfile_STORAGE_PROFILE_MEMORY:
		return wire.StorageMemory, nil
	case pb.StorageProfile_STORAGE_PROFILE_DURABLE:
		return wire.StorageDurable, nil
	default:
		return 0, fmt.Errorf("%w: storage profile %v", ErrInvalidEnum, p)
	}
}

// FromProtoConfirmLevel maps a protobuf confirm level to domain.
func FromProtoConfirmLevel(c pb.ConfirmLevel) (wire.ConfirmLevel, error) {
	switch c {
	case pb.ConfirmLevel_CONFIRM_LEVEL_INVALIDATE_APPLIED:
		return wire.ConfirmInvalidateApplied, nil
	default:
		return 0, fmt.Errorf("%w: confirm level %v", ErrInvalidEnum, c)
	}
}

// FromProtoMutationOp maps a protobuf mutation op to domain.
func FromProtoMutationOp(op pb.MutationOp) (Op, error) {
	switch op {
	case pb.MutationOp_MUTATION_OP_SET:
		return OpSet, nil
	case pb.MutationOp_MUTATION_OP_DELETE:
		return OpDelete, nil
	default:
		return 0, fmt.Errorf("%w: %v", ErrInvalidOp, op)
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

package rpc_test

import (
	"errors"
	"math"
	"testing"

	pb "github.com/sanketn26/gossipcache/api/gen/gossipcache/v1"
	"github.com/sanketn26/gossipcache/internal/rpc"
	"github.com/sanketn26/gossipcache/internal/wire"
)

func TestGetRoundTrip(t *testing.T) {
	t.Parallel()
	min := wire.VersionTag{PartitionID: 3, Sequence: 9}
	req := rpc.GetRequest{Key: []byte("user:1"), MinVersion: &min, HubGeneration: 7}
	back, err := rpc.FromProtoGetRequest(rpc.ProtoGetRequest(req))
	if err != nil {
		t.Fatal(err)
	}
	if string(back.Key) != "user:1" || back.MinVersion == nil || *back.MinVersion != min || back.HubGeneration != 7 {
		t.Fatalf("get request round-trip mismatch: %+v", back)
	}

	resp := rpc.GetResponse{
		Status:        wire.StatusOK,
		HubGeneration: 7,
		Version:       wire.VersionTag{PartitionID: 3, Sequence: 10},
		Value:         []byte("v"),
		TTLMillis:     1000,
		Kind:          wire.RecordValue,
	}
	got, err := rpc.FromProtoGetResponse(rpc.ProtoGetResponse(resp))
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != resp.Status || got.HubGeneration != resp.HubGeneration ||
		got.Version != resp.Version || string(got.Value) != "v" || got.TTLMillis != 1000 {
		t.Fatalf("get response round-trip mismatch: %+v", got)
	}
}

func TestMutationRoundTripAndFingerprint(t *testing.T) {
	t.Parallel()
	req := rpc.MutationRequest{
		Op:            rpc.OpSet,
		Key:           []byte("k"),
		Value:         []byte("v"),
		TTLMillis:     50,
		MutationID:    wire.NewMutationID(1, 2),
		Mode:          wire.WriteFast,
		W:             1,
		Confirm:       wire.ConfirmInvalidateApplied,
		Timeout:       250,
		HubGeneration: 9,
	}
	back, err := rpc.FromProtoMutationRequest(rpc.ProtoMutationRequest(req))
	if err != nil {
		t.Fatal(err)
	}
	if back.Op != req.Op || string(back.Key) != "k" || string(back.Value) != "v" ||
		back.MutationID != req.MutationID || back.W != 1 || back.Timeout != 250 || back.HubGeneration != 9 {
		t.Fatalf("mutation round-trip mismatch: %+v", back)
	}

	fp := rpc.Fingerprint(req)
	if err := fp.CheckMatch(rpc.Fingerprint(back)); err != nil {
		t.Fatal(err)
	}
	mismatched := back
	mismatched.Value = []byte("other")
	if err := fp.CheckMatch(rpc.Fingerprint(mismatched)); !errors.Is(err, rpc.ErrMutationFingerprintMismatch) {
		t.Fatalf("expected fingerprint mismatch, got %v", err)
	}

	ok := rpc.MutationResponse{
		Status:        wire.StatusOK,
		HubGeneration: 2,
		Version:       wire.VersionTag{PartitionID: 1, Sequence: 3},
	}
	got, err := rpc.FromProtoMutationResponse(rpc.ProtoMutationResponse(ok))
	if err != nil {
		t.Fatal(err)
	}
	if got != ok {
		t.Fatalf("mutation response round-trip mismatch: %+v", got)
	}
}

func TestHandshakeAndStatusValidation(t *testing.T) {
	t.Parallel()
	req := rpc.HandshakeRequest{
		Protocol:              wire.CurrentProtocolRange(),
		ClusterID:             "prod-a",
		ExpectedHubGeneration: 0,
	}
	reqBack, err := rpc.FromProtoHandshakeRequest(rpc.ProtoHandshakeRequest(req))
	if err != nil {
		t.Fatal(err)
	}
	if reqBack.ClusterID != "prod-a" || reqBack.ExpectedHubGeneration != 0 {
		t.Fatalf("handshake request mismatch: %+v", reqBack)
	}

	hs := rpc.Handshake{
		ProtocolVersion: wire.CurrentProtocolVersion,
		HubGeneration:   1,
		PartitionCount:  16,
		StorageProfile:  wire.StorageMemory,
		ClusterID:       "prod-a",
	}
	back, err := rpc.FromProtoHandshake(rpc.ProtoHandshake(hs))
	if err != nil {
		t.Fatal(err)
	}
	if back != hs {
		t.Fatalf("handshake mismatch: %+v", back)
	}

	bad := hs
	bad.DurableHealthy = true
	if err := bad.Validate(); !errors.Is(err, rpc.ErrInvalidDurableHealth) {
		t.Fatalf("expected durable health error, got %v", err)
	}

	status := rpc.HubStatus{
		HubGeneration:  1,
		StorageProfile: wire.StorageMemory,
		Status:         wire.StatusOK,
	}
	statusBack, err := rpc.FromProtoHubStatus(rpc.ProtoHubStatus(status))
	if err != nil {
		t.Fatal(err)
	}
	if statusBack != status {
		t.Fatalf("hub status mismatch: %+v", statusBack)
	}
}

func TestStatusHelpers(t *testing.T) {
	t.Parallel()
	if !rpc.StatusCarriesCommittedVersion(wire.StatusOK) {
		t.Fatal("OK should carry committed version")
	}
	if !rpc.StatusCarriesCommittedVersion(wire.StatusErrWriteConfirmTimeout) {
		t.Fatal("write confirm timeout should carry committed version")
	}
	if rpc.StatusCarriesGetRecord(wire.StatusNotFound) {
		t.Fatal("not found must not carry record")
	}
	if !rpc.RetrySameMutationID(wire.StatusNotCaughtUp) {
		t.Fatal("not caught up is retryable")
	}
}

func TestFromProtoEnumRejectsUnknownWithoutNarrowing(t *testing.T) {
	t.Parallel()

	// Status 65536 would wrap to StatusOK (0) under a bare uint16 cast.
	if _, err := rpc.FromProtoStatus(pb.Status(65536)); !errors.Is(err, rpc.ErrInvalidStatus) {
		t.Fatalf("status 65536: got %v, want ErrInvalidStatus", err)
	}
	// Kind/mode 256 would wrap to the valid zero value under a bare uint8 cast.
	if _, err := rpc.FromProtoRecordKind(pb.RecordKind(256)); !errors.Is(err, rpc.ErrInvalidEnum) {
		t.Fatalf("record kind 256: got %v, want ErrInvalidEnum", err)
	}
	if _, err := rpc.FromProtoWriteMode(pb.WriteMode(256)); !errors.Is(err, rpc.ErrInvalidEnum) {
		t.Fatalf("write mode 256: got %v, want ErrInvalidEnum", err)
	}
	if _, err := rpc.FromProtoStorageProfile(pb.StorageProfile(99)); !errors.Is(err, rpc.ErrInvalidEnum) {
		t.Fatalf("storage profile 99: got %v, want ErrInvalidEnum", err)
	}
	if _, err := rpc.FromProtoConfirmLevel(pb.ConfirmLevel(7)); !errors.Is(err, rpc.ErrInvalidEnum) {
		t.Fatalf("confirm level 7: got %v, want ErrInvalidEnum", err)
	}
}

func TestFromProtoProtocolVersionRejectsUint16Overflow(t *testing.T) {
	t.Parallel()

	// 65537 would wrap to version 1 under a bare uint16 cast and pass validation.
	_, err := rpc.FromProtoHandshakeRequest(&pb.HandshakeRequest{
		ProtocolVersion:     math.MaxUint16 + 1,
		MinSupportedVersion: 1,
		ClusterId:           "prod-a",
	})
	if !errors.Is(err, wire.ErrInvalidProtocolRange) {
		t.Fatalf("handshake request: got %v, want ErrInvalidProtocolRange", err)
	}

	_, err = rpc.FromProtoHandshake(&pb.HandshakeResponse{
		ProtocolVersion: math.MaxUint16 + 1,
		HubGeneration:   1,
		PartitionCount:  16,
		StorageProfile:  pb.StorageProfile_STORAGE_PROFILE_MEMORY,
		ClusterId:       "prod-a",
	})
	if !errors.Is(err, wire.ErrInvalidProtocolRange) {
		t.Fatalf("handshake response: got %v, want ErrInvalidProtocolRange", err)
	}
}

func TestFromProtoGetResponseRejectsUnknownStatus(t *testing.T) {
	t.Parallel()
	_, err := rpc.FromProtoGetResponse(&pb.GetResponse{
		Status:        pb.Status(65536),
		HubGeneration: 1,
	})
	if !errors.Is(err, rpc.ErrInvalidStatus) {
		t.Fatalf("got %v, want ErrInvalidStatus", err)
	}
}

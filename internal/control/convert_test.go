package control_test

import (
	"errors"
	"math"
	"testing"

	pb "github.com/sanketn26/gossipcache/api/gen/gossipcache/v1"
	"github.com/sanketn26/gossipcache/internal/control"
	"github.com/sanketn26/gossipcache/internal/wire"
)

func TestInvalidationBatchRoundTrip(t *testing.T) {
	t.Parallel()
	batch := control.InvalidationBatch{
		StreamID:      3,
		HubGeneration: 9,
		Events: []control.InvalidationEvent{
			{
				StreamSequence: 10,
				Key:            []byte("a"),
				Version:        wire.VersionTag{PartitionID: 3, Sequence: 100},
				Kind:           wire.RecordValue,
				MutationID:     wire.NewMutationID(1, 1),
			},
			{
				StreamSequence: 11,
				Key:            []byte("b"),
				Version:        wire.VersionTag{PartitionID: 3, Sequence: 101},
				Kind:           wire.RecordTombstone,
				MutationID:     wire.NewMutationID(1, 2),
			},
		},
	}
	msg, err := control.ProtoServerMessage(batch)
	if err != nil {
		t.Fatal(err)
	}
	body, err := control.FromProtoServerMessage(msg)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := body.(control.InvalidationBatch)
	if !ok {
		t.Fatalf("unexpected type %T", body)
	}
	if got.StreamID != batch.StreamID || got.HubGeneration != batch.HubGeneration || len(got.Events) != 2 {
		t.Fatalf("batch mismatch: %+v", got)
	}
	if string(got.Events[0].Key) != "a" || got.Events[1].Kind != wire.RecordTombstone {
		t.Fatalf("events mismatch: %+v", got.Events)
	}
}

func TestClientEnvelopeRoundTrip(t *testing.T) {
	t.Parallel()
	hello := control.Hello{
		NodeID: 42,
		Protocol: wire.ProtocolRange{
			Version:      wire.CurrentProtocolVersion,
			MinSupported: wire.MinSupportedProtocolVersion,
		},
		Subscriptions: []control.StreamWatermark{{
			StreamID: 1, AppliedThrough: 8, HubGeneration: 2,
		}},
		ClusterID:             "prod-a",
		ExpectedHubGeneration: 2,
	}
	msg, err := control.ProtoClientMessage(hello)
	if err != nil {
		t.Fatal(err)
	}
	body, err := control.FromProtoClientMessage(msg)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := body.(control.Hello)
	if !ok {
		t.Fatalf("unexpected type %T", body)
	}
	if got.NodeID != 42 || len(got.Subscriptions) != 1 || got.Subscriptions[0].AppliedThrough != 8 ||
		got.ClusterID != "prod-a" || got.ExpectedHubGeneration != 2 {
		t.Fatalf("hello mismatch: %+v", got)
	}
}

func TestBatchRejectsNonContiguous(t *testing.T) {
	t.Parallel()
	batch := control.InvalidationBatch{
		StreamID:      1,
		HubGeneration: 1,
		Events: []control.InvalidationEvent{
			{
				StreamSequence: 1,
				Key:            []byte("a"),
				Version:        wire.VersionTag{PartitionID: 1, Sequence: 1},
				Kind:           wire.RecordValue,
				MutationID:     wire.NewMutationID(1, 1),
			},
			{
				StreamSequence: 3,
				Key:            []byte("b"),
				Version:        wire.VersionTag{PartitionID: 1, Sequence: 2},
				Kind:           wire.RecordValue,
				MutationID:     wire.NewMutationID(1, 2),
			},
		},
	}
	if err := batch.Validate(); !errors.Is(err, control.ErrNonContiguousBatch) {
		t.Fatalf("expected non-contiguous error, got %v", err)
	}
}

func TestReplayUnavailableValidation(t *testing.T) {
	t.Parallel()
	ok := control.ReplayUnavailable{
		StreamID: 1, HubGeneration: 1, RequestedFrom: 2,
		OldestAvailable: 10, StreamHead: 20,
	}
	back, err := control.FromProtoReplayUnavailable(control.ProtoReplayUnavailable(ok))
	if err != nil {
		t.Fatal(err)
	}
	if back != ok {
		t.Fatalf("replay unavailable mismatch: %+v", back)
	}
}

func TestProtoClientMessageTypedNil(t *testing.T) {
	t.Parallel()
	cases := []any{
		(*control.Hello)(nil),
		(*control.Subscribe)(nil),
		(*control.StreamAcknowledgement)(nil),
		(*control.ReplayRequest)(nil),
		(*control.InvalidateConfirm)(nil),
	}
	for _, body := range cases {
		_, err := control.ProtoClientMessage(body)
		if !errors.Is(err, control.ErrInvalidMessage) {
			t.Fatalf("%T: got %v, want ErrInvalidMessage", body, err)
		}
	}
}

func TestProtoServerMessageTypedNil(t *testing.T) {
	t.Parallel()
	cases := []any{
		(*control.InvalidationBatch)(nil),
		(*control.StreamCheckpoint)(nil),
		(*control.ReplayUnavailable)(nil),
		(*control.ControlError)(nil),
	}
	for _, body := range cases {
		_, err := control.ProtoServerMessage(body)
		if !errors.Is(err, control.ErrInvalidMessage) {
			t.Fatalf("%T: got %v, want ErrInvalidMessage", body, err)
		}
	}
}

func TestFromProtoHelloRejectsUint16Overflow(t *testing.T) {
	t.Parallel()
	_, err := control.FromProtoHello(&pb.Hello{
		NodeId:              1,
		ProtocolVersion:     math.MaxUint16 + 1,
		MinSupportedVersion: 1,
		ClusterId:           "prod-a",
	})
	if !errors.Is(err, wire.ErrInvalidProtocolRange) {
		t.Fatalf("got %v, want ErrInvalidProtocolRange", err)
	}
}

func TestBatchRejectsOversizedEncodedEstimate(t *testing.T) {
	t.Parallel()
	// One max-size key repeated enough times exceeds MaxControlMessageBytes
	// while staying under MaxBatchEvents.
	const events = 800
	batch := control.InvalidationBatch{
		StreamID:      1,
		HubGeneration: 1,
		Events:        make([]control.InvalidationEvent, events),
	}
	key := make([]byte, wire.MaxKeyLen)
	for i := range key {
		key[i] = 'a'
	}
	for i := 0; i < events; i++ {
		batch.Events[i] = control.InvalidationEvent{
			StreamSequence: uint64(i + 1),
			Key:            key,
			Version:        wire.VersionTag{PartitionID: 1, Sequence: uint64(i + 1)},
			Kind:           wire.RecordValue,
			MutationID:     wire.NewMutationID(1, uint64(i+1)),
		}
	}
	if batch.EstimatedEncodedSize() <= control.MaxControlMessageBytes {
		t.Fatalf("test fixture too small: estimated %d", batch.EstimatedEncodedSize())
	}
	if err := batch.Validate(); !errors.Is(err, control.ErrBatchTooLarge) {
		t.Fatalf("got %v, want ErrBatchTooLarge", err)
	}
}

func TestControlErrorRoundTrip(t *testing.T) {
	t.Parallel()
	ce := control.ControlError{Status: wire.StatusErrRateLimited, Detail: "subscribe budget"}
	msg, err := control.ProtoServerMessage(ce)
	if err != nil {
		t.Fatal(err)
	}
	body, err := control.FromProtoServerMessage(msg)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := body.(control.ControlError)
	if !ok || got != ce {
		t.Fatalf("control error mismatch: %+v", body)
	}
}

func TestFromProtoInvalidationBatchRejectsUnknownKind(t *testing.T) {
	t.Parallel()
	// Kind 256 would wrap to RECORD_KIND_VALUE under a bare uint8 cast.
	id := wire.NewMutationID(1, 1)
	_, err := control.FromProtoInvalidationBatch(&pb.InvalidationBatch{
		StreamId:      1,
		HubGeneration: 1,
		Events: []*pb.InvalidationEvent{{
			StreamSequence: 1,
			Key:            []byte("k"),
			Version:        &pb.VersionTag{PartitionId: 1, Sequence: 1},
			Kind:           pb.RecordKind(256),
			MutationId:     &pb.MutationID{Id: id[:]},
		}},
	})
	if !errors.Is(err, control.ErrInvalidRecordKind) {
		t.Fatalf("got %v, want ErrInvalidRecordKind", err)
	}
}

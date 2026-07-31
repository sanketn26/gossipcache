package rpc_test

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash/crc32"
	"io"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/sanketn26/gossipcache/internal/rpc"
	"github.com/sanketn26/gossipcache/internal/wire"
)

func goldenMessages() map[string]struct {
	CorrelationID uint32
	Message       rpc.Message
} {
	minVersion := wire.VersionTag{PartitionID: 3, Sequence: 21}
	return map[string]struct {
		CorrelationID uint32
		Message       rpc.Message
	}{
		"handshake_request": {
			CorrelationID: 1,
			Message: rpc.HandshakeRequest{
				Protocol: wire.CurrentProtocolRange(),
			},
		},
		"handshake": {
			CorrelationID: 1,
			Message: rpc.Handshake{
				ProtocolVersion: wire.CurrentProtocolVersion,
				HubGeneration:   2,
				PartitionCount:  16,
				StorageProfile:  wire.StorageMemory,
				DurableHealthy:  false,
			},
		},
		"handshake_durable": {
			CorrelationID: 2,
			Message: rpc.Handshake{
				ProtocolVersion: wire.CurrentProtocolVersion,
				HubGeneration:   9,
				PartitionCount:  64,
				StorageProfile:  wire.StorageDurable,
				DurableHealthy:  true,
			},
		},
		"hub_status": {
			CorrelationID: 3,
			Message: rpc.HubStatus{
				HubGeneration:  2,
				StorageProfile: wire.StorageDurable,
				DurableHealthy: false,
			},
		},
		"get_request": {
			CorrelationID: 10,
			Message: rpc.GetRequest{
				Key:        []byte("key"),
				MinVersion: &minVersion,
			},
		},
		"get_request_no_min": {
			CorrelationID: 11,
			Message: rpc.GetRequest{
				Key: []byte{0x00, 0xff},
			},
		},
		"get_response_value": {
			CorrelationID: 10,
			Message: rpc.GetResponse{
				Status:        wire.StatusOK,
				HubGeneration: 2,
				Version:       wire.VersionTag{PartitionID: 3, Sequence: 21},
				Value:         []byte("value"),
				TTLMillis:     1500,
				Kind:          wire.RecordValue,
			},
		},
		"get_response_tombstone": {
			CorrelationID: 12,
			Message: rpc.GetResponse{
				Status:        wire.StatusOK,
				HubGeneration: 2,
				Version:       wire.VersionTag{PartitionID: 3, Sequence: 22},
				Kind:          wire.RecordTombstone,
			},
		},
		"get_response_not_found": {
			CorrelationID: 13,
			Message: rpc.GetResponse{
				Status:        wire.StatusNotFound,
				HubGeneration: 2,
			},
		},
		"get_response_not_caught_up": {
			CorrelationID: 14,
			Message: rpc.GetResponse{
				Status:        wire.StatusNotCaughtUp,
				HubGeneration: 2,
			},
		},
		"mutation_set": {
			CorrelationID: 20,
			Message: rpc.MutationRequest{
				Op:         rpc.OpSet,
				Key:        []byte("key"),
				Value:      []byte("value"),
				TTLMillis:  1500,
				MutationID: wire.NewMutationID(8, 13),
				Mode:       wire.WriteFast,
				W:          1,
				Confirm:    wire.ConfirmInvalidateApplied,
				Timeout:    250,
			},
		},
		"mutation_delete": {
			CorrelationID: 21,
			Message: rpc.MutationRequest{
				Op:         rpc.OpDelete,
				Key:        []byte("key"),
				MutationID: wire.NewMutationID(8, 14),
				Mode:       wire.WriteSync,
			},
		},
		"mutation_response_ok": {
			CorrelationID: 20,
			Message: rpc.MutationResponse{
				Status:        wire.StatusOK,
				HubGeneration: 2,
				Version:       wire.VersionTag{PartitionID: 3, Sequence: 21},
			},
		},
		"mutation_response_w_timeout": {
			CorrelationID: 22,
			Message: rpc.MutationResponse{
				Status:        wire.StatusErrWriteConfirmTimeout,
				HubGeneration: 2,
				Version:       wire.VersionTag{PartitionID: 3, Sequence: 21},
			},
		},
		"mutation_response_durability": {
			CorrelationID: 23,
			Message: rpc.MutationResponse{
				Status:        wire.StatusErrDurabilityUnavailable,
				HubGeneration: 2,
			},
		},
		"mutation_response_bad_generation": {
			CorrelationID: 24,
			Message: rpc.MutationResponse{
				Status:        wire.StatusErrBadGeneration,
				HubGeneration: 2,
			},
		},
		"mutation_response_invalid_argument": {
			CorrelationID: 25,
			Message: rpc.MutationResponse{
				Status:        wire.StatusErrInvalidArgument,
				HubGeneration: 2,
			},
		},
		"mutation_response_rate_limited": {
			CorrelationID: 26,
			Message: rpc.MutationResponse{
				Status:        wire.StatusErrRateLimited,
				HubGeneration: 2,
			},
		},
		"mutation_response_internal": {
			CorrelationID: 27,
			Message: rpc.MutationResponse{
				Status:        wire.StatusErrInternal,
				HubGeneration: 2,
			},
		},
	}
}

func TestGoldenFrames(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("testdata/rpc_vectors.json")
	if err != nil {
		t.Fatalf("read vectors: %v", err)
	}
	var vectors map[string]string
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatalf("decode vectors: %v", err)
	}

	messages := goldenMessages()
	if len(vectors) != len(messages) {
		t.Fatalf("vector count = %d, want %d", len(vectors), len(messages))
	}
	for name, entry := range messages {
		name, entry := name, entry
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			frame, err := rpc.MarshalFrame(entry.CorrelationID, entry.Message)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			got := hex.EncodeToString(frame)
			if got != vectors[name] {
				t.Errorf("frame hex:\n got %s\nwant %s", got, vectors[name])
			}
			correlationID, decoded, err := rpc.UnmarshalFrame(frame)
			if err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if correlationID != entry.CorrelationID {
				t.Fatalf("correlation id = %d, want %d", correlationID, entry.CorrelationID)
			}
			if !reflect.DeepEqual(decoded, entry.Message) {
				t.Fatalf("round trip:\n got %#v\nwant %#v", decoded, entry.Message)
			}
		})
	}
}

func TestMessageTypeValues(t *testing.T) {
	t.Parallel()

	messages := []rpc.Message{
		rpc.HandshakeRequest{},
		rpc.Handshake{},
		rpc.HubStatus{},
		rpc.GetRequest{},
		rpc.GetResponse{},
		rpc.MutationRequest{},
		rpc.MutationResponse{},
	}
	for i, message := range messages {
		want := rpc.MessageType(i + 1)
		if message.Type() != want {
			t.Errorf("%T type = %d, want %d", message, message.Type(), want)
		}
		if !want.Valid() {
			t.Errorf("defined message type %d is invalid", want)
		}
	}
	if rpc.MessageType(0).Valid() || rpc.MessageType(8).Valid() {
		t.Fatal("unknown message type reported valid")
	}
}

func TestProtocolDefaults(t *testing.T) {
	t.Parallel()

	if rpc.DefaultRPCPort != 7400 {
		t.Fatalf("default RPC port = %d, want 7400", rpc.DefaultRPCPort)
	}
	if rpc.DefaultDedupWindow != 5*time.Minute {
		t.Fatalf("dedup window = %s, want 5m", rpc.DefaultDedupWindow)
	}
	if rpc.HeaderSize != 20 {
		t.Fatalf("header size = %d, want 20", rpc.HeaderSize)
	}
	wantMax := wire.MaxValueLen + wire.MaxKeyLen + 41
	if rpc.MaxRPCPayload != wantMax {
		t.Fatalf("max payload = %d, want %d", rpc.MaxRPCPayload, wantMax)
	}
	// A max key + max value mutation must fit under the payload ceiling.
	maxMutation := wire.MaxValueLen + wire.MaxKeyLen + 41
	if maxMutation > rpc.MaxRPCPayload {
		t.Fatalf("max mutation size %d exceeds MaxRPCPayload %d", maxMutation, rpc.MaxRPCPayload)
	}
}

func TestOpValues(t *testing.T) {
	t.Parallel()

	if rpc.OpSet != 1 || rpc.OpDelete != 2 {
		t.Fatalf("op values: set=%d delete=%d", rpc.OpSet, rpc.OpDelete)
	}
	if !rpc.OpSet.Valid() || !rpc.OpDelete.Valid() || rpc.Op(0).Valid() || rpc.Op(3).Valid() {
		t.Fatal("op validity mismatch")
	}
}

func TestStatusClassificationContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status              wire.Status
		class               wire.StatusClass
		carriesCommitted    bool
		carriesGetRecord    bool
		retrySameMutationID bool
	}{
		{wire.StatusOK, wire.StatusClassSuccess, true, true, false},
		{wire.StatusNotFound, wire.StatusClassSuccess, false, false, false},
		{wire.StatusNotCaughtUp, wire.StatusClassRetryable, false, false, true},
		{wire.StatusErrDurabilityUnavailable, wire.StatusClassTerminal, false, false, false},
		{wire.StatusErrBadGeneration, wire.StatusClassTerminal, false, false, false},
		{wire.StatusErrRateLimited, wire.StatusClassRetryable, false, false, true},
		{wire.StatusErrInvalidArgument, wire.StatusClassTerminal, false, false, false},
		{wire.StatusErrWriteConfirmTimeout, wire.StatusClassSuccess, true, false, false},
		{wire.StatusErrInternal, wire.StatusClassTerminal, false, false, false},
	}
	for _, test := range tests {
		if got := test.status.Class(); got != test.class {
			t.Errorf("%s.Class() = %d, want %d", test.status, got, test.class)
		}
		if got := rpc.StatusCarriesCommittedVersion(test.status); got != test.carriesCommitted {
			t.Errorf("%s committed version = %t, want %t", test.status, got, test.carriesCommitted)
		}
		if got := rpc.StatusCarriesGetRecord(test.status); got != test.carriesGetRecord {
			t.Errorf("%s get record = %t, want %t", test.status, got, test.carriesGetRecord)
		}
		if got := rpc.RetrySameMutationID(test.status); got != test.retrySameMutationID {
			t.Errorf("%s retry same mutation = %t, want %t", test.status, got, test.retrySameMutationID)
		}
	}

}

func TestFrameHeaderAndStreamIO(t *testing.T) {
	t.Parallel()

	message := rpc.MutationResponse{
		Status:        wire.StatusOK,
		HubGeneration: 7,
		Version:       wire.VersionTag{PartitionID: 4, Sequence: 99},
	}
	const correlationID uint32 = 0x0a0b0c0d

	var stream shortWriter
	if err := rpc.WriteFrame(&stream, correlationID, message); err != nil {
		t.Fatalf("write: %v", err)
	}
	frame := stream.Bytes()
	// status(2) + generation(8) + version(4+8) = 22
	const payloadLen = 22
	if len(frame) != rpc.HeaderSize+payloadLen {
		t.Fatalf("frame length = %d, want %d", len(frame), rpc.HeaderSize+payloadLen)
	}
	if got := binary.BigEndian.Uint32(frame[0:4]); got != 0x47435231 {
		t.Fatalf("magic = %#x", got)
	}
	if got := binary.BigEndian.Uint16(frame[4:6]); got != uint16(wire.CurrentProtocolVersion) {
		t.Fatalf("version = %d", got)
	}
	if got := binary.BigEndian.Uint16(frame[6:8]); got != uint16(rpc.MessageMutationResponse) {
		t.Fatalf("message type = %d", got)
	}
	if got := binary.BigEndian.Uint32(frame[8:12]); got != correlationID {
		t.Fatalf("correlation id = %#x, want %#x", got, correlationID)
	}
	if got := binary.BigEndian.Uint32(frame[12:16]); got != payloadLen {
		t.Fatalf("payload length = %d", got)
	}

	gotID, decoded, err := rpc.ReadFrame(bytes.NewReader(frame))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if gotID != correlationID {
		t.Fatalf("read correlation = %d, want %d", gotID, correlationID)
	}
	if !reflect.DeepEqual(decoded, message) {
		t.Fatalf("decoded = %#v, want %#v", decoded, message)
	}
}

func TestMarshalFrameVersion(t *testing.T) {
	t.Parallel()

	message := rpc.HubStatus{
		HubGeneration:  7,
		StorageProfile: wire.StorageMemory,
	}
	frame, err := rpc.MarshalFrameVersion(9, message, wire.CurrentProtocolVersion)
	if err != nil {
		t.Fatalf("marshal current version: %v", err)
	}
	if got := wire.ProtocolVersion(binary.BigEndian.Uint16(frame[4:6])); got != wire.CurrentProtocolVersion {
		t.Fatalf("encoded version = %d, want %d", got, wire.CurrentProtocolVersion)
	}
	if _, err := rpc.MarshalFrameVersion(9, message, wire.CurrentProtocolVersion+1); !errors.Is(err, rpc.ErrUnsupportedVersion) {
		t.Fatalf("future version error = %v, want unsupported version", err)
	}
	if _, err := rpc.MarshalFrameVersion(9, message, 0); !errors.Is(err, rpc.ErrUnsupportedVersion) {
		t.Fatalf("zero version error = %v, want unsupported version", err)
	}

	correlationID, decoded, err := rpc.UnmarshalFrameVersion(frame, wire.CurrentProtocolVersion)
	if err != nil {
		t.Fatalf("unmarshal current version: %v", err)
	}
	if correlationID != 9 {
		t.Fatalf("correlation id = %d, want 9", correlationID)
	}
	if !reflect.DeepEqual(decoded, message) {
		t.Fatalf("decoded = %#v, want %#v", decoded, message)
	}

	var stream bytes.Buffer
	if err := rpc.WriteFrameVersion(&stream, 9, message, wire.CurrentProtocolVersion); err != nil {
		t.Fatalf("write current version: %v", err)
	}
	correlationID, decoded, err = rpc.ReadFrameVersion(&stream, wire.CurrentProtocolVersion)
	if err != nil {
		t.Fatalf("read current version: %v", err)
	}
	if correlationID != 9 || !reflect.DeepEqual(decoded, message) {
		t.Fatalf("stream decoded = %d %#v", correlationID, decoded)
	}
}

func TestFrameFailuresFailClosed(t *testing.T) {
	t.Parallel()

	valid, err := rpc.MarshalFrame(1, rpc.MutationResponse{
		Status:        wire.StatusOK,
		HubGeneration: 1,
		Version:       wire.VersionTag{PartitionID: 1, Sequence: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		frame func() []byte
		want  error
	}{
		{
			name:  "short header",
			frame: func() []byte { return append([]byte(nil), valid[:rpc.HeaderSize-1]...) },
			want:  rpc.ErrTruncatedFrame,
		},
		{
			name: "bad magic",
			frame: func() []byte {
				frame := append([]byte(nil), valid...)
				frame[0] ^= 0xff
				return frame
			},
			want: rpc.ErrBadMagic,
		},
		{
			name: "bad crc",
			frame: func() []byte {
				frame := append([]byte(nil), valid...)
				frame[16] ^= 0xff
				return frame
			},
			want: rpc.ErrBadHeaderCRC,
		},
		{
			name: "truncated payload",
			frame: func() []byte {
				return append([]byte(nil), valid[:len(valid)-1]...)
			},
			want: rpc.ErrTruncatedFrame,
		},
		{
			name: "trailing frame bytes",
			frame: func() []byte {
				return append(append([]byte(nil), valid...), 0)
			},
			want: rpc.ErrTrailingPayload,
		},
		{
			name: "trailing payload bytes",
			frame: func() []byte {
				return rewriteHeader(valid, wire.CurrentProtocolVersion, rpc.MessageMutationResponse, 1, 13)
			},
			want: rpc.ErrTrailingPayload,
		},
		{
			name: "unsupported version",
			frame: func() []byte {
				return rewriteHeader(valid, wire.CurrentProtocolVersion+1, rpc.MessageMutationResponse, 1, 14)
			},
			want: rpc.ErrUnsupportedVersion,
		},
		{
			name: "unknown type",
			frame: func() []byte {
				return rewriteHeader(valid, wire.CurrentProtocolVersion, 99, 1, 14)
			},
			want: rpc.ErrUnknownMessageType,
		},
		{
			name: "oversized payload",
			frame: func() []byte {
				return rewriteHeader(valid[:rpc.HeaderSize], wire.CurrentProtocolVersion, rpc.MessageMutationResponse, 1, rpc.MaxRPCPayload+1)
			},
			want: rpc.ErrPayloadTooLarge,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, _, err := rpc.UnmarshalFrame(test.frame()); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}

	if _, _, err := rpc.ReadFrame(bytes.NewReader(valid[:5])); !errors.Is(err, rpc.ErrTruncatedFrame) {
		t.Fatalf("short stream error = %v", err)
	}
}

func TestMessageValidation(t *testing.T) {
	t.Parallel()

	validMutation := rpc.MutationRequest{
		Op:         rpc.OpSet,
		Key:        []byte("key"),
		Value:      []byte("value"),
		MutationID: wire.NewMutationID(1, 1),
		Mode:       wire.WriteFast,
		Confirm:    wire.ConfirmInvalidateApplied,
	}
	tests := []struct {
		name    string
		message rpc.Message
		want    error
	}{
		{
			name:    "handshake request protocol",
			message: rpc.HandshakeRequest{},
			want:    wire.ErrInvalidProtocolRange,
		},
		{
			name:    "handshake generation",
			message: rpc.Handshake{ProtocolVersion: 1, PartitionCount: 1, StorageProfile: wire.StorageMemory},
			want:    rpc.ErrInvalidHubGeneration,
		},
		{
			name:    "handshake partitions",
			message: rpc.Handshake{ProtocolVersion: 1, HubGeneration: 1, StorageProfile: wire.StorageMemory},
			want:    wire.ErrInvalidPartitionCount,
		},
		{
			name: "handshake memory healthy",
			message: rpc.Handshake{
				ProtocolVersion: 1, HubGeneration: 1, PartitionCount: 1,
				StorageProfile: wire.StorageMemory, DurableHealthy: true,
			},
			want: rpc.ErrInvalidDurableHealth,
		},
		{
			name:    "handshake profile",
			message: rpc.Handshake{ProtocolVersion: 1, HubGeneration: 1, PartitionCount: 1, StorageProfile: 9},
			want:    wire.ErrInvalidStorageProfile,
		},
		{
			name:    "hub status generation",
			message: rpc.HubStatus{StorageProfile: wire.StorageMemory},
			want:    rpc.ErrInvalidHubGeneration,
		},
		{
			name: "hub status memory healthy",
			message: rpc.HubStatus{
				HubGeneration: 1, StorageProfile: wire.StorageMemory, DurableHealthy: true,
			},
			want: rpc.ErrInvalidDurableHealth,
		},
		{
			name:    "get empty key",
			message: rpc.GetRequest{},
			want:    wire.ErrKeyEmpty,
		},
		{
			name:    "get key too large",
			message: rpc.GetRequest{Key: make([]byte, wire.MaxKeyLen+1)},
			want:    wire.ErrKeyTooLarge,
		},
		{
			name: "get response not found with version",
			message: rpc.GetResponse{
				Status: wire.StatusNotFound, HubGeneration: 1,
				Version: wire.VersionTag{PartitionID: 1, Sequence: 1},
			},
			want: rpc.ErrUnexpectedVersion,
		},
		{
			name: "get response ok missing version",
			message: rpc.GetResponse{
				Status: wire.StatusOK, HubGeneration: 1, Kind: wire.RecordValue, Value: []byte("v"),
			},
			want: rpc.ErrMissingVersion,
		},
		{
			name: "get response ok zero sequence",
			message: rpc.GetResponse{
				Status: wire.StatusOK, HubGeneration: 1,
				Version: wire.VersionTag{PartitionID: 1, Sequence: 0},
				Kind:    wire.RecordValue, Value: []byte("v"),
			},
			want: rpc.ErrMissingVersion,
		},
		{
			name: "get response tombstone with value",
			message: rpc.GetResponse{
				Status: wire.StatusOK, HubGeneration: 1,
				Version: wire.VersionTag{PartitionID: 1, Sequence: 1},
				Kind:    wire.RecordTombstone, Value: []byte("x"),
			},
			want: rpc.ErrUnexpectedValue,
		},
		{
			name: "get response write confirm timeout",
			message: rpc.GetResponse{
				Status: wire.StatusErrWriteConfirmTimeout, HubGeneration: 1,
			},
			want: rpc.ErrInvalidGetStatus,
		},
		{
			name:    "mutation op",
			message: rpc.MutationRequest{Key: []byte("k"), MutationID: wire.NewMutationID(1, 1)},
			want:    rpc.ErrInvalidOp,
		},
		{
			name: "mutation delete with value",
			message: rpc.MutationRequest{
				Op: rpc.OpDelete, Key: []byte("k"), Value: []byte("v"),
				MutationID: wire.NewMutationID(1, 1),
			},
			want: rpc.ErrUnexpectedValue,
		},
		{
			name: "mutation delete with ttl",
			message: rpc.MutationRequest{
				Op: rpc.OpDelete, Key: []byte("k"), TTLMillis: 1,
				MutationID: wire.NewMutationID(1, 1),
			},
			want: rpc.ErrUnexpectedTTL,
		},
		{
			name: "mutation missing id",
			message: rpc.MutationRequest{
				Op: rpc.OpSet, Key: []byte("k"), Value: []byte("v"),
			},
			want: wire.ErrMutationIDRequired,
		},
		{
			name: "mutation bad mode",
			message: func() rpc.Message {
				m := validMutation
				m.Mode = 9
				return m
			}(),
			want: wire.ErrInvalidWriteMode,
		},
		{
			name: "mutation response ok without version",
			message: rpc.MutationResponse{
				Status: wire.StatusOK, HubGeneration: 1,
			},
			want: rpc.ErrMissingVersion,
		},
		{
			name: "mutation response ok zero sequence",
			message: rpc.MutationResponse{
				Status: wire.StatusOK, HubGeneration: 1,
				Version: wire.VersionTag{PartitionID: 1, Sequence: 0},
			},
			want: rpc.ErrMissingVersion,
		},
		{
			name: "mutation response w timeout without version",
			message: rpc.MutationResponse{
				Status: wire.StatusErrWriteConfirmTimeout, HubGeneration: 1,
			},
			want: rpc.ErrMissingVersion,
		},
		{
			name: "mutation response w timeout zero sequence",
			message: rpc.MutationResponse{
				Status: wire.StatusErrWriteConfirmTimeout, HubGeneration: 1,
				Version: wire.VersionTag{PartitionID: 3, Sequence: 0},
			},
			want: rpc.ErrMissingVersion,
		},
		{
			name: "mutation response durability with version",
			message: rpc.MutationResponse{
				Status: wire.StatusErrDurabilityUnavailable, HubGeneration: 1,
				Version: wire.VersionTag{PartitionID: 1, Sequence: 1},
			},
			want: rpc.ErrUnexpectedVersion,
		},
		{
			name: "mutation response not found",
			message: rpc.MutationResponse{
				Status: wire.StatusNotFound, HubGeneration: 1,
			},
			want: rpc.ErrInvalidMutationStatus,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := test.message.Validate(); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if _, err := rpc.MarshalFrame(1, test.message); !errors.Is(err, test.want) {
				t.Fatalf("marshal error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestMarshalRejectsNilMessagePointer(t *testing.T) {
	t.Parallel()

	var message *rpc.Handshake
	if _, err := rpc.MarshalFrame(1, message); !errors.Is(err, rpc.ErrInvalidMessage) {
		t.Fatalf("error = %v, want invalid message", err)
	}
}

func TestMutableDataIsCopiedAtBoundaries(t *testing.T) {
	t.Parallel()

	key := []byte("key")
	value := []byte("value")
	minVersion := wire.VersionTag{PartitionID: 1, Sequence: 1}
	get := rpc.GetRequest{Key: key, MinVersion: &minVersion}
	getClone := get.Clone()
	set := rpc.MutationRequest{
		Op:         rpc.OpSet,
		Key:        key,
		Value:      value,
		MutationID: wire.NewMutationID(1, 1),
	}
	setClone := set.Clone()
	resp := rpc.GetResponse{
		Status:        wire.StatusOK,
		HubGeneration: 1,
		Version:       wire.VersionTag{PartitionID: 1, Sequence: 1},
		Value:         value,
		Kind:          wire.RecordValue,
	}
	respClone := resp.Clone()

	frame, err := rpc.MarshalFrame(7, set)
	if err != nil {
		t.Fatal(err)
	}
	key[0] = 'K'
	value[0] = 'V'
	minVersion.Sequence = 99
	if string(getClone.Key) != "key" || getClone.MinVersion.Sequence != 1 {
		t.Fatalf("get clone aliases input: key=%q min=%v", getClone.Key, getClone.MinVersion)
	}
	if string(setClone.Key) != "key" || string(setClone.Value) != "value" {
		t.Fatalf("set clone aliases input: key=%q value=%q", setClone.Key, setClone.Value)
	}
	if string(respClone.Value) != "value" {
		t.Fatalf("response clone aliases input: %q", respClone.Value)
	}

	_, decodedMessage, err := rpc.UnmarshalFrame(frame)
	if err != nil {
		t.Fatal(err)
	}
	decoded := decodedMessage.(rpc.MutationRequest)
	// Corrupt a payload byte after decode; decoded value must already own its copy.
	frame[rpc.HeaderSize+1+4] ^= 0xff
	if string(decoded.Key) != "key" || string(decoded.Value) != "value" {
		t.Fatalf("decoded aliases frame: key=%q value=%q", decoded.Key, decoded.Value)
	}
}

func TestMutationFingerprintAndDedupKey(t *testing.T) {
	t.Parallel()

	base := rpc.MutationRequest{
		Op:         rpc.OpSet,
		Key:        []byte("key"),
		Value:      []byte("value"),
		TTLMillis:  10,
		MutationID: wire.NewMutationID(1, 7),
		Mode:       wire.WriteFast,
		W:          1,
		Confirm:    wire.ConfirmInvalidateApplied,
		Timeout:    100,
	}
	same := base
	same.Timeout = 999 // timeout is not part of the immutable fingerprint
	same.MutationID = wire.NewMutationID(9, 9)

	if !rpc.Fingerprint(base).Equal(rpc.Fingerprint(same)) {
		t.Fatal("fingerprint must ignore MutationID and Timeout")
	}

	mismatch := base
	mismatch.Mode = wire.WriteSync
	if rpc.Fingerprint(base).Equal(rpc.Fingerprint(mismatch)) {
		t.Fatal("fingerprint must include write mode")
	}
	if err := rpc.Fingerprint(base).CheckMatch(rpc.Fingerprint(mismatch)); !errors.Is(err, rpc.ErrMutationFingerprintMismatch) {
		t.Fatalf("CheckMatch error = %v, want fingerprint mismatch", err)
	}

	keyA := rpc.DedupKey{NodeID: 1, MutationID: base.MutationID}
	keyB := rpc.DedupKey{NodeID: 2, MutationID: base.MutationID}
	if keyA == keyB {
		t.Fatal("dedup keys must be scoped by authenticated node identity")
	}
	if keyA.Validate() != nil {
		t.Fatalf("valid dedup key rejected: %v", keyA.Validate())
	}
	if err := (rpc.DedupKey{MutationID: base.MutationID}).Validate(); !errors.Is(err, rpc.ErrInvalidNodeID) {
		t.Fatalf("zero node error = %v", err)
	}
	if err := (rpc.DedupKey{NodeID: 1}).Validate(); !errors.Is(err, wire.ErrMutationIDRequired) {
		t.Fatalf("zero mutation error = %v", err)
	}
}

func TestGetRequestPartitionValidation(t *testing.T) {
	t.Parallel()

	key := []byte("routed-key")
	partition := wire.PartitionOf(key, 16)
	req := rpc.GetRequest{
		Key:        key,
		MinVersion: &wire.VersionTag{PartitionID: partition, Sequence: 3},
	}
	if err := req.ValidateForPartitions(16); err != nil {
		t.Fatalf("valid min version rejected: %v", err)
	}
	req.MinVersion.PartitionID = partition + 1
	if err := req.ValidateForPartitions(16); err == nil {
		t.Fatal("mismatched min version partition accepted")
	}
}

func TestWriteModeCarriedOnEveryMutation(t *testing.T) {
	t.Parallel()

	for _, mode := range []wire.WriteMode{wire.WriteFast, wire.WriteSync} {
		req := rpc.MutationRequest{
			Op:         rpc.OpSet,
			Key:        []byte("k"),
			Value:      []byte("v"),
			MutationID: wire.NewMutationID(1, 1),
			Mode:       mode,
		}
		frame, err := rpc.MarshalFrame(1, req)
		if err != nil {
			t.Fatalf("mode %d marshal: %v", mode, err)
		}
		_, decoded, err := rpc.UnmarshalFrame(frame)
		if err != nil {
			t.Fatalf("mode %d unmarshal: %v", mode, err)
		}
		got := decoded.(rpc.MutationRequest)
		if got.Mode != mode {
			t.Fatalf("mode = %d, want %d", got.Mode, mode)
		}
	}
}

func TestOversizedValueRejected(t *testing.T) {
	t.Parallel()

	req := rpc.MutationRequest{
		Op:         rpc.OpSet,
		Key:        []byte("k"),
		Value:      make([]byte, wire.MaxValueLen+1),
		MutationID: wire.NewMutationID(1, 1),
	}
	if err := req.Validate(); !errors.Is(err, wire.ErrValueTooLarge) {
		t.Fatalf("validation error = %v, want value too large", err)
	}
	if _, err := rpc.MarshalFrame(1, req); !errors.Is(err, wire.ErrValueTooLarge) {
		t.Fatalf("marshal error = %v, want value too large", err)
	}
}

func TestMaxSizedMutationEncodes(t *testing.T) {
	t.Parallel()

	req := rpc.MutationRequest{
		Op:         rpc.OpSet,
		Key:        make([]byte, wire.MaxKeyLen),
		Value:      make([]byte, wire.MaxValueLen),
		MutationID: wire.NewMutationID(1, 1),
		Mode:       wire.WriteFast,
	}
	for i := range req.Key {
		req.Key[i] = byte(i)
	}
	for i := range req.Value {
		req.Value[i] = byte(i * 3)
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("max mutation validation: %v", err)
	}
	frameBytes, err := rpc.MarshalFrame(1, req)
	if err != nil {
		t.Fatalf("max mutation marshal: %v", err)
	}
	payloadLen := len(frameBytes) - rpc.HeaderSize
	if payloadLen > rpc.MaxRPCPayload {
		t.Fatalf("payload %d exceeds MaxRPCPayload %d", payloadLen, rpc.MaxRPCPayload)
	}
	if payloadLen != wire.MaxKeyLen+wire.MaxValueLen+41 {
		t.Fatalf("payload length = %d, want %d", payloadLen, wire.MaxKeyLen+wire.MaxValueLen+41)
	}
	_, decoded, err := rpc.UnmarshalFrame(frameBytes)
	if err != nil {
		t.Fatalf("max mutation unmarshal: %v", err)
	}
	got := decoded.(rpc.MutationRequest)
	if len(got.Key) != wire.MaxKeyLen || len(got.Value) != wire.MaxValueLen {
		t.Fatalf("decoded lengths key=%d value=%d", len(got.Key), len(got.Value))
	}
}

func rewriteHeader(frame []byte, version wire.ProtocolVersion, messageType rpc.MessageType, correlationID, payloadLen uint32) []byte {
	rewritten := append([]byte(nil), frame...)
	if len(rewritten) < rpc.HeaderSize {
		rewritten = make([]byte, rpc.HeaderSize)
		binary.BigEndian.PutUint32(rewritten[0:4], 0x47435231)
	}
	binary.BigEndian.PutUint16(rewritten[4:6], uint16(version))
	binary.BigEndian.PutUint16(rewritten[6:8], uint16(messageType))
	binary.BigEndian.PutUint32(rewritten[8:12], correlationID)
	binary.BigEndian.PutUint32(rewritten[12:16], payloadLen)
	binary.BigEndian.PutUint32(rewritten[16:20], crc32.Checksum(rewritten[:16], crc32.MakeTable(crc32.Castagnoli)))
	return rewritten
}

// shortWriter forces partial Write calls so WriteFrame's write loop is exercised.
type shortWriter struct {
	bytes.Buffer
}

func (w *shortWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return w.Buffer.Write(p[:1])
}

func TestShortWriterEventuallyFlushes(t *testing.T) {
	t.Parallel()

	// Ensure shortWriter is not optimized away as only a helper.
	var w shortWriter
	if _, err := w.Write([]byte("ab")); err != nil {
		t.Fatal(err)
	}
	if w.String() != "a" {
		t.Fatalf("short writer wrote %q", w.String())
	}
	_ = io.EOF
}

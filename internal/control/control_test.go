package control_test

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"testing"

	"github.com/sanketn26/gossipcache/internal/control"
	"github.com/sanketn26/gossipcache/internal/wire"
)

func goldenMessages() map[string]control.Message {
	return map[string]control.Message{
		"hello": control.Hello{
			NodeID:   0x0102030405060708,
			Protocol: wire.CurrentProtocolRange(),
			Subscriptions: []control.StreamWatermark{
				{StreamID: 3, AppliedThrough: 9, HubGeneration: 2},
			},
		},
		"subscribe": control.Subscribe{
			HubGeneration: 2,
			StreamIDs:     []uint32{3, 7},
		},
		"invalidation_batch": control.InvalidationBatch{
			StreamID:      3,
			HubGeneration: 2,
			Events: []control.InvalidationEvent{
				{
					StreamSequence: 10,
					Key:            []byte("key"),
					Version:        wire.VersionTag{PartitionID: 3, Sequence: 21},
					Kind:           wire.RecordValue,
					MutationID:     wire.NewMutationID(8, 13),
				},
				{
					StreamSequence: 11,
					Key:            []byte{0x00, 0xff},
					Version:        wire.VersionTag{PartitionID: 3, Sequence: 22},
					Kind:           wire.RecordTombstone,
					MutationID:     wire.NewMutationID(8, 14),
				},
			},
		},
		"hop_frame_ack": control.HopFrameAck{
			StreamID: 3, ReceivedThrough: 11,
		},
		"stream_acknowledgement": control.StreamAcknowledgement{
			StreamID: 3, AppliedThrough: 10,
		},
		"stream_checkpoint": control.StreamCheckpoint{
			StreamID: 3, HubGeneration: 2, StreamHead: 11,
		},
		"replay_request": control.ReplayRequest{
			StreamID: 3, FromSequence: 4, ToSequence: 9,
		},
		"replay_unavailable": control.ReplayUnavailable{
			StreamID: 3, HubGeneration: 2, RequestedFrom: 4, OldestAvailable: 6, StreamHead: 11,
		},
		"invalidate_confirm": control.InvalidateConfirm{
			StreamID: 3, StreamSequence: 10, NodeID: 0x0102030405060708,
		},
	}
}

func TestGoldenFrames(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("testdata/control_vectors.json")
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
	for name, message := range messages {
		name, message := name, message
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			frame, err := control.MarshalFrame(message)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			got := hex.EncodeToString(frame)
			if got != vectors[name] {
				t.Errorf("frame hex:\n got %s\nwant %s", got, vectors[name])
			}
			decoded, err := control.UnmarshalFrame(frame)
			if err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if !reflect.DeepEqual(decoded, message) {
				t.Fatalf("round trip:\n got %#v\nwant %#v", decoded, message)
			}
		})
	}
}

func TestMessageTypeValues(t *testing.T) {
	t.Parallel()

	messages := []control.Message{
		control.Hello{},
		control.Subscribe{},
		control.InvalidationBatch{},
		control.HopFrameAck{},
		control.StreamAcknowledgement{},
		control.StreamCheckpoint{},
		control.ReplayRequest{},
		control.ReplayUnavailable{},
		control.InvalidateConfirm{},
	}
	for i, message := range messages {
		want := control.MessageType(i + 1)
		if message.Type() != want {
			t.Errorf("%T type = %d, want %d", message, message.Type(), want)
		}
		if !want.Valid() {
			t.Errorf("defined message type %d is invalid", want)
		}
	}
	if control.MessageType(0).Valid() || control.MessageType(10).Valid() {
		t.Fatal("unknown message type reported valid")
	}
}

func TestProtocolDefaults(t *testing.T) {
	t.Parallel()

	if control.DefaultSubscriberQueue != 4096 {
		t.Fatalf("subscriber queue = %d", control.DefaultSubscriberQueue)
	}
	if control.DefaultCheckpointInterval.String() != "1s" {
		t.Fatalf("checkpoint interval = %s", control.DefaultCheckpointInterval)
	}
	if control.DefaultStreamFreshnessTimeout.String() != "3s" {
		t.Fatalf("freshness timeout = %s", control.DefaultStreamFreshnessTimeout)
	}
}

func TestFrameHeaderAndStreamIO(t *testing.T) {
	t.Parallel()

	message := control.StreamCheckpoint{StreamID: 4, HubGeneration: 7, StreamHead: 99}
	var stream shortWriter
	if err := control.WriteFrame(&stream, &message); err != nil {
		t.Fatalf("write: %v", err)
	}
	frame := stream.Bytes()
	if len(frame) != control.HeaderSize+20 {
		t.Fatalf("frame length = %d, want %d", len(frame), control.HeaderSize+20)
	}
	if got := binary.BigEndian.Uint32(frame[0:4]); got != 0x47435331 {
		t.Fatalf("magic = %#x", got)
	}
	if got := binary.BigEndian.Uint16(frame[4:6]); got != uint16(wire.CurrentProtocolVersion) {
		t.Fatalf("version = %d", got)
	}
	if got := binary.BigEndian.Uint16(frame[6:8]); got != uint16(control.MessageStreamCheckpoint) {
		t.Fatalf("message type = %d", got)
	}
	if got := binary.BigEndian.Uint32(frame[8:12]); got != 20 {
		t.Fatalf("payload length = %d", got)
	}

	decoded, err := control.ReadFrame(bytes.NewReader(frame))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !reflect.DeepEqual(decoded, message) {
		t.Fatalf("decoded = %#v, want %#v", decoded, message)
	}
}

func TestMarshalFrameVersion(t *testing.T) {
	t.Parallel()

	message := control.StreamCheckpoint{StreamID: 4, HubGeneration: 7, StreamHead: 99}
	frame, err := control.MarshalFrameVersion(message, wire.CurrentProtocolVersion)
	if err != nil {
		t.Fatalf("marshal current version: %v", err)
	}
	if got := wire.ProtocolVersion(binary.BigEndian.Uint16(frame[4:6])); got != wire.CurrentProtocolVersion {
		t.Fatalf("encoded version = %d, want %d", got, wire.CurrentProtocolVersion)
	}
	if _, err := control.MarshalFrameVersion(message, wire.CurrentProtocolVersion+1); !errors.Is(err, control.ErrUnsupportedVersion) {
		t.Fatalf("future version error = %v, want unsupported version", err)
	}
	if _, err := control.MarshalFrameVersion(message, 0); !errors.Is(err, control.ErrUnsupportedVersion) {
		t.Fatalf("zero version error = %v, want unsupported version", err)
	}

	decoded, err := control.UnmarshalFrameVersion(frame, wire.CurrentProtocolVersion)
	if err != nil {
		t.Fatalf("unmarshal current version: %v", err)
	}
	if !reflect.DeepEqual(decoded, message) {
		t.Fatalf("decoded = %#v, want %#v", decoded, message)
	}

	var stream bytes.Buffer
	if err := control.WriteFrameVersion(&stream, message, wire.CurrentProtocolVersion); err != nil {
		t.Fatalf("write current version: %v", err)
	}
	decoded, err = control.ReadFrameVersion(&stream, wire.CurrentProtocolVersion)
	if err != nil {
		t.Fatalf("read current version: %v", err)
	}
	if !reflect.DeepEqual(decoded, message) {
		t.Fatalf("stream decoded = %#v, want %#v", decoded, message)
	}
}

func TestFrameFailuresFailClosed(t *testing.T) {
	t.Parallel()

	valid, err := control.MarshalFrame(control.HopFrameAck{StreamID: 1, ReceivedThrough: 2})
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
			frame: func() []byte { return append([]byte(nil), valid[:15]...) },
			want:  control.ErrTruncatedFrame,
		},
		{
			name: "bad magic",
			frame: func() []byte {
				frame := append([]byte(nil), valid...)
				frame[0] ^= 0xff
				return frame
			},
			want: control.ErrBadMagic,
		},
		{
			name: "bad crc",
			frame: func() []byte {
				frame := append([]byte(nil), valid...)
				frame[12] ^= 0xff
				return frame
			},
			want: control.ErrBadHeaderCRC,
		},
		{
			name: "truncated payload",
			frame: func() []byte {
				return append([]byte(nil), valid[:len(valid)-1]...)
			},
			want: control.ErrTruncatedFrame,
		},
		{
			name: "trailing frame bytes",
			frame: func() []byte {
				return append(append([]byte(nil), valid...), 0)
			},
			want: control.ErrTrailingPayload,
		},
		{
			name: "trailing payload bytes",
			frame: func() []byte {
				return rewriteHeader(valid, wire.CurrentProtocolVersion, control.MessageHopFrameAck, 11)
			},
			want: control.ErrTrailingPayload,
		},
		{
			name: "unsupported version",
			frame: func() []byte {
				return rewriteHeader(valid, wire.CurrentProtocolVersion+1, control.MessageHopFrameAck, 12)
			},
			want: control.ErrUnsupportedVersion,
		},
		{
			name: "unknown type",
			frame: func() []byte {
				return rewriteHeader(valid, wire.CurrentProtocolVersion, 99, 12)
			},
			want: control.ErrUnknownMessageType,
		},
		{
			name: "oversized payload",
			frame: func() []byte {
				return rewriteHeader(valid[:control.HeaderSize], wire.CurrentProtocolVersion, control.MessageHopFrameAck, control.MaxControlPayload+1)
			},
			want: control.ErrPayloadTooLarge,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := control.UnmarshalFrame(test.frame()); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}

	if _, err := control.ReadFrame(bytes.NewReader(valid[:5])); !errors.Is(err, control.ErrTruncatedFrame) {
		t.Fatalf("short stream error = %v", err)
	}
}

func TestMessageValidation(t *testing.T) {
	t.Parallel()

	validEvent := control.InvalidationEvent{
		StreamSequence: 1,
		Key:            []byte("key"),
		Version:        wire.VersionTag{PartitionID: 2, Sequence: 1},
		MutationID:     wire.NewMutationID(1, 1),
	}
	tests := []struct {
		name    string
		message control.Message
		want    error
	}{
		{name: "hello node", message: control.Hello{Protocol: wire.CurrentProtocolRange()}, want: control.ErrInvalidNodeID},
		{name: "hello protocol", message: control.Hello{NodeID: 1}, want: wire.ErrInvalidProtocolRange},
		{
			name: "hello duplicate",
			message: control.Hello{NodeID: 1, Protocol: wire.CurrentProtocolRange(), Subscriptions: []control.StreamWatermark{
				{StreamID: 2, HubGeneration: 1}, {StreamID: 2, HubGeneration: 1},
			}},
			want: control.ErrDuplicateSubscription,
		},
		{name: "subscribe empty", message: control.Subscribe{HubGeneration: 1}, want: control.ErrEmptySubscriptions},
		{name: "subscribe generation", message: control.Subscribe{StreamIDs: []uint32{1}}, want: control.ErrInvalidHubGeneration},
		{name: "batch empty", message: control.InvalidationBatch{HubGeneration: 1}, want: control.ErrEmptyInvalidationBatch},
		{
			name: "batch gap",
			message: control.InvalidationBatch{StreamID: 2, HubGeneration: 1, Events: []control.InvalidationEvent{
				validEvent,
				withStreamSequence(validEvent, 3),
			}},
			want: control.ErrNonContiguousBatch,
		},
		{
			name: "batch partition",
			message: control.InvalidationBatch{StreamID: 3, HubGeneration: 1, Events: []control.InvalidationEvent{
				validEvent,
			}},
			want: control.ErrPartitionMismatch,
		},
		{
			name: "batch mutation",
			message: control.InvalidationBatch{StreamID: 2, HubGeneration: 1, Events: []control.InvalidationEvent{
				func() control.InvalidationEvent {
					event := validEvent
					event.MutationID = wire.MutationID{}
					return event
				}(),
			}},
			want: wire.ErrMutationIDRequired,
		},
		{name: "hop sequence", message: control.HopFrameAck{}, want: control.ErrInvalidSequence},
		{name: "application sequence", message: control.StreamAcknowledgement{}, want: control.ErrInvalidSequence},
		{name: "checkpoint generation", message: control.StreamCheckpoint{}, want: control.ErrInvalidHubGeneration},
		{name: "replay from zero", message: control.ReplayRequest{ToSequence: 1}, want: control.ErrInvalidSequenceRange},
		{name: "replay reversed", message: control.ReplayRequest{FromSequence: 2, ToSequence: 1}, want: control.ErrInvalidSequenceRange},
		{
			name: "unavailable window",
			message: control.ReplayUnavailable{
				HubGeneration: 1, RequestedFrom: 1, OldestAvailable: 4, StreamHead: 2,
			},
			want: control.ErrInvalidSequenceRange,
		},
		{
			name: "unavailable request at oldest retained",
			message: control.ReplayUnavailable{
				HubGeneration: 1, RequestedFrom: 5, OldestAvailable: 5, StreamHead: 10,
			},
			want: control.ErrInvalidSequenceRange,
		},
		{
			name: "unavailable request inside retained window",
			message: control.ReplayUnavailable{
				HubGeneration: 1, RequestedFrom: 7, OldestAvailable: 5, StreamHead: 10,
			},
			want: control.ErrInvalidSequenceRange,
		},
		{name: "confirm node", message: control.InvalidateConfirm{StreamSequence: 1}, want: control.ErrInvalidNodeID},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := test.message.Validate(); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if _, err := control.MarshalFrame(test.message); !errors.Is(err, test.want) {
				t.Fatalf("marshal error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestMarshalRejectsNilMessagePointer(t *testing.T) {
	t.Parallel()

	var message *control.Hello
	if _, err := control.MarshalFrame(message); !errors.Is(err, control.ErrInvalidMessage) {
		t.Fatalf("error = %v, want invalid message", err)
	}
}

func TestInvalidationBatchRejectsOversizedPayloadBeforeMarshal(t *testing.T) {
	t.Parallel()

	const eventCount = 256
	events := make([]control.InvalidationEvent, eventCount)
	for i := range events {
		events[i] = control.InvalidationEvent{
			StreamSequence: uint64(i + 1),
			Key:            make([]byte, wire.MaxKeyLen),
			Version:        wire.VersionTag{PartitionID: 1, Sequence: uint64(i + 1)},
			MutationID:     wire.NewMutationID(1, uint64(i+1)),
		}
	}
	batch := control.InvalidationBatch{
		StreamID:      1,
		HubGeneration: 1,
		Events:        events,
	}
	if err := batch.Validate(); !errors.Is(err, control.ErrPayloadTooLarge) {
		t.Fatalf("validation error = %v, want payload too large", err)
	}
	if _, err := control.MarshalFrame(batch); !errors.Is(err, control.ErrPayloadTooLarge) {
		t.Fatalf("marshal error = %v, want payload too large", err)
	}
}

func TestDecodeRejectsImpossibleCountsBeforeAllocating(t *testing.T) {
	t.Parallel()

	hello := control.Hello{NodeID: 1, Protocol: wire.CurrentProtocolRange()}
	frame, err := control.MarshalFrame(hello)
	if err != nil {
		t.Fatal(err)
	}
	// Hello count begins after node ID and the two protocol fields.
	binary.BigEndian.PutUint32(frame[control.HeaderSize+12:], control.MaxSubscriptions)
	frame = rewriteHeader(frame, wire.CurrentProtocolVersion, control.MessageHello, uint32(len(frame)-control.HeaderSize))
	if _, err := control.UnmarshalFrame(frame); !errors.Is(err, control.ErrTruncatedFrame) {
		t.Fatalf("error = %v, want truncated frame", err)
	}
}

func TestMutableDataIsCopiedAtBoundaries(t *testing.T) {
	t.Parallel()

	key := []byte("key")
	batch := control.InvalidationBatch{
		StreamID:      1,
		HubGeneration: 1,
		Events: []control.InvalidationEvent{{
			StreamSequence: 1,
			Key:            key,
			Version:        wire.VersionTag{PartitionID: 1, Sequence: 1},
			MutationID:     wire.NewMutationID(1, 1),
		}},
	}
	clone := batch.Clone()
	frame, err := control.MarshalFrame(batch)
	if err != nil {
		t.Fatal(err)
	}
	key[0] = 'K'
	batch.Events[0].Key[1] = 'E'
	if string(clone.Events[0].Key) != "key" {
		t.Fatalf("clone aliases input: %q", clone.Events[0].Key)
	}
	decodedMessage, err := control.UnmarshalFrame(frame)
	if err != nil {
		t.Fatal(err)
	}
	decoded := decodedMessage.(control.InvalidationBatch)
	frame[control.HeaderSize+16+8+4] ^= 0xff
	if string(decoded.Events[0].Key) != "key" {
		t.Fatalf("decoded key aliases frame: %q", decoded.Events[0].Key)
	}

	streamIDs := []uint32{1}
	subscribeClone := (control.Subscribe{HubGeneration: 1, StreamIDs: streamIDs}).Clone()
	streamIDs[0] = 2
	if subscribeClone.StreamIDs[0] != 1 {
		t.Fatal("subscribe clone aliases input")
	}
}

func rewriteHeader(frame []byte, version wire.ProtocolVersion, messageType control.MessageType, payloadLen uint32) []byte {
	rewritten := append([]byte(nil), frame...)
	if len(rewritten) < control.HeaderSize {
		rewritten = append(rewritten, make([]byte, control.HeaderSize-len(rewritten))...)
	}
	binary.BigEndian.PutUint16(rewritten[4:6], uint16(version))
	binary.BigEndian.PutUint16(rewritten[6:8], uint16(messageType))
	binary.BigEndian.PutUint32(rewritten[8:12], payloadLen)
	// Castagnoli CRC32C of bytes 0..11. Keep the calculation independent from
	// the package's unexported table.
	binary.BigEndian.PutUint32(rewritten[12:16], crc32c(rewritten[:12]))
	return rewritten
}

func crc32c(data []byte) uint32 {
	const polynomial = 0x82f63b78
	crc := ^uint32(0)
	for _, value := range data {
		crc ^= uint32(value)
		for range 8 {
			mask := uint32(0) - (crc & 1)
			crc = (crc >> 1) ^ (polynomial & mask)
		}
	}
	return ^crc
}

func withStreamSequence(event control.InvalidationEvent, sequence uint64) control.InvalidationEvent {
	event.StreamSequence = sequence
	return event
}

type shortWriter struct {
	bytes.Buffer
}

func (w *shortWriter) Write(value []byte) (int, error) {
	if len(value) > 3 {
		value = value[:3]
	}
	return w.Buffer.Write(value)
}

var _ io.Writer = (*shortWriter)(nil)

package rpc

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/sanketn26/gossipcache/internal/frame"
	"github.com/sanketn26/gossipcache/internal/wire"
)

// frameMagic is ASCII "GCR1" (GossipCache RPC v1). It is distinct from the
// control-plane magic so the two ports fail closed on accidental mixups.
const frameMagic uint32 = 0x47435231

// Header is the decoded fixed portion of an RPC frame.
type Header struct {
	Version       wire.ProtocolVersion
	Type          MessageType
	CorrelationID uint32
	PayloadLen    uint32
}

// MarshalFrame validates and encodes one complete RPC frame.
func MarshalFrame(correlationID uint32, message Message) ([]byte, error) {
	return MarshalFrameVersion(correlationID, message, wire.CurrentProtocolVersion)
}

// MarshalFrameVersion validates and encodes one complete frame using a
// specific locally supported schema version.
func MarshalFrameVersion(correlationID uint32, message Message, version wire.ProtocolVersion) ([]byte, error) {
	if frame.IsNilMessage(message) {
		return nil, fmt.Errorf("%w: nil", ErrInvalidMessage)
	}
	if !supportedVersion(version) {
		return nil, fmt.Errorf("%w: %d", ErrUnsupportedVersion, version)
	}
	if err := message.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidMessage, err)
	}
	payload, err := marshalPayloadVersion(version, message)
	if err != nil {
		return nil, err
	}
	if len(payload) > MaxRPCPayload {
		return nil, ErrPayloadTooLarge
	}

	header := make([]byte, HeaderSize)
	frame.PutMagic(header, frameMagic)
	frame.PutVersion(header, version)
	frame.PutType(header, uint16(message.Type()))
	binary.BigEndian.PutUint32(header[8:12], correlationID)
	binary.BigEndian.PutUint32(header[12:16], uint32(len(payload)))
	frame.SealHeader(header)
	return frame.AppendFrame(header, payload), nil
}

// UnmarshalFrame decodes exactly one complete frame. Unknown schemas and
// malformed headers fail closed.
func UnmarshalFrame(raw []byte) (uint32, Message, error) {
	return unmarshalFrame(raw, nil)
}

// UnmarshalFrameVersion decodes one complete frame and requires the negotiated
// protocol version.
func UnmarshalFrameVersion(raw []byte, version wire.ProtocolVersion) (uint32, Message, error) {
	if !supportedVersion(version) {
		return 0, nil, fmt.Errorf("%w: %d", ErrUnsupportedVersion, version)
	}
	return unmarshalFrame(raw, &version)
}

func unmarshalFrame(raw []byte, expectedVersion *wire.ProtocolVersion) (uint32, Message, error) {
	if len(raw) < HeaderSize {
		return 0, nil, ErrTruncatedFrame
	}
	header, err := parseHeader(raw[:HeaderSize])
	if err != nil {
		return 0, nil, err
	}
	if expectedVersion != nil && header.Version != *expectedVersion {
		return 0, nil, fmt.Errorf("%w: got %d, want %d", ErrUnsupportedVersion, header.Version, *expectedVersion)
	}
	_, payload, err := frame.SplitFrame(raw, HeaderSize, header.PayloadLen, ErrTruncatedFrame, ErrTrailingPayload)
	if err != nil {
		return 0, nil, err
	}
	message, err := unmarshalPayloadVersion(header.Version, header.Type, payload)
	if err != nil {
		return 0, nil, err
	}
	return header.CorrelationID, message, nil
}

// ReadFrame reads and decodes one frame from a stream.
func ReadFrame(reader io.Reader) (uint32, Message, error) {
	return readFrame(reader, nil)
}

// ReadFrameVersion reads one frame and requires the negotiated protocol version.
func ReadFrameVersion(reader io.Reader, version wire.ProtocolVersion) (uint32, Message, error) {
	if !supportedVersion(version) {
		return 0, nil, fmt.Errorf("%w: %d", ErrUnsupportedVersion, version)
	}
	return readFrame(reader, &version)
}

func readFrame(reader io.Reader, expectedVersion *wire.ProtocolVersion) (uint32, Message, error) {
	headerBytes, err := frame.ReadExact(reader, HeaderSize, ErrTruncatedFrame)
	if err != nil {
		return 0, nil, err
	}
	header, err := parseHeader(headerBytes)
	if err != nil {
		return 0, nil, err
	}
	if expectedVersion != nil && header.Version != *expectedVersion {
		return 0, nil, fmt.Errorf("%w: got %d, want %d", ErrUnsupportedVersion, header.Version, *expectedVersion)
	}
	payload, err := frame.ReadExact(reader, int(header.PayloadLen), ErrTruncatedFrame)
	if err != nil {
		return 0, nil, err
	}
	message, err := unmarshalPayloadVersion(header.Version, header.Type, payload)
	if err != nil {
		return 0, nil, err
	}
	return header.CorrelationID, message, nil
}

// WriteFrame writes one validated frame to a stream.
func WriteFrame(writer io.Writer, correlationID uint32, message Message) error {
	raw, err := MarshalFrame(correlationID, message)
	if err != nil {
		return err
	}
	return frame.WriteAll(writer, raw)
}

// WriteFrameVersion writes one validated frame using the negotiated protocol
// version.
func WriteFrameVersion(writer io.Writer, correlationID uint32, message Message, version wire.ProtocolVersion) error {
	raw, err := MarshalFrameVersion(correlationID, message, version)
	if err != nil {
		return err
	}
	return frame.WriteAll(writer, raw)
}

func parseHeader(encoded []byte) (Header, error) {
	if len(encoded) != HeaderSize {
		return Header{}, ErrTruncatedFrame
	}
	if frame.Magic(encoded) != frameMagic {
		return Header{}, ErrBadMagic
	}
	if !frame.VerifyHeaderCRC(encoded) {
		return Header{}, ErrBadHeaderCRC
	}
	version := frame.Version(encoded)
	if !supportedVersion(version) {
		return Header{}, fmt.Errorf("%w: %d", ErrUnsupportedVersion, version)
	}
	messageType := MessageType(frame.Type(encoded))
	if !messageType.Valid() {
		return Header{}, fmt.Errorf("%w: %d", ErrUnknownMessageType, messageType)
	}
	correlationID := binary.BigEndian.Uint32(encoded[8:12])
	payloadLen := binary.BigEndian.Uint32(encoded[12:16])
	if payloadLen > MaxRPCPayload {
		return Header{}, fmt.Errorf("%w: %d", ErrPayloadTooLarge, payloadLen)
	}
	return Header{
		Version:       version,
		Type:          messageType,
		CorrelationID: correlationID,
		PayloadLen:    payloadLen,
	}, nil
}

// supportedVersion reports whether this package has a codec for version.
// Schema support stays plane-local so control and rpc can evolve independently.
func supportedVersion(version wire.ProtocolVersion) bool {
	current := wire.CurrentProtocolRange()
	if version < current.MinSupported || version > current.Version {
		return false
	}
	switch version {
	case 1:
		return true
	default:
		return false
	}
}
